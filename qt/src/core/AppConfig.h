#pragma once

#include <QString>

struct AppConfig {
    QString workspaceDir;
    QString homeDir;
    QString sandboxDir;
    QString logDir;
    QString libraryPath;
    QString appVersion;
    bool verboseLogging = false;

    static AppConfig load();
};
