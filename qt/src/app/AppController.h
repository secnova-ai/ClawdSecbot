#pragma once

#include "bridge/GoBridge.h"
#include "core/AppConfig.h"

#include <memory>

#include <QThreadPool>

class MainWindow;
class QSystemTrayIcon;

class AppController {
public:
    explicit AppController(const AppConfig& config);
    ~AppController();

private:
    void createTrayIcon();
    void requestQuit();
    void beginAsyncShutdown();

    AppConfig config_;
    GoBridge bridge_;
    std::unique_ptr<MainWindow> mainWindow_;
    std::unique_ptr<QSystemTrayIcon> trayIcon_;
    QThreadPool shutdownPool_;
    bool quitInProgress_ = false;
    bool shutdownStarted_ = false;
    bool bridgeShutdown_ = false;
};
