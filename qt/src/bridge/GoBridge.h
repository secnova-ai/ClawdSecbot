#pragma once

#include "core/AppConfig.h"

#include <QJsonObject>
#include <QLibrary>
#include <QMutex>
#include <QObject>
#include <QString>
#include <QThreadPool>
#include <QWaitCondition>

#include <atomic>

class GoBridge final : public QObject {
    Q_OBJECT

public:
    explicit GoBridge(QObject* parent = nullptr);
    ~GoBridge() override;

    bool initialize(const AppConfig& config, QString* errorMessage = nullptr);
    void shutdown();
    bool isReady() const;
    QString libraryPath() const;
    QThreadPool* workerPool();

    QJsonObject call(const char* method) const;
    QJsonObject call(const char* method, const QString& value) const;
    QJsonObject call(const char* method, const QString& first, const QString& second) const;
    QJsonObject call(const char* method, const QString& value, int number) const;
    QJsonObject callInt(const char* method, int value) const;

signals:
    void bridgeError(const QString& message);

private:
    class CallLease {
    public:
        CallLease(const GoBridge& bridge, bool requireReady);
        ~CallLease();
        explicit operator bool() const;

    private:
        const GoBridge& bridge_;
        bool active_ = false;
    };

    using NoArgFunction = char* (*)();
    using OneArgFunction = char* (*)(const char*);
    using TwoArgFunction = char* (*)(const char*, const char*);
    using OneArgOneIntFunction = char* (*)(const char*, int);
    using OneIntFunction = char* (*)(int);
    using FreeStringFunction = void (*)(char*);

    QJsonObject decodeAndFree(char* result) const;
    QJsonObject failure(const QString& message) const;
    bool beginCall(bool requireReady) const;
    void endCall() const;

    mutable QLibrary library_;
    FreeStringFunction freeString_ = nullptr;
    std::atomic_bool ready_ = false;
    std::atomic_bool shuttingDown_ = false;
    mutable QMutex callMutex_;
    mutable QWaitCondition callsFinished_;
    mutable int activeCalls_ = 0;
    QThreadPool workerPool_;
};
