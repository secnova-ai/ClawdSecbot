#include "ui/dialogs/SettingsDialog.h"

#include "bridge/GoBridge.h"
#include "common/AppLogger.h"
#include "service/ApiServerState.h"
#include "service/StartupService.h"
#include "ui/DialogChrome.h"
#include "ui/UiDialogs.h"

#include <QCheckBox>
#include <QComboBox>
#include <QFutureWatcher>
#include <QHBoxLayout>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QLineEdit>
#include <QPushButton>
#include <QScrollArea>
#include <QTabBar>
#include <QTabWidget>
#include <QVBoxLayout>
#include <QtConcurrent>

namespace {
QLabel* settingsTitleLabel(const QString& text, QWidget* parent, const char* name = "dialogTitle") {
    auto* label = new QLabel(text, parent);
    label->setObjectName(QString::fromLatin1(name));
    return label;
}

QWidget* settingsScrollPage(QWidget* content, QWidget* parent) {
    auto* scroll = new QScrollArea(parent);
    scroll->setWidgetResizable(true);
    scroll->setFrameShape(QFrame::NoFrame);
    scroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    scroll->setWidget(content);
    return scroll;
}

QWidget* settingsFieldBlock(const QString& labelText, QWidget* field, QWidget* parent) {
    auto* block = new QWidget(parent);
    auto* layout = new QVBoxLayout(block);
    layout->setContentsMargins(0, 0, 0, 0);
    layout->setSpacing(6);
    auto* label = new QLabel(labelText, block);
    label->setObjectName(QStringLiteral("muted"));
    layout->addWidget(label);
    layout->addWidget(field);
    return block;
}

QFrame* settingsActionRow(const QString& icon, const QString& title, const QString& description, QWidget* parent) {
    auto* frame = new QFrame(parent);
    frame->setObjectName(QStringLiteral("card"));
    auto* layout = new QHBoxLayout(frame);
    layout->setContentsMargins(12, 10, 12, 10);
    auto* glyph = new QLabel(icon, frame);
    glyph->setStyleSheet(QStringLiteral("font-size: 18px; color: #818CF8;"));
    layout->addWidget(glyph);
    auto* text = new QWidget(frame);
    auto* textLayout = new QVBoxLayout(text);
    textLayout->setContentsMargins(0, 0, 0, 0);
    textLayout->setSpacing(2);
    textLayout->addWidget(settingsTitleLabel(title, text, "sectionTitle"));
    auto* detail = new QLabel(description, text);
    detail->setObjectName(QStringLiteral("subtle"));
    detail->setWordWrap(true);
    textLayout->addWidget(detail);
    layout->addWidget(text, 1);
    return frame;
}

QJsonObject settingsUnwrap(const QJsonObject& response) {
    return response.value(QStringLiteral("data")).isObject() ? response.value(QStringLiteral("data")).toObject() : response;
}

bool settingBool(const QJsonValue& value) {
    if (value.isBool()) return value.toBool();
    const QString normalized = value.toVariant().toString().trimmed().toLower();
    return normalized == QStringLiteral("true") || normalized == QStringLiteral("1");
}

}

