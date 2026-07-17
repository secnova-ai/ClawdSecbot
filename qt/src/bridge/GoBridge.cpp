#include "bridge/GoBridge.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonParseError>
#include <QThreadPool>

GoBridge::GoBridge(QObject* parent) : QObject(parent) {}

GoBridge::~GoBridge() { shutdown(); }

bool GoBridge::initialize(const AppConfig& config, QString* errorMessage) {
    if (shuttingDown_.load()) {
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
    ready_.store(true);

    using ThreeArgFunction = char* (*)(const char*, const char*, const char*);
    const QByteArray workspace = config.workspaceDir.toUtf8();
    const QByteArray home = config.homeDir.toUtf8();
    const QByteArray sandbox = config.sandboxDir.toUtf8();
    const auto initPathsWithConfig = reinterpret_cast<ThreeArgFunction>(library_.resolve("InitPathsWithConfigFFI"));
    if (initPathsWithConfig != nullptr) {
        decodeAndFree(initPathsWithConfig(workspace.constData(), home.constData(), sandbox.constData()));
    } else {
        const auto initPaths = reinterpret_cast<TwoArgFunction>(library_.resolve("InitPathsFFI"));
        if (initPaths != nullptr) decodeAndFree(initPaths(workspace.constData(), home.constData()));
    }
    call("InitLoggingFFI", config.logDir);
    const QJsonObject versionPayload{{QStringLiteral("current_version"), config.appVersion}};
    const QJsonObject database = call("InitDatabase", QString::fromUtf8(QJsonDocument(versionPayload).toJson(QJsonDocument::Compact)));
    if (!database.value(QStringLiteral("success")).toBool(true)) {
        const QString message = database.value(QStringLiteral("error")).toString(QStringLiteral("Go database initialization failed."));
        if (errorMessage != nullptr) *errorMessage = message;
        ready_.store(false);
        freeString_ = nullptr;
        library_.unload();
        return false;
    }
    return true;
}

void GoBridge::shutdown() {
    if (shuttingDown_.exchange(true)) return;

    // Every QtConcurrent task in this client uses the global pool. Waiting here
    // keeps the Go symbols loaded until all queued and running FFI calls finish.
    QThreadPool::globalInstance()->waitForDone();

    if (library_.isLoaded() && freeString_ != nullptr) {
        const auto closeDatabase = reinterpret_cast<NoArgFunction>(library_.resolve("CloseDatabase"));
        if (closeDatabase != nullptr) decodeAndFree(closeDatabase());
    }
    ready_.store(false);
    freeString_ = nullptr;
    if (library_.isLoaded()) library_.unload();
}

bool GoBridge::isReady() const { return ready_.load() && !shuttingDown_.load(); }
QString GoBridge::libraryPath() const { return library_.fileName(); }

QJsonObject GoBridge::call(const char* method) const {
    if (!isReady()) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<NoArgFunction>(library_.resolve(method));
    return fn == nullptr ? failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method))) : decodeAndFree(fn());
}

QJsonObject GoBridge::call(const char* method, const QString& value) const {
    if (!isReady()) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<OneArgFunction>(library_.resolve(method));
    if (fn == nullptr) return failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method)));
    const QByteArray utf8 = value.toUtf8();
    return decodeAndFree(fn(utf8.constData()));
}

QJsonObject GoBridge::call(const char* method, const QString& first, const QString& second) const {
    if (!isReady()) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<TwoArgFunction>(library_.resolve(method));
    if (fn == nullptr) return failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method)));
    const QByteArray firstUtf8 = first.toUtf8();
    const QByteArray secondUtf8 = second.toUtf8();
    return decodeAndFree(fn(firstUtf8.constData(), secondUtf8.constData()));
}

QJsonObject GoBridge::call(const char* method, const QString& value, int number) const {
    if (!isReady()) return failure(QStringLiteral("Go bridge is not ready."));
    const auto fn = reinterpret_cast<OneArgOneIntFunction>(library_.resolve(method));
    if (fn == nullptr) return failure(QStringLiteral("Missing Go symbol: %1").arg(QString::fromLatin1(method)));
    const QByteArray utf8 = value.toUtf8();
    return decodeAndFree(fn(utf8.constData(), number));
}

QJsonObject GoBridge::callInt(const char* method, int value) const {
    if (!isReady()) return failure(QStringLiteral("Go bridge is not ready."));
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
