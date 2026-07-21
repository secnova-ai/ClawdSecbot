#include "app/AppController.h"

#include "common/AppLogger.h"
#include "service/ApiServerState.h"
#include "service/ProtectionService.h"
#include "ui/MainWindow.h"
#include "ui/UiDialogs.h"

#include <QApplication>
#include <QDialog>
#include <QEvent>
#include <QIcon>
#include <QFutureWatcher>
#include <QJsonObject>
#include <QMenu>
#include <QPointer>
#include <QSystemTrayIcon>
#include <QTimer>
#include <QtConcurrent>

AppController::AppController(const AppConfig& config) : config_(config) {
    qApp->installEventFilter(this);
    shutdownPool_.setMaxThreadCount(1);
    mainWindow_ = std::make_unique<MainWindow>(&bridge_);
    mainWindow_->show();
    createTrayIcon();
    auto* watcher = new QFutureWatcher<QPair<bool, QString>>(mainWindow_.get());
    QObject::connect(watcher, &QFutureWatcher<QPair<bool, QString>>::finished, mainWindow_.get(), [this, watcher]() {
        const auto result = watcher->result();
        if (!result.first) {
            AppLogger::error(QStringLiteral("Go bridge initialization failed: %1").arg(result.second));
            UiDialogs::showWarning(mainWindow_.get(), QObject::tr("Go 业务库初始化失败"), result.second);
        } else {
            mainWindow_->initializeData();
            restoreApiServer();
        }
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run(bridge_.workerPool(), [bridge = &bridge_, config = config_]() {
        QString error;
        const bool ready = bridge->initialize(config, &error);
        return qMakePair(ready, error);
    }));
}

void AppController::restoreApiServer() {
    auto* watcher = new QFutureWatcher<QJsonObject>(mainWindow_.get());
    QObject::connect(watcher, &QFutureWatcher<QJsonObject>::finished, mainWindow_.get(), [this, watcher]() {
        const QJsonObject result = watcher->result();
        watcher->deleteLater();
        if (!result.value(QStringLiteral("success")).toBool()) {
            AppLogger::error(QStringLiteral("Failed to restore API server state: %1").arg(result.value(QStringLiteral("error")).toString()));
            UiDialogs::showWarning(mainWindow_.get(), QObject::tr("API 服务启动失败"),
                                   result.value(QStringLiteral("error")).toString(QObject::tr("无法恢复上次的 API 服务状态。")));
        }
    });
    watcher->setFuture(QtConcurrent::run(bridge_.workerPool(), [bridge = &bridge_]() {
        const QJsonObject setting = bridge->call("GetAppSettingFFI", QStringLiteral("api_server_enabled"));
        if (!setting.value(QStringLiteral("success")).toBool()) return setting;
        const QJsonValue raw = setting.value(QStringLiteral("data"));
        const QJsonValue value = raw.isObject() ? raw.toObject().value(QStringLiteral("value")) : raw;
        const QString normalized = value.toVariant().toString().trimmed().toLower();
        const bool enabled = value.isBool() ? value.toBool() : normalized == QStringLiteral("true") || normalized == QStringLiteral("1");
        if (!enabled) return QJsonObject{{QStringLiteral("success"), true}};
        QJsonObject result = bridge->call("StartAPIServerFFI", QStringLiteral("{\"port\":0}"));
        if (!result.value(QStringLiteral("success")).toBool() && ApiServerState::isAlreadyInDesiredState(result, true)) {
            result = QJsonObject{{QStringLiteral("success"), true}};
        }
        return result;
    }));
}

AppController::~AppController() {
    qApp->removeEventFilter(this);
    shutdownPool_.waitForDone();
    if (!bridgeShutdown_) bridge_.shutdown();
}

bool AppController::eventFilter(QObject* watched, QEvent* event) {
    if (watched == qApp && event->type() == QEvent::Quit && !applicationQuitAllowed_) {
        event->ignore();
        QTimer::singleShot(0, mainWindow_.get(), [this]() { requestQuit(); });
        return true;
    }
    return QObject::eventFilter(watched, event);
}

void AppController::createTrayIcon() {
    if (!QSystemTrayIcon::isSystemTrayAvailable()) return;
    trayIcon_ = std::make_unique<QSystemTrayIcon>(QIcon(QStringLiteral(":/images/tray_icon.png")));
    auto* menu = new QMenu(mainWindow_.get());
    auto* showAction = menu->addAction(QObject::tr("显示 ClawdSecbot"));
    menu->addSeparator();
    auto* quitAction = menu->addAction(QObject::tr("退出"));
    QObject::connect(showAction, &QAction::triggered, mainWindow_.get(), [this]() {
        mainWindow_->show();
        mainWindow_->raise();
        mainWindow_->activateWindow();
    });
    QObject::connect(quitAction, &QAction::triggered, mainWindow_.get(), [this]() { requestQuit(); });
    QObject::connect(trayIcon_.get(), &QSystemTrayIcon::activated, mainWindow_.get(), [this](QSystemTrayIcon::ActivationReason reason) {
        if (reason == QSystemTrayIcon::Trigger || reason == QSystemTrayIcon::DoubleClick) {
            mainWindow_->show();
            mainWindow_->raise();
            mainWindow_->activateWindow();
        }
    });
    trayIcon_->setContextMenu(menu);
    trayIcon_->setToolTip(QStringLiteral("ClawdSecbot"));
    trayIcon_->show();
}

void AppController::requestQuit() {
    if (quitInProgress_) return;
    if (!bridge_.isReady()) {
        quitInProgress_ = true;
        beginAsyncShutdown();
        return;
    }
    quitInProgress_ = true;
    auto* countWatcher = new QFutureWatcher<QJsonObject>(mainWindow_.get());
    QObject::connect(countWatcher, &QFutureWatcher<QJsonObject>::finished, mainWindow_.get(), [this, countWatcher]() {
        const QJsonObject result = countWatcher->result();
        countWatcher->deleteLater();
        if (!result.value(QStringLiteral("success")).toBool()) {
            quitInProgress_ = false;
            UiDialogs::showWarning(mainWindow_.get(), QObject::tr("无法检查防护状态"),
                                   result.value(QStringLiteral("error")).toString(QObject::tr("业务引擎未返回有效结果，请稍后重试。")));
            return;
        }
        const int count = result.value(QStringLiteral("data")).toObject().value(QStringLiteral("count")).toInt();
        if (count <= 0) {
            beginAsyncShutdown();
            return;
        }

        UiDialogs::choose(mainWindow_.get(), QObject::tr("退出前恢复 Bot"),
                          QObject::tr("当前有 %1 个 Bot 正在防护。退出前恢复原始配置，可避免 Bot 保留本地代理地址。").arg(count),
                          DialogChrome::Tone::Warning,
                          {{QStringLiteral("cancel"), QObject::tr("取消"), UiDialogs::ButtonStyle::Secondary, false},
                           {QStringLiteral("exit"), QObject::tr("仅退出"), UiDialogs::ButtonStyle::Danger, false},
                           {QStringLiteral("restore"), QObject::tr("恢复并退出"), UiDialogs::ButtonStyle::Primary, true}},
                          [this](const QString& choice) {
            if (choice.isEmpty() || choice == QStringLiteral("cancel")) {
                quitInProgress_ = false;
                return;
            }
            if (choice == QStringLiteral("exit")) {
                beginAsyncShutdown();
                return;
            }
            const QPointer<QDialog> progress(UiDialogs::showProgress(
                mainWindow_.get(), QObject::tr("正在恢复"), QObject::tr("正在停止防护并恢复 Bot 原始配置，请稍候…")));
            auto* restoreWatcher = new QFutureWatcher<QStringList>(mainWindow_.get());
            QObject::connect(restoreWatcher, &QFutureWatcher<QStringList>::finished, mainWindow_.get(),
                             [this, restoreWatcher, progress]() {
                const QStringList failures = restoreWatcher->result();
                restoreWatcher->deleteLater();
                if (progress != nullptr) progress->close();
                if (!failures.isEmpty()) {
                    UiDialogs::choose(
                        mainWindow_.get(), QObject::tr("部分 Bot 恢复失败"),
                        QObject::tr("以下 Bot 未能恢复原始配置：\n%1\n\n直接退出可能使 Bot 保留本地代理地址。").arg(failures.join(QLatin1Char('\n'))),
                        DialogChrome::Tone::Danger,
                        {{QStringLiteral("cancel"), QObject::tr("返回应用"), UiDialogs::ButtonStyle::Secondary, true},
                         {QStringLiteral("exit"), QObject::tr("仍然退出"), UiDialogs::ButtonStyle::Danger, false}},
                        [this](const QString& choice) {
                            if (choice == QStringLiteral("exit")) beginAsyncShutdown();
                            else quitInProgress_ = false;
                        });
                    return;
                }
                beginAsyncShutdown();
            });
            restoreWatcher->setFuture(QtConcurrent::run(bridge_.workerPool(), [bridge = &bridge_]() { return ProtectionService::restoreAll(*bridge); }));
        });
    });
    countWatcher->setFuture(QtConcurrent::run(bridge_.workerPool(), [bridge = &bridge_]() {
        return ProtectionService::activeProtectionSummary(*bridge);
    }));
}

void AppController::beginAsyncShutdown() {
    if (bridgeShutdown_) {
        applicationQuitAllowed_ = true;
        qApp->quit();
        return;
    }
    if (shutdownStarted_) return;
    shutdownStarted_ = true;
    const QPointer<QDialog> progress(UiDialogs::showProgress(
        mainWindow_.get(), QObject::tr("正在退出"), QObject::tr("正在结束后台任务并关闭业务引擎，请稍候…")));
    auto* watcher = new QFutureWatcher<void>(mainWindow_.get());
    QObject::connect(watcher, &QFutureWatcher<void>::finished, mainWindow_.get(), [this, watcher, progress]() {
        bridgeShutdown_ = true;
        watcher->deleteLater();
        if (progress != nullptr) progress->close();
        applicationQuitAllowed_ = true;
        qApp->quit();
    });
    watcher->setFuture(QtConcurrent::run(&shutdownPool_, [this]() { bridge_.shutdown(); }));
}
