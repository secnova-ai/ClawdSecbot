#pragma once

#include "bridge/GoBridge.h"
#include "core/AppConfig.h"

#include <memory>

#include <QObject>
#include <QThreadPool>

class MainWindow;
class QEvent;
class QSystemTrayIcon;

class AppController final : public QObject {
public:
    explicit AppController(const AppConfig& config);
    ~AppController();

protected:
    bool eventFilter(QObject* watched, QEvent* event) override;

private:
    void createTrayIcon();
    void restoreApiServer();
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
    bool applicationQuitAllowed_ = false;
};