SettingsDialog::SettingsDialog(GoBridge* bridge, QWidget* parent)
    : QDialog(parent), bridge_(bridge) {
    setWindowTitle(tr("全局设置"));
    setProperty("tone", DialogChrome::toneName(DialogChrome::Tone::Accent));
    DialogChrome::prepare(this, QSize(540, 680), QSize(520, 640));
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(26, 24, 26, 22);
    root->setSpacing(18);
    root->addWidget(DialogChrome::createHeader(this, QStringLiteral("⚙"), QStringLiteral("全局设置"),
                                                QStringLiteral("配置安全模型、服务和本地数据")));

    tabs_ = new QTabWidget(this);
    tabs_->setObjectName(QStringLiteral("segmentedTabs"));
    tabs_->tabBar()->setObjectName(QStringLiteral("settingsTabBar"));
    tabs_->tabBar()->setExpanding(true);
    tabs_->tabBar()->setDrawBase(false);
    auto* modelPage = new QWidget(tabs_);
    auto* modelLayout = new QVBoxLayout(modelPage);
    modelLayout->setContentsMargins(0, 16, 0, 0);
    modelLayout->setSpacing(12);
    provider_ = new QComboBox(modelPage);
    provider_->addItem(QStringLiteral("MiniMax-EN"), QStringLiteral("minimax"));
    provider_->addItem(QStringLiteral("OpenAI"), QStringLiteral("openai"));
    provider_->addItem(QStringLiteral("Anthropic"), QStringLiteral("anthropic"));
    provider_->addItem(QStringLiteral("DeepSeek"), QStringLiteral("deepseek"));
    provider_->addItem(QStringLiteral("Ollama"), QStringLiteral("ollama"));
    baseUrl_ = new QLineEdit(modelPage);
    apiKey_ = new QLineEdit(modelPage);
    apiKey_->setEchoMode(QLineEdit::Password);
    modelName_ = new QLineEdit(modelPage);
    modelLayout->addWidget(settingsFieldBlock(QStringLiteral("模型供应商"), provider_, modelPage));
    modelLayout->addWidget(settingsFieldBlock(QStringLiteral("基础 URL"), baseUrl_, modelPage));
    modelLayout->addWidget(settingsFieldBlock(QStringLiteral("API 密钥"), apiKey_, modelPage));
    modelLayout->addWidget(settingsFieldBlock(QStringLiteral("模型名称"), modelName_, modelPage));
    auto* hint = new QLabel(QStringLiteral("ⓘ  刷新模型列表前，请先填写 Base URL 和 API 密钥。"), modelPage);
    hint->setWordWrap(true);
    hint->setStyleSheet(QStringLiteral("padding: 10px; border: 1px solid rgba(99,102,241,90); border-radius: 8px; color: #A5B4FC;"));
    modelLayout->addWidget(hint);
    modelLayout->addStretch();
    tabs_->addTab(modelPage, QStringLiteral("♢  安全模型"));

    auto* general = new QWidget(tabs_);
    auto* generalLayout = new QVBoxLayout(general);
    generalLayout->setContentsMargins(0, 16, 0, 0);
    generalLayout->setSpacing(10);
    auto* startup = settingsActionRow(QStringLiteral("⏻"), QStringLiteral("开机自启"), QString(), general);
    startupCheck_ = new QCheckBox(startup);
    QString startupError;
    startupCheck_->setChecked(StartupService::isEnabled(&startupError));
    if (!startupError.isEmpty()) {
        AppLogger::error(QStringLiteral("Failed to read launch at startup state: %1").arg(startupError));
    }
    connect(startupCheck_, &QCheckBox::toggled, this, [this](bool checked) {
        startupCheck_->setEnabled(false);
        auto* watcher = new QFutureWatcher<QPair<bool, QString>>(this);
        connect(watcher, &QFutureWatcher<QPair<bool, QString>>::finished, this, [this, watcher, checked]() {
            const auto result = watcher->result();
            startupCheck_->setEnabled(true);
            if (!result.first) {
                startupCheck_->blockSignals(true);
                startupCheck_->setChecked(!checked);
                startupCheck_->blockSignals(false);
                UiDialogs::showWarning(this, QStringLiteral("开机自启设置失败"), result.second);
            }
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run([checked]() {
            QString error;
            return qMakePair(StartupService::setEnabled(checked, &error), error);
        }));
    });
    static_cast<QHBoxLayout*>(startup->layout())->addWidget(startupCheck_);
    generalLayout->addWidget(startup);
    auto* schedule = settingsActionRow(QStringLiteral("◴"), QStringLiteral("定时扫描设置"), QStringLiteral("按设定间隔自动执行安全扫描"), general);
    scheduleCombo_ = new QComboBox(schedule);
    scheduleCombo_->addItems({QStringLiteral("关闭"), QStringLiteral("30 分钟"), QStringLiteral("1 小时"), QStringLiteral("6 小时"), QStringLiteral("24 小时")});
    connect(scheduleCombo_, &QComboBox::currentIndexChanged, this, [this](int index) {
        if (bridge_ == nullptr) return;
        const QList<int> seconds{0, 1800, 3600, 21600, 86400};
        const int selectedSeconds = seconds.value(index);
        const QJsonObject payload{{QStringLiteral("key"), QStringLiteral("scheduled_scan_interval_seconds")}, {QStringLiteral("value"), QString::number(selectedSeconds)}};
        const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
        scheduleCombo_->setEnabled(false);
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, selectedSeconds]() {
            scheduleCombo_->setEnabled(true);
            const QJsonObject result = watcher->result();
            if (result.value(QStringLiteral("success")).toBool()) emit scheduledScanIntervalChanged(selectedSeconds);
            else {
                UiDialogs::showWarning(this, QStringLiteral("定时扫描设置失败"), result.value(QStringLiteral("error")).toString());
                const QList<int> intervals{0, 1800, 3600, 21600, 86400};
                const int currentIndex = intervals.indexOf(scheduleCombo_->property("persistedSeconds").toInt());
                scheduleCombo_->blockSignals(true);
                scheduleCombo_->setCurrentIndex(currentIndex >= 0 ? currentIndex : 0);
                scheduleCombo_->blockSignals(false);
            }
            if (result.value(QStringLiteral("success")).toBool()) scheduleCombo_->setProperty("persistedSeconds", selectedSeconds);
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, json]() { return bridge->call("SaveAppSettingFFI", json); }));
    });
    if (bridge_ != nullptr && bridge_->isReady()) {
        auto* scheduleWatcher = new QFutureWatcher<QJsonObject>(this);
        connect(scheduleWatcher, &QFutureWatcher<QJsonObject>::finished, this, [this, scheduleWatcher]() {
            const QJsonObject response = scheduleWatcher->result();
            if (!response.value(QStringLiteral("success")).toBool()) {
                AppLogger::error(QStringLiteral("Failed to load scheduled scan setting: %1")
                                     .arg(response.value(QStringLiteral("error")).toString()));
                scheduleWatcher->deleteLater();
                return;
            }
            const QJsonValue raw = response.value(QStringLiteral("data"));
            const int seconds = raw.isObject() ? raw.toObject().value(QStringLiteral("value")).toVariant().toInt() : raw.toVariant().toInt();
            const QList<int> intervals{0, 1800, 3600, 21600, 86400};
            const int index = intervals.indexOf(seconds);
            scheduleCombo_->setProperty("persistedSeconds", seconds);
            scheduleCombo_->blockSignals(true);
            scheduleCombo_->setCurrentIndex(index >= 0 ? index : 0);
            scheduleCombo_->blockSignals(false);
            scheduleWatcher->deleteLater();
        });
        scheduleWatcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_]() { return bridge->call("GetAppSettingFFI", QStringLiteral("scheduled_scan_interval_seconds")); }));
    }
    static_cast<QHBoxLayout*>(schedule->layout())->addWidget(scheduleCombo_);
    generalLayout->addWidget(schedule);
    auto* api = settingsActionRow(QStringLiteral("▤"), QStringLiteral("API 服务"), QString(), general);
    auto* apiCheck = new QCheckBox(api);
    connect(apiCheck, &QCheckBox::toggled, this, [this, apiCheck](bool checked) {
        if (bridge_ == nullptr) return;
        apiCheck->setEnabled(false);
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, apiCheck, checked]() {
            const QJsonObject result = watcher->result();
            apiCheck->setEnabled(true);
            if (!result.value(QStringLiteral("success")).toBool()) {
                apiCheck->blockSignals(true);
                apiCheck->setChecked(!checked);
                apiCheck->blockSignals(false);
                UiDialogs::showWarning(this, QStringLiteral("API 服务"), result.value(QStringLiteral("error")).toString());
            }
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, checked]() {
            QJsonObject result = checked ? bridge->call("StartAPIServerFFI", QStringLiteral("{\"port\":0}")) : bridge->call("StopAPIServerFFI");
            const bool runtimeChanged = result.value(QStringLiteral("success")).toBool();
            if (!runtimeChanged && !ApiServerState::isAlreadyInDesiredState(result, checked)) return result;
            const QJsonObject payload{{QStringLiteral("key"), QStringLiteral("api_server_enabled")},
                                      {QStringLiteral("value"), checked ? QStringLiteral("true") : QStringLiteral("false")}};
            const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
            result = bridge->call("SaveAppSettingFFI", json);
            if (!result.value(QStringLiteral("success")).toBool() && runtimeChanged) {
                const QJsonObject rollback = checked ? bridge->call("StopAPIServerFFI")
                                                     : bridge->call("StartAPIServerFFI", QStringLiteral("{\"port\":0}"));
                if (!rollback.value(QStringLiteral("success")).toBool()) {
                    AppLogger::error(QStringLiteral("Failed to roll back API server state: %1")
                                         .arg(rollback.value(QStringLiteral("error")).toString()));
                }
            }
            return result;
        }));
    });
    if (bridge_ != nullptr && bridge_->isReady()) {
        auto* apiWatcher = new QFutureWatcher<QJsonObject>(this);
        connect(apiWatcher, &QFutureWatcher<QJsonObject>::finished, this, [apiWatcher, apiCheck]() {
            const QJsonObject response = apiWatcher->result();
            if (!response.value(QStringLiteral("success")).toBool()) {
                AppLogger::error(QStringLiteral("Failed to load API server setting: %1")
                                     .arg(response.value(QStringLiteral("error")).toString()));
                apiWatcher->deleteLater();
                return;
            }
            const QJsonValue raw = response.value(QStringLiteral("data"));
            const QJsonValue value = raw.isObject() ? raw.toObject().value(QStringLiteral("value")) : raw;
            apiCheck->blockSignals(true);
            apiCheck->setChecked(settingBool(value));
            apiCheck->blockSignals(false);
            apiWatcher->deleteLater();
        });
        apiWatcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_]() { return bridge->call("GetAppSettingFFI", QStringLiteral("api_server_enabled")); }));
    }
    static_cast<QHBoxLayout*>(api->layout())->addWidget(apiCheck);
    generalLayout->addWidget(api);
    auto* section = new QLabel(QStringLiteral("数据管理"), general);
    section->setObjectName(QStringLiteral("subtle"));
    generalLayout->addWidget(section);
    auto* clearData = new QPushButton(QStringLiteral("⌫  清空数据\n     清除日志、统计和分析数据"), general);
    clearData->setMinimumHeight(58);
    clearData->setStyleSheet(QStringLiteral("text-align:left;"));
    auto* restoreConfig = new QPushButton(QStringLiteral("↶  恢复初始配置\n     恢复到首次启动前的状态"), general);
    restoreConfig->setMinimumHeight(58);
    restoreConfig->setStyleSheet(QStringLiteral("text-align:left;"));
    auto* about = new QPushButton(QStringLiteral("ⓘ  关于 ClawdSecbot\n     版本 %1 / Qt 6").arg(QStringLiteral(CLAWDSECBOT_VERSION)), general);
    about->setMinimumHeight(58);
    about->setStyleSheet(QStringLiteral("text-align:left;"));
    generalLayout->addWidget(clearData);
    generalLayout->addWidget(restoreConfig);
    generalLayout->addWidget(about);
    connect(clearData, &QPushButton::clicked, this, [this, clearData]() {
        if (bridge_ == nullptr) return;
        UiDialogs::confirm(this, QStringLiteral("清空数据"), QStringLiteral("确定清空日志、统计和分析数据吗？"), [this, clearData]() {
            clearData->setEnabled(false);
            const QString original = clearData->text();
            clearData->setText(QStringLiteral("◌  正在清空数据…"));
            auto* watcher = new QFutureWatcher<QJsonObject>(this);
            connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, clearData, original]() {
                clearData->setEnabled(true);
                clearData->setText(original);
                const QJsonObject result = watcher->result();
                if (result.value(QStringLiteral("success")).toBool()) UiDialogs::showInformation(this, QStringLiteral("清空数据"), QStringLiteral("数据已清空"));
                else UiDialogs::showWarning(this, QStringLiteral("清空失败"), result.value(QStringLiteral("error")).toString());
                watcher->deleteLater();
            });
            watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_]() { return bridge->call("ClearAllDataFFI"); }));
        }, QStringLiteral("清空"));
    });
    connect(restoreConfig, &QPushButton::clicked, this, [this, restoreConfig]() {
        if (bridge_ == nullptr) return;
        UiDialogs::confirm(this, QStringLiteral("恢复初始配置"), QStringLiteral("确定恢复到首次启动前的状态吗？"), [this, restoreConfig]() {
            restoreConfig->setEnabled(false);
            const QString original = restoreConfig->text();
            restoreConfig->setText(QStringLiteral("◌  正在恢复初始配置…"));
            auto* watcher = new QFutureWatcher<QJsonObject>(this);
            connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, restoreConfig, original]() {
                restoreConfig->setEnabled(true);
                restoreConfig->setText(original);
                const QJsonObject result = watcher->result();
                if (result.value(QStringLiteral("success")).toBool()) UiDialogs::showInformation(this, QStringLiteral("恢复初始配置"), QStringLiteral("配置已恢复到初始状态"));
                else UiDialogs::showWarning(this, QStringLiteral("恢复失败"), result.value(QStringLiteral("error")).toString());
                watcher->deleteLater();
            });
            watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_]() { return bridge->call("RestoreToInitialConfigFFI"); }));
        }, QStringLiteral("恢复"));
    });
    connect(about, &QPushButton::clicked, this, [this]() {
        UiDialogs::showAbout(this, QStringLiteral("关于 ClawdSecbot"),
                             QStringLiteral("ClawdSecbot %1\nQt 6 桌面客户端\nGo 安全业务引擎").arg(QStringLiteral(CLAWDSECBOT_VERSION)));
    });
    generalLayout->addStretch();
    tabs_->addTab(settingsScrollPage(general, tabs_), QStringLiteral("☷  通用设置"));
    root->addWidget(tabs_, 1);

    auto* buttons = new QHBoxLayout;
    buttons->addStretch();
    auto* cancel = new QPushButton(QStringLiteral("取消"), this);
    auto* validate = new QPushButton(QStringLiteral("验证连通性"), this);
    saveButton_ = new QPushButton(QStringLiteral("保存"), this);
    saveButton_->setObjectName(QStringLiteral("primaryButton"));
    connect(cancel, &QPushButton::clicked, this, &QDialog::reject);
    connect(validate, &QPushButton::clicked, this, [this, validate]() {
        if (bridge_ == nullptr) return;
        const QJsonObject payload{{QStringLiteral("provider"), provider_->currentData().toString()}, {QStringLiteral("endpoint"), baseUrl_->text()}, {QStringLiteral("api_key"), apiKey_->text()}, {QStringLiteral("model"), modelName_->text()}};
        const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        validate->setEnabled(false);
        validate->setText(QStringLiteral("验证中…"));
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, validate]() {
            validate->setEnabled(true);
            validate->setText(QStringLiteral("验证连通性"));
            const QJsonObject response = watcher->result();
            if (response.value(QStringLiteral("success")).toBool()) UiDialogs::showInformation(this, QStringLiteral("连通性验证"), QStringLiteral("连通性验证通过"));
            else UiDialogs::showWarning(this, QStringLiteral("连通性验证失败"), response.value(QStringLiteral("error")).toString());
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, json]() { return bridge->call("TestModelConnectionFFI", json); }));
    });
    connect(saveButton_, &QPushButton::clicked, this, &SettingsDialog::saveCurrentTab);
    buttons->addWidget(cancel);
    buttons->addWidget(validate);
    buttons->addWidget(saveButton_);
    root->addLayout(buttons);
    loadModelConfig();
}

