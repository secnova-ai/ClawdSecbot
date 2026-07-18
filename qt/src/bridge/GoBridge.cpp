#include "bridge/GoBridge.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonParseError>
#include <QMutexLocker>

GoBridge::GoBridge(QObject* parent) : QObject(parent) {}

GoBridge::~GoBridge() { shutdown(); }

GoBridge::CallLease::CallLease(const GoBridge& bridge, bool requireReady)
    : bridge_(bridge), active_(bridge_.beginCall(requireReady)) {}

GoBridge::CallLease::~CallLease() {
    if (active_) bridge_.endCall();
}

GoBridge::CallLease::operator bool() const { return active_; }

bool GoBridge::initialize(const AppConfig& config, QString* errorMessage) {
    const CallLease initializeLease(*this, false);
    if (!initializeLease) {
        if (errorMessage != nullptr) *errorMessage = QStringLiteral("Go bridge is shutting down.");
        return false;
    }
    if (ready_.load()) return true;
    library_.setFileName(config.libraryPath);
    if (!library_.load()) {
        const QString message = QStringLiteral("Failed to load Go library %1: %2")
                                    .arg(config.libraryPath, library_.errorString());
        if (errorMessage != nullptr) *errorMessage = message;
        emit bridgeError(message);
        return false;
    }
    freeString_ = reinterpret_cast<FreeStringFunction>(library_.resolve("FreeString"));
    if (freeString_ == nullptr) {
        const QString message = QStringLiteral("FreeString is missing from the Go library.");
        if (errorMessage != nullptr) *errorMessage = message;
        library_.unload();
        return false;
    }
    const auto failInitialization = [this, errorMessage](const QString& message) {
        if (errorMessage != nullptr) *errorMessage = message;
        ready_.store(false);
        freeString_ = nullptr;
        library_.unload();
        return false;
    };
    const auto responseError = [](const QString& stage, const QJsonObject& response) {
        return response.value(QStringLiteral("error")).toString(
            QStringLiteral("%1 returned an invalid response.").arg(stage));
    };
    using ThreeArgFunction = char* (*)(const char*, const char*, const char*);
    const QByteArray workspace = config.workspaceDir.toUtf8();
    const QByteArray home = config.homeDir.toUtf8();
    const QByteArray sandbox = config.sandboxDir.toUtf8();
    const auto initPathsWithConfig = reinterpret_cast<ThreeArgFunction>(library_.resolve("InitPathsWithConfigFFI"));
    if (initPathsWithConfig == nullptr)
        return failInitialization(QStringLiteral("InitPathsWithConfigFFI is missing from the Go library."));
    const QJsonObject paths = decodeAndFree(initPathsWithConfig(workspace.constData(), home.constData(), sandbox.constData()));
    if (!paths.value(QStringLiteral("success")).toBool())
        return failInitialization(responseError(QStringLiteral("Go path initialization"), paths));

    const auto initLogging = reinterpret_cast<OneArgFunction>(library_.resolve("InitLoggingFFI"));
    if (initLogging == nullptr)
        return failInitialization(QStringLiteral("InitLoggingFFI is missing from the Go library."));
    const QByteArray logDir = config.logDir.toUtf8();
    const QJsonObject logging = decodeAndFree(initLogging(logDir.constData()));
    if (!logging.value(QStringLiteral("success")).toBool())
        return failInitialization(responseError(QStringLiteral("Go logging initialization"), logging));

    const QJsonObject versionPayload{{QStringLiteral("current_version"), config.appVersion}};
    const auto initDatabase = reinterpret_cast<OneArgFunction>(library_.resolve("InitDatabase"));
    if (initDatabase == nullptr)
        return failInitialization(QStringLiteral("InitDatabase is missing from the Go library."));
    const QByteArray versionJson = QJsonDocument(versionPayload).toJson(QJsonDocument::Compact);
    const QJsonObject database = decodeAndFree(initDatabase(versionJson.constData()));
    if (!database.value(QStringLiteral("success")).toBool())
        return failInitialization(responseError(QStringLiteral("Go database initialization"), database));
    if (shuttingDown_.load()) {
        if (errorMessage != nullptr) *errorMessage = QStringLiteral("Go bridge was shut down during initialization.");
        return false;
    }
    ready_.store(true);
    return true;
}

