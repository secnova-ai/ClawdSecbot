#include "service/StartupService.h"

#include "common/AppLogger.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QSaveFile>
#include <QSettings>
#include <QStandardPaths>

namespace {
constexpr auto kStartupId = "com.bot.secnova.clawdsecbot";

bool writeFile(const QString& path, const QByteArray& content, QString* errorMessage) {
    if (!QDir{}.mkpath(QFileInfo(path).absolutePath())) {
        if (errorMessage != nullptr) *errorMessage = QStringLiteral("Failed to create startup directory: %1").arg(QFileInfo(path).absolutePath());
        return false;
    }
    QSaveFile file(path);
    if (!file.open(QIODevice::WriteOnly | QIODevice::Truncate)) {
        if (errorMessage != nullptr) *errorMessage = file.errorString();
        return false;
    }
    if (file.write(content) != content.size() || !file.commit()) {
        if (errorMessage != nullptr) *errorMessage = file.errorString();
        return false;
    }
    return true;
}

bool removeFile(const QString& path, QString* errorMessage) {
    if (!QFileInfo::exists(path) || QFile::remove(path)) return true;
    if (errorMessage != nullptr) *errorMessage = QStringLiteral("Failed to remove startup registration: %1").arg(path);
    return false;
}

#if defined(Q_OS_MACOS)
QString macPlistPath(const StartupServicePaths& paths) {
    return QDir(paths.macLaunchAgentsDir).filePath(QString::fromLatin1(kStartupId) + QStringLiteral(".plist"));
}
#endif

#if !defined(Q_OS_MACOS) && !defined(Q_OS_WIN)
QString linuxDesktopPath(const StartupServicePaths& paths) {
    return QDir(paths.linuxAutostartDir).filePath(QStringLiteral("clawdsecbot.desktop"));
}
#endif
}

StartupServicePaths StartupService::defaultPaths() {
    const QString home = QDir::homePath();
    return {
        QCoreApplication::applicationFilePath(),
        QDir(home).filePath(QStringLiteral("Library/LaunchAgents")),
        QDir(QStandardPaths::writableLocation(QStandardPaths::ConfigLocation)).filePath(QStringLiteral("autostart")),
    };
}

bool StartupService::isEnabled(QString* errorMessage) {
    return isEnabledForPaths(defaultPaths(), errorMessage);
}

bool StartupService::setEnabled(bool enabled, QString* errorMessage) {
    const bool success = setEnabledForPaths(enabled, defaultPaths(), errorMessage);
    if (success) AppLogger::info(QStringLiteral("Launch at startup %1.").arg(enabled ? QStringLiteral("enabled") : QStringLiteral("disabled")));
    else AppLogger::error(QStringLiteral("Failed to update launch at startup: %1").arg(errorMessage == nullptr ? QString() : *errorMessage));
    return success;
}

bool StartupService::isEnabledForPaths(const StartupServicePaths& paths, QString* errorMessage) {
#if defined(Q_OS_MACOS)
    QFile file(macPlistPath(paths));
    if (!file.exists()) return false;
    if (!file.open(QIODevice::ReadOnly)) {
        if (errorMessage != nullptr) *errorMessage = file.errorString();
        return false;
    }
    const QByteArray expected = QStringLiteral("<string>%1</string>").arg(paths.executablePath.toHtmlEscaped()).toUtf8();
    return file.readAll().contains(expected);
#elif defined(Q_OS_WIN)
    QSettings registry(QStringLiteral("HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"), QSettings::NativeFormat);
    if (registry.status() != QSettings::NoError) {
        if (errorMessage != nullptr) *errorMessage = QStringLiteral("Failed to read the Windows startup registry.");
        return false;
    }
    const QString registered = registry.value(QStringLiteral("ClawdSecbot")).toString();
    return registered == QStringLiteral("\"%1\"").arg(QDir::toNativeSeparators(paths.executablePath));
#else
    QFile file(linuxDesktopPath(paths));
    if (!file.exists()) return false;
    if (!file.open(QIODevice::ReadOnly)) {
        if (errorMessage != nullptr) *errorMessage = file.errorString();
        return false;
    }
    QString executable = paths.executablePath;
    executable.replace(QLatin1Char('\\'), QStringLiteral("\\\\"));
    executable.replace(QLatin1Char('"'), QStringLiteral("\\\""));
    return file.readAll().contains(QStringLiteral("Exec=\"%1\"").arg(executable).toUtf8());
#endif
}

bool StartupService::setEnabledForPaths(bool enabled, const StartupServicePaths& paths, QString* errorMessage) {
    if (paths.executablePath.trimmed().isEmpty()) {
        if (errorMessage != nullptr) *errorMessage = QStringLiteral("Application executable path is empty.");
        return false;
    }
#if defined(Q_OS_MACOS)
    const QString path = macPlistPath(paths);
    if (!enabled) return removeFile(path, errorMessage);
    const QString executable = paths.executablePath.toHtmlEscaped();
    const QString plist = QStringLiteral(
        "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
        "<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n"
        "<plist version=\"1.0\"><dict>\n"
        "<key>Label</key><string>%1</string>\n"
        "<key>ProgramArguments</key><array><string>%2</string></array>\n"
        "<key>RunAtLoad</key><true/>\n"
        "</dict></plist>\n")
        .arg(QString::fromLatin1(kStartupId), executable);
    return writeFile(path, plist.toUtf8(), errorMessage);
#elif defined(Q_OS_WIN)
    Q_UNUSED(paths.macLaunchAgentsDir)
    Q_UNUSED(paths.linuxAutostartDir)
    QSettings registry(QStringLiteral("HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"), QSettings::NativeFormat);
    if (enabled) registry.setValue(QStringLiteral("ClawdSecbot"), QStringLiteral("\"%1\"").arg(QDir::toNativeSeparators(paths.executablePath)));
    else registry.remove(QStringLiteral("ClawdSecbot"));
    registry.sync();
    if (registry.status() == QSettings::NoError) return true;
    if (errorMessage != nullptr) *errorMessage = QStringLiteral("Failed to update the Windows startup registry.");
    return false;
#else
    const QString path = linuxDesktopPath(paths);
    if (!enabled) return removeFile(path, errorMessage);
    QString executable = paths.executablePath;
    executable.replace(QLatin1Char('\\'), QStringLiteral("\\\\"));
    executable.replace(QLatin1Char('"'), QStringLiteral("\\\""));
    const QString desktop = QStringLiteral(
        "[Desktop Entry]\nType=Application\nName=ClawdSecbot\nExec=\"%1\"\n"
        "Terminal=false\nX-GNOME-Autostart-enabled=true\n")
        .arg(executable);
    return writeFile(path, desktop.toUtf8(), errorMessage);
#endif
}
