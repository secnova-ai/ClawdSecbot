#include "app/AppController.h"

#include "common/AppLogger.h"
#include "service/ProtectionService.h"
#include "ui/MainWindow.h"

#include <QApplication>
#include <QIcon>
#include <QFutureWatcher>
#include <QJsonObject>
#include <QMenu>
#include <QMessageBox>
#include <QPushButton>
#include <QSystemTrayIcon>
#include <QtConcurrent>

AppController::AppController(const AppConfig& config) : config_(config) {
    mainWindow_ = std::make_unique<MainWindow>(&bridge_);
    mainWindow_->show();
    createTrayIcon();
    auto* watcher = new QFutureWatcher<QPair<bool, QString>>(mainWindow_.get());
    QObject::connect(watcher, &QFutureWatcher<QPair<bool, QString>>::finished, mainWindow_.get(), [this, watcher]() {
        const auto result = watcher->result();
        if (!result.first) {
            AppLogger::error(QStringLiteral("Go bridge initialization failed: %1").arg(result.second));
            QMessageBox::warning(mainWindow_.get(), QObject::tr("Go 业务库初始化失败"), result.second);
        } else {
            mainWindow_->initializeData();
        }
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = &bridge_, config = config_]() {
        QString error;
        const bool ready = bridge->initialize(config, &error);
        return qMakePair(ready, error);
    }));
}

AppController::~AppController() { bridge_.shutdown(); }

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
        qApp->quit();
        return;
    }
    quitInProgress_ = true;
    auto* countWatcher = new QFutureWatcher<QJsonObject>(mainWindow_.get());
    QObject::connect(countWatcher, &QFutureWatcher<QJsonObject>::finished, mainWindow_.get(), [this, countWatcher]() {
        const QJsonObject result = countWatcher->result();
        countWatcher->deleteLater();
        const int count = result.value(QStringLiteral("data")).toObject().value(QStringLiteral("count")).toInt();
        if (count <= 0) {
            qApp->quit();
            return;
        }

        QMessageBox box(QMessageBox::Warning, QObject::tr("退出前恢复 Bot"),
                        QObject::tr("当前有 %1 个 Bot 正在防护。是否在退出前停止防护并恢复原始配置？").arg(count),
                        QMessageBox::NoButton, mainWindow_.get());
        auto* cancel = box.addButton(QObject::tr("取消"), QMessageBox::RejectRole);
        auto* exitOnly = box.addButton(QObject::tr("仅退出"), QMessageBox::DestructiveRole);
        auto* restore = box.addButton(QObject::tr("恢复并退出"), QMessageBox::AcceptRole);
        box.setDefaultButton(restore);
        box.exec();
        if (box.clickedButton() == cancel || box.clickedButton() == nullptr) {
            quitInProgress_ = false;
            return;
        }
        if (box.clickedButton() == exitOnly) {
            qApp->quit();
            return;
        }

        auto* progress = new QMessageBox(QMessageBox::Information, QObject::tr("正在恢复"),
                                         QObject::tr("正在停止防护并恢复 Bot 原始配置，请稍候…"),
                                         QMessageBox::NoButton, mainWindow_.get());
        progress->setAttribute(Qt::WA_DeleteOnClose);
        progress->show();
        auto* restoreWatcher = new QFutureWatcher<QStringList>(mainWindow_.get());
        QObject::connect(restoreWatcher, &QFutureWatcher<QStringList>::finished, mainWindow_.get(),
                         [this, restoreWatcher, progress]() {
            const QStringList failures = restoreWatcher->result();
            restoreWatcher->deleteLater();
            progress->close();
            if (!failures.isEmpty()) {
                QMessageBox::warning(mainWindow_.get(), QObject::tr("部分 Bot 恢复失败"), failures.join(QLatin1Char('\n')));
            }
            qApp->quit();
        });
        restoreWatcher->setFuture(QtConcurrent::run([bridge = &bridge_]() { return ProtectionService::restoreAll(*bridge); }));
    });
    countWatcher->setFuture(QtConcurrent::run([bridge = &bridge_]() { return bridge->call("GetActiveProtectionCountFFI"); }));
}
