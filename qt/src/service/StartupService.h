#pragma once

#include <QString>

struct StartupServicePaths {
    QString executablePath;
    QString macLaunchAgentsDir;
    QString linuxAutostartDir;
};

class StartupService final {
public:
    static StartupServicePaths defaultPaths();
    static bool isEnabled(QString* errorMessage = nullptr);
    static bool setEnabled(bool enabled, QString* errorMessage = nullptr);

    // Exposed for deterministic tests without changing the current user's startup configuration.
    static bool isEnabledForPaths(const StartupServicePaths& paths, QString* errorMessage = nullptr);
    static bool setEnabledForPaths(bool enabled, const StartupServicePaths& paths, QString* errorMessage = nullptr);
};
