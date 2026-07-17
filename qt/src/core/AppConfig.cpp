#include "core/AppConfig.h"

#include <QDir>
#include <QFileInfo>
#include <QJsonDocument>
#include <QJsonObject>
#include <QCoreApplication>
#include <QStandardPaths>

#ifndef CLAWDSECBOT_VERSION
#define CLAWDSECBOT_VERSION "0.0.0"
#endif

AppConfig AppConfig::load() {
    const QString home = QDir::homePath();
#if defined(Q_OS_MACOS)
    const QString appData = QDir(home).filePath(QStringLiteral("Library/Application Support/com.bot.secnova.clawdsecbot"));
#else
    const QString appData = QStandardPaths::writableLocation(QStandardPaths::AppDataLocation);
#endif
    QDir{}.mkpath(appData);

    QString sandboxDir = QDir(home).filePath(QStringLiteral(".botsec"));
    QString logDir = QDir(appData).filePath(QStringLiteral("logs"));
    const QString configPath = QDir(appData).filePath(QStringLiteral("app_config.json"));
    QFile configFile(configPath);
    if (configFile.open(QIODevice::ReadOnly)) {
        const QJsonObject object = QJsonDocument::fromJson(configFile.readAll()).object();
        const QString configuredSandbox = object.value(QStringLiteral("sandbox_dir")).toString().trimmed();
        const QString configuredLog = object.value(QStringLiteral("log_dir")).toString().trimmed();
        if (!configuredSandbox.isEmpty()) sandboxDir = configuredSandbox;
        if (!configuredLog.isEmpty()) logDir = configuredLog;
    }

    QString libraryPath;
#if defined(Q_OS_MACOS)
    libraryPath = QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("../Resources/plugins/botsec.dylib"));
#elif defined(Q_OS_WIN)
    libraryPath = QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("plugins/botsec.dll"));
#else
    libraryPath = QDir(QCoreApplication::applicationDirPath()).filePath(QStringLiteral("plugins/botsec.so"));
#endif
    if (!QFileInfo::exists(libraryPath)) {
#if defined(Q_OS_MACOS)
        const QString libraryName = QStringLiteral("botsec.dylib");
#elif defined(Q_OS_WIN)
        const QString libraryName = QStringLiteral("botsec.dll");
#else
        const QString libraryName = QStringLiteral("botsec.so");
#endif
        const QString fromWorkingTree = QDir::current().filePath(QStringLiteral("../plugins/%1").arg(libraryName));
        const QString fromRepositoryRoot = QDir::current().filePath(QStringLiteral("plugins/%1").arg(libraryName));
        libraryPath = QFileInfo::exists(fromWorkingTree) ? fromWorkingTree : fromRepositoryRoot;
    }
    return {
        appData,
        home,
        sandboxDir,
        logDir,
        QDir::cleanPath(libraryPath),
        QString::fromLatin1(CLAWDSECBOT_VERSION),
        false,
    };
}