void GoBridge::shutdown() {
    if (shuttingDown_.exchange(true)) return;
    ready_.store(false);
    workerPool_.waitForDone();
    {
        QMutexLocker locker(&callMutex_);
        while (activeCalls_ > 0) callsFinished_.wait(&callMutex_);
    }

    if (library_.isLoaded() && freeString_ != nullptr) {
        const auto closeDatabase = reinterpret_cast<NoArgFunction>(library_.resolve("CloseDatabase"));
        if (closeDatabase != nullptr) decodeAndFree(closeDatabase());
    }
    freeString_ = nullptr;
    if (library_.isLoaded()) library_.unload();
}

bool GoBridge::isReady() const { return ready_.load() && !shuttingDown_.load(); }
QString GoBridge::libraryPath() const { return library_.fileName(); }
QThreadPool* GoBridge::workerPool() { return &workerPool_; }

QJsonObject GoBridge::call(const char* method) const {
    const CallLease lease(*this, true);
    if (!lease) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<NoArgFunction>(library_.resolve(method));
    return fn == nullptr ? failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method))) : decodeAndFree(fn());
}

QJsonObject GoBridge::call(const char* method, const QString& value) const {
    const CallLease lease(*this, true);
    if (!lease) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<OneArgFunction>(library_.resolve(method));
    if (fn == nullptr) return failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method)));
    const QByteArray utf8 = value.toUtf8();
    return decodeAndFree(fn(utf8.constData()));
}

QJsonObject GoBridge::call(const char* method, const QString& first, const QString& second) const {
    const CallLease lease(*this, true);
    if (!lease) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<TwoArgFunction>(library_.resolve(method));
    if (fn == nullptr) return failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method)));
    const QByteArray firstUtf8 = first.toUtf8();
    const QByteArray secondUtf8 = second.toUtf8();
    return decodeAndFree(fn(firstUtf8.constData(), secondUtf8.constData()));
}

QJsonObject GoBridge::call(const char* method, const QString& value, int number) const {
    const CallLease lease(*this, true);
    if (!lease) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<OneArgOneIntFunction>(library_.resolve(method));
    if (fn == nullptr) return failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method)));
    const QByteArray utf8 = value.toUtf8();
    return decodeAndFree(fn(utf8.constData(), number));
}

QJsonObject GoBridge::callInt(const char* method, int value) const {
    const CallLease lease(*this, true);
    if (!lease) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<OneIntFunction>(library_.resolve(method));
    return fn == nullptr ? failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method))) : decodeAndFree(fn(value));
}

QJsonObject GoBridge::decodeAndFree(char* result) const {
    if (result == nullptr) return failure(QStringLiteral("Go returned a null result."));
    const QByteArray bytes(result);
    if (freeString_ != nullptr) freeString_(result);
    QJsonParseError error{};
    const QJsonDocument document = QJsonDocument::fromJson(bytes, &error);
    if (error.error != QJsonParseError::NoError || document.isNull()) {
        return failure(QStringLiteral("Invalid JSON from Go: %1").arg(error.errorString()));
    }
    if (document.isArray()) {
        return {{QStringLiteral("success"), true}, {QStringLiteral("data"), document.array()}};
    }
    return document.object();
}

QJsonObject GoBridge::failure(const QString& message) const {
    return {{QStringLiteral("success"), false}, {QStringLiteral("error"), message}};
}

bool GoBridge::beginCall(bool requireReady) const {
    QMutexLocker locker(&callMutex_);
    if (shuttingDown_.load() || (requireReady && !ready_.load())) return false;
    ++activeCalls_;
    return true;
}

void GoBridge::endCall() const {
    QMutexLocker locker(&callMutex_);
    --activeCalls_;
    if (activeCalls_ == 0) callsFinished_.wakeAll();
}