void SettingsDialog::loadModelConfig() {
    if (bridge_ == nullptr || !bridge_->isReady()) return;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonObject data = settingsUnwrap(watcher->result());
        const int providerIndex = provider_->findData(data.value(QStringLiteral("provider")).toString());
        if (providerIndex >= 0) provider_->setCurrentIndex(providerIndex);
        baseUrl_->setText(data.value(QStringLiteral("endpoint")).toString());
        apiKey_->setText(data.value(QStringLiteral("api_key")).toString());
        modelName_->setText(data.value(QStringLiteral("model")).toString(data.value(QStringLiteral("model_name")).toString()));
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_]() { return bridge->call("GetSecurityModelConfigFFI"); }));
}

void SettingsDialog::saveCurrentTab() {
    if (tabs_->currentIndex() == 0 && bridge_ != nullptr) {
        const QJsonObject payload{{QStringLiteral("provider"), provider_->currentData().toString()}, {QStringLiteral("endpoint"), baseUrl_->text()}, {QStringLiteral("api_key"), apiKey_->text()}, {QStringLiteral("model"), modelName_->text()}};
        const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        saveButton_->setEnabled(false);
        saveButton_->setText(QStringLiteral("保存中…"));
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
            saveButton_->setEnabled(true);
            saveButton_->setText(QStringLiteral("保存"));
            const QJsonObject response = watcher->result();
            if (!response.value(QStringLiteral("success")).toBool()) UiDialogs::showWarning(this, QStringLiteral("保存失败"), response.value(QStringLiteral("error")).toString());
            else accept();
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, json]() { return bridge->call("SaveSecurityModelConfigFFI", json); }));
        return;
    }
    accept();
}
