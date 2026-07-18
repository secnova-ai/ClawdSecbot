#include "ui/Dialogs.h"

#include "bridge/GoBridge.h"
#include "ui/DialogChrome.h"
#include "ui/UiDialogs.h"

#include <QAbstractButton>
#include <QButtonGroup>
#include <QCheckBox>
#include <QClipboard>
#include <QComboBox>
#include <QDialogButtonBox>
#include <QDateTime>
#include <QFormLayout>
#include <QFutureWatcher>
#include <QGridLayout>
#include <QGuiApplication>
#include <QHash>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QIntValidator>
#include <QLabel>
#include <QLineEdit>
#include <QListWidget>
#include <QPlainTextEdit>
#include <QProgressBar>
#include <QPushButton>
#include <QScrollArea>
#include <QSettings>
#include <QStackedWidget>
#include <QTabBar>
#include <QTabWidget>
#include <QTimer>
#include <QVBoxLayout>
#include <QtConcurrent>

namespace {
QLabel* titleLabel(const QString& text, QWidget* parent, const char* name = "dialogTitle") {
    auto* label = new QLabel(text, parent);
    label->setObjectName(QString::fromLatin1(name));
    return label;
}

QWidget* scrollPage(QWidget* content, QWidget* parent) {
    auto* scroll = new QScrollArea(parent);
    scroll->setWidgetResizable(true);
    scroll->setFrameShape(QFrame::NoFrame);
    scroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    scroll->setWidget(content);
    return scroll;
}

QWidget* fieldBlock(const QString& labelText, QWidget* field, QWidget* parent) {
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

QFrame* actionRow(const QString& icon, const QString& title, const QString& description, QWidget* parent) {
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
    textLayout->addWidget(titleLabel(title, text, "sectionTitle"));
    auto* detail = new QLabel(description, text);
    detail->setObjectName(QStringLiteral("subtle"));
    detail->setWordWrap(true);
    textLayout->addWidget(detail);
    layout->addWidget(text, 1);
    return frame;
}

QJsonObject unwrap(const QJsonObject& response) {
    return response.value(QStringLiteral("data")).isObject() ? response.value(QStringLiteral("data")).toObject() : response;
}

QJsonObject embeddedObject(const QJsonValue& value) {
    if (value.isObject()) return value.toObject();
    if (value.isString()) return QJsonDocument::fromJson(value.toString().toUtf8()).object();
    return {};
}

QString joinedValues(const QJsonArray& values) {
    QStringList result;
    for (const QJsonValue& value : values) result.append(value.toString());
    return result.join(QStringLiteral(";"));
}

QJsonArray splitValues(const QString& text) {
    QJsonArray result;
    for (const QString& value : text.split(';', Qt::SkipEmptyParts)) result.append(value.trimmed());
    return result;
}

void selectData(QComboBox* combo, const QString& value) {
    const int index = combo->findData(value);
    if (index >= 0) combo->setCurrentIndex(index);
}

QString mitigationRiskTitle(const RiskModel& risk) {
    static const QHash<QString, QString> titles{
        {QStringLiteral("gateway_bind_unsafe"), QStringLiteral("非回环地址绑定")}, {QStringLiteral("gateway_auth_disabled"), QStringLiteral("未配置认证")},
        {QStringLiteral("gateway_auth_password_mode"), QStringLiteral("网关启用了密码模式")}, {QStringLiteral("gateway_weak_password"), QStringLiteral("认证密码太弱")},
        {QStringLiteral("gateway_weak_token"), QStringLiteral("网关 Token 强度不足")}, {QStringLiteral("config_perm_unsafe"), QStringLiteral("配置文件权限不安全")},
        {QStringLiteral("config_dir_perm_unsafe"), QStringLiteral("配置目录权限不安全")}, {QStringLiteral("sandbox_disabled_default"), QStringLiteral("默认沙箱已禁用")},
        {QStringLiteral("sandbox_disabled_agent"), QStringLiteral("Agent 沙箱已禁用")}, {QStringLiteral("logging_redact_off"), QStringLiteral("敏感数据脱敏已禁用")},
        {QStringLiteral("audit_disabled"), QStringLiteral("安全审计日志已禁用")}, {QStringLiteral("autonomy_workspace_unrestricted"), QStringLiteral("工作区访问范围未限制")},
        {QStringLiteral("log_dir_perm_unsafe"), QStringLiteral("日志目录权限不安全")}, {QStringLiteral("plaintext_secrets"), QStringLiteral("配置文件中发现明文密钥")},
        {QStringLiteral("skills_not_scanned"), QStringLiteral("Skills 未进行提示词注入扫描")}, {QStringLiteral("openclaw_insecure_or_dangerous_flags"), QStringLiteral("OpenClaw 网关危险开关已启用")},
        {QStringLiteral("openclaw_config_patch_outdated"), QStringLiteral("OpenClaw 配置安全补丁缺失")}, {QStringLiteral("terminal_backend_local"), QStringLiteral("终端后端为本地执行")},
        {QStringLiteral("approvals_mode_disabled"), QStringLiteral("审批模式已禁用")}, {QStringLiteral("redact_secrets_disabled"), QStringLiteral("密钥脱敏已禁用")},
        {QStringLiteral("model_base_url_public"), QStringLiteral("自定义模型地址暴露公网")}, {QStringLiteral("process_running_as_root"), QStringLiteral("进程以 root 身份运行")},
        {QStringLiteral("memory_dir_perm_unsafe"), QStringLiteral("memory 目录权限不安全")}, {QStringLiteral("skill_agent_risk"), QStringLiteral("检测到高风险 Skill")},
    };
    return titles.value(risk.id, risk.title);
}

QString mitigationRiskDescription(const RiskModel& risk) {
    const QJsonObject args = risk.raw.value(QStringLiteral("args")).toObject();
    if (risk.id == QStringLiteral("openclaw_insecure_or_dangerous_flags")) {
        QStringList flags;
        for (const QJsonValue& value : args.value(QStringLiteral("flags")).toArray()) flags.append(value.toString());
        return QStringLiteral("OpenClaw 网关开关削弱了认证或来源信任保护：%1。请关闭这些危险开关后重新扫描。")
            .arg(flags.join(QStringLiteral("；")));
    }
    if (risk.id == QStringLiteral("terminal_backend_local")) return QStringLiteral("terminal.backend 为 local，Agent 操作将直接在宿主机执行，缺少远程隔离。");
    if (risk.id == QStringLiteral("model_base_url_public")) {
        return QStringLiteral("model.base_url 指向非本地地址：%1。建议改为本地或受控私网地址。")
            .arg(args.value(QStringLiteral("base_url")).toString());
    }
    return risk.description;
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
    modelLayout->addWidget(fieldBlock(QStringLiteral("模型供应商"), provider_, modelPage));
    modelLayout->addWidget(fieldBlock(QStringLiteral("基础 URL"), baseUrl_, modelPage));
    modelLayout->addWidget(fieldBlock(QStringLiteral("API 密钥"), apiKey_, modelPage));
    modelLayout->addWidget(fieldBlock(QStringLiteral("模型名称"), modelName_, modelPage));
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
    auto* startup = actionRow(QStringLiteral("⏻"), QStringLiteral("开机自启"), QString(), general);
    auto* startupCheck = new QCheckBox(startup);
    startupCheck->setChecked(QSettings{}.value(QStringLiteral("ui/launch_at_startup"), false).toBool());
    connect(startupCheck, &QCheckBox::toggled, this, [](bool checked) { QSettings{}.setValue(QStringLiteral("ui/launch_at_startup"), checked); });
    static_cast<QHBoxLayout*>(startup->layout())->insertWidget(startup->layout()->count() - 1, startupCheck);
    generalLayout->addWidget(startup);
    auto* schedule = actionRow(QStringLiteral("◴"), QStringLiteral("定时扫描设置"), QStringLiteral("按设定间隔自动执行安全扫描"), general);
    auto* scheduleCombo = new QComboBox(schedule);
    scheduleCombo->addItems({QStringLiteral("关闭"), QStringLiteral("30 分钟"), QStringLiteral("1 小时"), QStringLiteral("6 小时"), QStringLiteral("24 小时")});
    connect(scheduleCombo, &QComboBox::currentIndexChanged, this, [this](int index) {
        if (bridge_ == nullptr) return;
        const QList<int> seconds{0, 1800, 3600, 21600, 86400};
        const QJsonObject payload{{QStringLiteral("key"), QStringLiteral("scheduled_scan_interval_seconds")}, {QStringLiteral("value"), seconds.value(index)}};
        const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
        [[maybe_unused]] const QFuture<void> saveFuture = QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, json]() { bridge->call("SaveAppSettingFFI", json); });
    });
    if (bridge_ != nullptr && bridge_->isReady()) {
        auto* scheduleWatcher = new QFutureWatcher<QJsonObject>(this);
        connect(scheduleWatcher, &QFutureWatcher<QJsonObject>::finished, this, [scheduleWatcher, scheduleCombo]() {
            const QJsonObject response = scheduleWatcher->result();
            const QJsonValue raw = response.value(QStringLiteral("data"));
            const int seconds = raw.isObject() ? raw.toObject().value(QStringLiteral("value")).toVariant().toInt() : raw.toVariant().toInt();
            const QList<int> intervals{0, 1800, 3600, 21600, 86400};
            const int index = intervals.indexOf(seconds);
            scheduleCombo->blockSignals(true);
            scheduleCombo->setCurrentIndex(index >= 0 ? index : 0);
            scheduleCombo->blockSignals(false);
            scheduleWatcher->deleteLater();
        });
        scheduleWatcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_]() { return bridge->call("GetAppSettingFFI", QStringLiteral("scheduled_scan_interval_seconds")); }));
    }
    static_cast<QHBoxLayout*>(schedule->layout())->insertWidget(schedule->layout()->count() - 1, scheduleCombo);
    generalLayout->addWidget(schedule);
    auto* api = actionRow(QStringLiteral("▤"), QStringLiteral("API 服务"), QString(), general);
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
            } else {
                const QJsonObject payload{{QStringLiteral("key"), QStringLiteral("api_server_enabled")}, {QStringLiteral("value"), checked}};
                const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
                [[maybe_unused]] const QFuture<void> saveFuture = QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, json]() { bridge->call("SaveAppSettingFFI", json); });
            }
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, checked]() { return checked ? bridge->call("StartAPIServerFFI", QStringLiteral("{\"port\":0}")) : bridge->call("StopAPIServerFFI"); }));
    });
    if (bridge_ != nullptr && bridge_->isReady()) {
        auto* apiWatcher = new QFutureWatcher<QJsonObject>(this);
        connect(apiWatcher, &QFutureWatcher<QJsonObject>::finished, this, [apiWatcher, apiCheck]() {
            const QJsonValue raw = apiWatcher->result().value(QStringLiteral("data"));
            const QJsonValue value = raw.isObject() ? raw.toObject().value(QStringLiteral("value")) : raw;
            apiCheck->blockSignals(true);
            apiCheck->setChecked(value.toBool(false));
            apiCheck->blockSignals(false);
            apiWatcher->deleteLater();
        });
        apiWatcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_]() { return bridge->call("GetAppSettingFFI", QStringLiteral("api_server_enabled")); }));
    }
    static_cast<QHBoxLayout*>(api->layout())->insertWidget(api->layout()->count() - 1, apiCheck);
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
    auto* about = new QPushButton(QStringLiteral("ⓘ  关于 ClawdSecbot\n     版本 1.0.4 / Qt 6"), general);
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
    connect(about, &QPushButton::clicked, this, [this]() { UiDialogs::showAbout(this, QStringLiteral("关于 ClawdSecbot"), QStringLiteral("ClawdSecbot 1.0.4\nQt 6 桌面客户端\nGo 安全业务引擎")); });
    generalLayout->addStretch();
    tabs_->addTab(scrollPage(general, tabs_), QStringLiteral("☷  通用设置"));
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
        const QJsonObject data = unwrap(watcher->result());
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

ProtectionConfigDialog::ProtectionConfigDialog(const AssetModel& asset, GoBridge* bridge, QWidget* parent)
    : QDialog(parent), asset_(asset), bridge_(bridge) {
    setWindowTitle(QStringLiteral("开启防护"));
    setProperty("tone", DialogChrome::toneName(DialogChrome::Tone::Success));
    DialogChrome::prepare(this, QSize(580, 720), QSize(550, 660));
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(26, 24, 26, 22);
    root->setSpacing(18);
    root->addWidget(DialogChrome::createHeader(this, QStringLiteral("◇"), QStringLiteral("开启防护"),
                                                QStringLiteral("为 %1 配置实时安全策略").arg(asset_.name),
                                                DialogChrome::Tone::Success));
    tabs_ = new QTabWidget(this);
    tabs_->setObjectName(QStringLiteral("segmentedTabs"));
    tabs_->tabBar()->setObjectName(QStringLiteral("protectionTabBar"));
    tabs_->tabBar()->setExpanding(true);
    tabs_->tabBar()->setDrawBase(false);

    auto makePage = [this](const QString& intro) {
        auto* page = new QWidget(tabs_);
        auto* layout = new QVBoxLayout(page);
        layout->setContentsMargins(8, 18, 8, 8);
        layout->setSpacing(14);
        auto* label = new QLabel(intro, page);
        label->setWordWrap(true);
        label->setObjectName(QStringLiteral("muted"));
        layout->addWidget(label);
        return qMakePair(page, layout);
    };
    auto* promptContent = new QWidget(tabs_);
    auto* promptLayout = new QVBoxLayout(promptContent);
    promptLayout->setContentsMargins(8, 14, 8, 8);
    promptLayout->setSpacing(12);
    auto buildFeatureCard = [promptContent, promptLayout](const QString& icon, const QString& title, const QString& detail, const char* objectName) {
        auto* card = new QFrame(promptContent);
        card->setObjectName(QString::fromLatin1(objectName));
        auto* cardLayout = new QHBoxLayout(card);
        cardLayout->setContentsMargins(14, 12, 14, 12);
        cardLayout->setSpacing(10);
        auto* glyph = new QLabel(icon, card);
        glyph->setObjectName(QStringLiteral("featureIcon"));
        glyph->setAlignment(Qt::AlignCenter);
        glyph->setFixedSize(36, 36);
        cardLayout->addWidget(glyph);
        auto* copy = new QWidget(card);
        auto* copyLayout = new QVBoxLayout(copy);
        copyLayout->setContentsMargins(0, 0, 0, 0);
        copyLayout->setSpacing(3);
        copyLayout->addWidget(titleLabel(title, copy, "sectionTitle"));
        auto* description = titleLabel(detail, copy, "subtle");
        description->setWordWrap(true);
        copyLayout->addWidget(description);
        cardLayout->addWidget(copy, 1);
        auto* toggle = new QCheckBox(card);
        toggle->setObjectName(QStringLiteral("switchControl"));
        cardLayout->addWidget(toggle);
        promptLayout->addWidget(card);
        return toggle;
    };
    auditOnly_ = buildFeatureCard(QStringLiteral("◇"), QStringLiteral("仅审计模式"),
                                  QStringLiteral("仅记录请求与响应，不进行风险研判和拦截"), "featureCard");
    userInputDetection_ = buildFeatureCard(QStringLiteral("✓"), QStringLiteral("用户输入检测"),
                                           QStringLiteral("检测用户输入中的敏感操作与提示词攻击"), "featureCardActive");
    userInputDetection_->setChecked(true);

    auto* customCard = new QFrame(promptContent);
    customCard->setObjectName(QStringLiteral("ruleEditorCard"));
    auto* customLayout = new QVBoxLayout(customCard);
    customLayout->setContentsMargins(14, 12, 14, 12);
    customLayout->setSpacing(7);
    customLayout->addWidget(titleLabel(QStringLiteral("用户自定义规则"), customCard, "sectionTitle"));
    customLayout->addWidget(titleLabel(QStringLiteral("添加需要人工确认的敏感操作规则"), customCard, "subtle"));
    auto* ruleInputRow = new QHBoxLayout;
    newRule_ = new QLineEdit(customCard);
    newRule_->setPlaceholderText(QStringLiteral("输入需要确认的敏感操作，例如：删除生产数据"));
    auto* addRule = new QPushButton(QStringLiteral("＋"), customCard);
    addRule->setObjectName(QStringLiteral("ruleAddButton"));
    addRule->setFixedSize(40, 40);
    ruleInputRow->addWidget(newRule_, 1);
    ruleInputRow->addWidget(addRule);
    customLayout->addLayout(ruleInputRow);
    promptLayout->addWidget(customCard);
    promptLayout->addWidget(titleLabel(QStringLiteral("内置规则"), promptContent, "sectionTitle"));
    rulesContainer_ = new QWidget(promptContent);
    rulesLayout_ = new QVBoxLayout(rulesContainer_);
    rulesLayout_->setContentsMargins(0, 0, 0, 0);
    rulesLayout_->setSpacing(8);
    promptLayout->addWidget(rulesContainer_);
    promptLayout->addStretch();
    connect(addRule, &QPushButton::clicked, this, [this]() {
        const QString description = newRule_->text().trimmed();
        if (description.isEmpty()) return;
        const QJsonObject rule{{QStringLiteral("id"), QStringLiteral("custom_%1").arg(QDateTime::currentMSecsSinceEpoch())},
                               {QStringLiteral("scope"), QStringLiteral("custom")}, {QStringLiteral("enabled"), true},
                               {QStringLiteral("description"), description},
                               {QStringLiteral("applies_to"), QJsonArray{QStringLiteral("user_input"), QStringLiteral("tool_call"), QStringLiteral("tool_call_result"), QStringLiteral("final_result")}},
                               {QStringLiteral("action"), QStringLiteral("needs_confirmation")}, {QStringLiteral("risk_type"), QStringLiteral("HIGH_RISK_OPERATION")}};
        shepherdRules_.append(rule);
        appendRuleCard(rule);
        newRule_->clear();
    });
    tabs_->addTab(scrollPage(promptContent, tabs_), QStringLiteral("◐  智能规则"));

    auto* quotaContent = new QWidget(tabs_);
    auto* quotaLayout = new QVBoxLayout(quotaContent);
    quotaLayout->setContentsMargins(8, 18, 8, 8);
    quotaLayout->setSpacing(12);
    quotaLayout->addWidget(titleLabel(QStringLiteral("◉  Token使用限制"), quotaContent, "sectionTitle"));
    auto* quotaIntro = titleLabel(QStringLiteral("限制单轮会话和当日累计 Token，避免异常消耗。"), quotaContent, "muted");
    quotaIntro->setWordWrap(true);
    quotaLayout->addWidget(quotaIntro);

    auto buildPresets = [quotaContent](QVBoxLayout* layout, const QString& title, const QList<QPair<QString, int>>& options, QLineEdit** output) {
        layout->addWidget(titleLabel(title, quotaContent, "sectionTitle"));
        auto* group = new QButtonGroup(quotaContent);
        group->setExclusive(true);
        auto* row = new QHBoxLayout;
        row->setContentsMargins(0, 0, 0, 0);
        row->setSpacing(6);
        for (const auto& option : options) {
            auto* preset = new QPushButton(option.first, quotaContent);
            preset->setObjectName(QStringLiteral("presetButton"));
            preset->setCheckable(true);
            preset->setProperty("tokenValue", option.second);
            group->addButton(preset);
            row->addWidget(preset);
        }
        layout->addLayout(row);
        auto* input = new QLineEdit(quotaContent);
        input->setValidator(new QIntValidator(0, 100000000, input));
        input->setPlaceholderText(QStringLiteral("输入 Token 上限，0 表示不限制"));
        layout->addWidget(input);
        connect(group, &QButtonGroup::buttonClicked, input, [input](QAbstractButton* button) {
            input->setText(QString::number(button->property("tokenValue").toInt()));
        });
        connect(input, &QLineEdit::textChanged, group, [group](const QString& text) {
            const int current = text.toInt();
            for (QAbstractButton* button : group->buttons()) button->setChecked(button->property("tokenValue").toInt() == current);
        });
        *output = input;
    };
    buildPresets(quotaLayout, QStringLiteral("单轮会话Token上限"),
                 {{QStringLiteral("不限制"), 0}, {QStringLiteral("5万"), 50000}, {QStringLiteral("10万"), 100000},
                  {QStringLiteral("30万"), 300000}, {QStringLiteral("50万"), 500000}, {QStringLiteral("100万"), 1000000}}, &tokenLimit_);
    buildPresets(quotaLayout, QStringLiteral("当日总Token上限"),
                 {{QStringLiteral("不限制"), 0}, {QStringLiteral("1000万"), 10000000},
                  {QStringLiteral("5000万"), 50000000}, {QStringLiteral("1亿"), 100000000}}, &dailyTokenLimit_);
    auto* quotaHint = titleLabel(QStringLiteral("ⓘ  达到上限后，新请求会被防护代理拒绝；设置为 0 表示不限制。"), quotaContent, "quotaHint");
    quotaHint->setWordWrap(true);
    quotaLayout->addWidget(quotaHint);
    quotaLayout->addStretch();
    tabs_->addTab(quotaContent, QStringLiteral("◉  Token限制"));

    auto* permissionContent = new QWidget(tabs_);
    auto* permissionLayout = new QVBoxLayout(permissionContent);
    permissionLayout->setContentsMargins(8, 18, 8, 8);
    permissionLayout->setSpacing(12);
    auto* sandboxCard = new QFrame(permissionContent);
    sandboxCard->setObjectName(QStringLiteral("featureCard"));
    auto* sandboxLayout = new QHBoxLayout(sandboxCard);
    sandboxLayout->setContentsMargins(14, 12, 14, 12);
    auto* sandboxIcon = titleLabel(QStringLiteral("◇"), sandboxCard, "featureIcon");
    sandboxIcon->setAlignment(Qt::AlignCenter);
    sandboxIcon->setFixedSize(36, 36);
    sandboxLayout->addWidget(sandboxIcon);
    auto* sandboxCopy = new QWidget(sandboxCard);
    auto* sandboxCopyLayout = new QVBoxLayout(sandboxCopy);
    sandboxCopyLayout->setContentsMargins(0, 0, 0, 0);
    sandboxCopyLayout->setSpacing(3);
    sandboxCopyLayout->addWidget(titleLabel(QStringLiteral("沙箱防护"), sandboxCopy, "sectionTitle"));
    auto* sandboxDescription = titleLabel(QStringLiteral("限制网关进程的系统资源访问，强制执行权限设置规则"), sandboxCopy, "subtle");
    sandboxDescription->setWordWrap(true);
    sandboxCopyLayout->addWidget(sandboxDescription);
    sandboxLayout->addWidget(sandboxCopy, 1);
    sandboxEnabled_ = new QCheckBox(sandboxCard);
    sandboxEnabled_->setObjectName(QStringLiteral("switchControl"));
    sandboxLayout->addWidget(sandboxEnabled_);
    permissionLayout->addWidget(sandboxCard);

    auto addModeItems = [](QComboBox* combo) {
        combo->addItem(QStringLiteral("黑名单：禁止列表中的项目"), QStringLiteral("blacklist"));
        combo->addItem(QStringLiteral("白名单：仅允许列表中的项目"), QStringLiteral("whitelist"));
    };
    auto buildModeSelector = [permissionContent, addModeItems](QVBoxLayout* target, QComboBox** comboOutput) {
        auto* combo = new QComboBox(permissionContent);
        addModeItems(combo);
        combo->hide();
        auto* buttons = new QButtonGroup(permissionContent);
        buttons->setExclusive(true);
        auto* modeRow = new QHBoxLayout;
        modeRow->setSpacing(6);
        for (const auto& option : QList<QPair<QString, QString>>{{QStringLiteral("黑名单"), QStringLiteral("blacklist")}, {QStringLiteral("白名单"), QStringLiteral("whitelist")}}) {
            auto* button = new QPushButton(option.first, permissionContent);
            button->setObjectName(QStringLiteral("modeButton"));
            button->setCheckable(true);
            button->setProperty("modeValue", option.second);
            button->setChecked(option.second == QStringLiteral("blacklist"));
            buttons->addButton(button);
            modeRow->addWidget(button);
        }
        modeRow->addStretch();
        target->addLayout(modeRow);
        connect(buttons, &QButtonGroup::buttonClicked, combo, [combo](QAbstractButton* button) {
            const int index = combo->findData(button->property("modeValue").toString());
            if (index >= 0) combo->setCurrentIndex(index);
        });
        connect(combo, &QComboBox::currentIndexChanged, buttons, [combo, buttons]() {
            const QString current = combo->currentData().toString();
            for (QAbstractButton* button : buttons->buttons()) button->setChecked(button->property("modeValue").toString() == current);
        });
        combo->setCurrentIndex(0);
        *comboOutput = combo;
    };
    auto buildPermissionCard = [permissionContent, permissionLayout](const QString& icon, const QString& title, const QString& detail) {
        auto* card = new QFrame(permissionContent);
        card->setObjectName(QStringLiteral("permissionCard"));
        auto* layout = new QVBoxLayout(card);
        layout->setContentsMargins(14, 12, 14, 12);
        layout->setSpacing(8);
        layout->addWidget(titleLabel(QStringLiteral("%1  %2").arg(icon, title), card, "sectionTitle"));
        auto* description = titleLabel(detail, card, "subtle");
        description->setWordWrap(true);
        layout->addWidget(description);
        permissionLayout->addWidget(card);
        return qMakePair(card, layout);
    };

    auto pathCard = buildPermissionCard(QStringLiteral("□"), QStringLiteral("路径访问权限"), QStringLiteral("配置允许或禁止被代理的智能体访问的文件路径"));
    buildModeSelector(pathCard.second, &pathMode_);
    paths_ = new QLineEdit(pathCard.first);
    paths_->setPlaceholderText(QStringLiteral("每项以分号分隔，例如 /tmp;/Users/me/work"));
    pathCard.second->addWidget(paths_);

    auto networkCard = buildPermissionCard(QStringLiteral("◎"), QStringLiteral("网络访问权限"), QStringLiteral("配置允许或禁止访问的网段、域名"));
    networkCard.second->addWidget(titleLabel(QStringLiteral("↗  出栈 (Outbound)"), networkCard.first, "sectionTitle"));
    buildModeSelector(networkCard.second, &outboundMode_);
    outboundAddresses_ = new QLineEdit(networkCard.first);
    outboundAddresses_->setPlaceholderText(QStringLiteral("例如 192.168.1.0/24;*.internal.com"));
    networkCard.second->addWidget(outboundAddresses_);
    networkCard.second->addWidget(titleLabel(QStringLiteral("↙  入栈 (Inbound)"), networkCard.first, "sectionTitle"));
    buildModeSelector(networkCard.second, &inboundMode_);
    inboundAddresses_ = new QLineEdit(networkCard.first);
    inboundAddresses_->setPlaceholderText(QStringLiteral("每项以分号分隔，例如 localhost:8080"));
    networkCard.second->addWidget(inboundAddresses_);

    auto shellCard = buildPermissionCard(QStringLiteral(">_"), QStringLiteral("Shell命令权限"), QStringLiteral("配置允许或禁止执行的 Shell 命令"));
    buildModeSelector(shellCard.second, &shellMode_);
    shellCommands_ = new QLineEdit(shellCard.first);
    shellCommands_->setPlaceholderText(QStringLiteral("每项以分号分隔，例如 git status;ls"));
    shellCard.second->addWidget(shellCommands_);
    auto* permissionHint = titleLabel(QStringLiteral("注意：权限设置需要启用沙箱防护才能生效。启用后，网关进程将在受限环境中运行。"), permissionContent, "quotaHint");
    permissionHint->setWordWrap(true);
    permissionLayout->addWidget(permissionHint);
    permissionLayout->addStretch();
    tabs_->addTab(scrollPage(permissionContent, tabs_), QStringLiteral("◇  权限设置"));

    auto botPage = makePage(QStringLiteral("登记 Bot 实际使用的模型，防护代理会按此配置转发。"));
    botProvider_ = new QComboBox(botPage.first);
    botProvider_->addItem(QStringLiteral("OpenAI 兼容"), QStringLiteral("openai"));
    botProvider_->addItem(QStringLiteral("Anthropic"), QStringLiteral("anthropic"));
    botProvider_->addItem(QStringLiteral("Google Gemini"), QStringLiteral("google"));
    botProvider_->addItem(QStringLiteral("MiniMax"), QStringLiteral("minimax"));
    botProvider_->addItem(QStringLiteral("DeepSeek"), QStringLiteral("deepseek"));
    botProvider_->addItem(QStringLiteral("Ollama"), QStringLiteral("ollama"));
    botProvider_->addItem(QStringLiteral("自定义"), QStringLiteral("custom"));
    botBaseUrl_ = new QLineEdit(botPage.first);
    botApiKey_ = new QLineEdit(botPage.first);
    botApiKey_->setEchoMode(QLineEdit::Password);
    botModel_ = new QLineEdit(botPage.first);
    botSecretKey_ = new QLineEdit(botPage.first);
    botSecretKey_->setEchoMode(QLineEdit::Password);
    botPage.second->addWidget(fieldBlock(QStringLiteral("模型供应商"), botProvider_, botPage.first));
    botPage.second->addWidget(fieldBlock(QStringLiteral("基础 URL"), botBaseUrl_, botPage.first));
    botPage.second->addWidget(fieldBlock(QStringLiteral("API 密钥"), botApiKey_, botPage.first));
    botPage.second->addWidget(fieldBlock(QStringLiteral("Bot 模型"), botModel_, botPage.first));
    botSecretKey_->hide();
    auto* botHint = titleLabel(QStringLiteral("ⓘ  防护代理会使用此配置转发 Bot 的模型请求。"), botPage.first, "quotaHint");
    botHint->setWordWrap(true);
    botPage.second->addWidget(botHint);
    auto* validateBot = new QPushButton(QStringLiteral("验证连通性"), botPage.first);
    validateBot->setObjectName(QStringLiteral("secondaryButton"));
    botPage.second->addWidget(validateBot, 0, Qt::AlignRight);
    connect(validateBot, &QPushButton::clicked, this, [this, validateBot]() {
        if (bridge_ == nullptr) return;
        const QJsonObject payload{{QStringLiteral("provider"), botProvider_->currentData().toString()},
                                  {QStringLiteral("endpoint"), botBaseUrl_->text().trimmed()},
                                  {QStringLiteral("api_key"), botApiKey_->text().trimmed()},
                                  {QStringLiteral("model"), botModel_->text().trimmed()}};
        const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
        validateBot->setEnabled(false);
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, validateBot]() {
            validateBot->setEnabled(true);
            const QJsonObject result = watcher->result();
            if (result.value(QStringLiteral("success")).toBool()) UiDialogs::showInformation(this, QStringLiteral("连通性验证"), QStringLiteral("连通性验证通过"));
            else UiDialogs::showWarning(this, QStringLiteral("连通性验证失败"), result.value(QStringLiteral("error")).toString());
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, json]() { return bridge->call("TestModelConnectionFFI", json); }));
    });
    botPage.second->addStretch();
    tabs_->addTab(botPage.first, QStringLiteral("▣  Bot模型"));
    root->addWidget(tabs_, 1);

    auto* buttons = new QDialogButtonBox(QDialogButtonBox::Cancel | QDialogButtonBox::Save, this);
    DialogChrome::styleButtonBox(buttons);
    buttons->button(QDialogButtonBox::Cancel)->setText(QStringLiteral("取消"));
    saveButton_ = buttons->button(QDialogButtonBox::Save);
    saveButton_->setText(QStringLiteral("确认开启"));
    saveButton_->setObjectName(QStringLiteral("primaryButton"));
    connect(buttons, &QDialogButtonBox::rejected, this, &QDialog::reject);
    connect(buttons, &QDialogButtonBox::accepted, this, &ProtectionConfigDialog::saveConfig);
    root->addWidget(buttons);
    loadConfig();
}

QString ProtectionConfigDialog::sessionId() const { return sessionId_; }

void ProtectionConfigDialog::appendRuleCard(const QJsonObject& rule) {
    auto* card = new QFrame(rulesContainer_);
    card->setObjectName(QStringLiteral("ruleCard"));
    auto* layout = new QHBoxLayout(card);
    layout->setContentsMargins(14, 12, 14, 12);
    layout->setSpacing(10);
    auto* copy = new QWidget(card);
    auto* copyLayout = new QVBoxLayout(copy);
    copyLayout->setContentsMargins(0, 0, 0, 0);
    copyLayout->setSpacing(6);
    const QString scope = rule.value(QStringLiteral("scope")).toString();
    const QString rawDescription = rule.value(QStringLiteral("description")).toString();
    QString ruleTitle = scope == QStringLiteral("custom") ? QStringLiteral("自定义规则") : QStringLiteral("安全操作规则");
    QString ruleDescription = rawDescription;
    if (rawDescription.startsWith(QStringLiteral("Writing to or modifying critical system files"))) {
        ruleTitle = QStringLiteral("关键文件修改");
        ruleDescription = QStringLiteral("修改关键系统文件或项目工作区之外的文件时要求用户确认；项目工作区内的正常编辑不受影响。");
    } else if (rawDescription.startsWith(QStringLiteral("Sending emails, messages"))) {
        ruleTitle = QStringLiteral("外部消息发送");
        ruleDescription = QStringLiteral("向外部联系人发送邮件、消息或通知前要求用户确认。");
    } else if (rawDescription.startsWith(QStringLiteral("Executing dangerous shell commands"))) {
        ruleTitle = QStringLiteral("危险命令执行");
        ruleDescription = QStringLiteral("执行删除、提权、权限修改、服务管理或持久化相关的危险命令前要求用户确认。");
    } else if (rawDescription.startsWith(QStringLiteral("Changing global system settings"))) {
        ruleTitle = QStringLiteral("系统配置变更");
        ruleDescription = QStringLiteral("修改全局系统设置、启动项、Shell 配置或服务配置前要求用户确认。");
    } else if (rawDescription.startsWith(QStringLiteral("Payment, purchase, subscription"))) {
        ruleTitle = QStringLiteral("支付与订阅操作");
        ruleDescription = QStringLiteral("付款、购买、订阅、账单或转账操作必须由用户明确确认。");
    }
    copyLayout->addWidget(titleLabel(ruleTitle, copy, "sectionTitle"));
    auto* description = titleLabel(ruleDescription, copy, "muted");
    description->setWordWrap(true);
    copyLayout->addWidget(description);
    auto* badges = new QHBoxLayout;
    badges->setContentsMargins(0, 0, 0, 0);
    badges->setSpacing(6);
    auto appendBadge = [badges, copy](const QString& text) {
        if (text.isEmpty()) return;
        auto* badge = titleLabel(text, copy, "ruleBadge");
        badges->addWidget(badge);
    };
    appendBadge(scope == QStringLiteral("custom") ? QStringLiteral("自定义") : QStringLiteral("内置"));
    appendBadge(rule.value(QStringLiteral("action")).toString() == QStringLiteral("needs_confirmation") ? QStringLiteral("需要确认") : QStringLiteral("安全检查"));
    const QString riskType = rule.value(QStringLiteral("risk_type")).toString();
    static const QHash<QString, QString> riskTypeLabels{
        {QStringLiteral("HIGH_RISK_OPERATION"), QStringLiteral("高风险操作")},
        {QStringLiteral("SENSITIVE_DATA_EXFILTRATION"), QStringLiteral("敏感数据外发")},
        {QStringLiteral("UNEXPECTED_CODE_EXECUTION"), QStringLiteral("意外代码执行")},
    };
    appendBadge(riskTypeLabels.value(riskType, riskType));
    badges->addStretch();
    copyLayout->addLayout(badges);
    layout->addWidget(copy, 1);
    auto* toggle = new QCheckBox(card);
    toggle->setObjectName(QStringLiteral("switchControl"));
    toggle->setChecked(rule.value(QStringLiteral("enabled")).toBool(true));
    layout->addWidget(toggle, 0, Qt::AlignTop);
    ruleChecks_.append(toggle);
    rulesLayout_->addWidget(card);
}

void ProtectionConfigDialog::loadConfig() {
    if (bridge_ == nullptr || asset_.id.isEmpty()) return;
    tabs_->setEnabled(false);
    saveButton_->setEnabled(false);
    saveButton_->setText(QStringLiteral("加载配置中…"));
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonObject responses = watcher->result();
        const QJsonObject data = unwrap(responses.value(QStringLiteral("config")).toObject());
        const QJsonObject shepherdData = unwrap(responses.value(QStringLiteral("rules")).toObject());
        const QJsonArray providers = responses.value(QStringLiteral("providers")).toObject().value(QStringLiteral("data")).toArray();
        if (!providers.isEmpty()) {
            botProvider_->clear();
            for (const QJsonValue& value : providers) {
                const QJsonObject provider = value.toObject();
                botProvider_->addItem(provider.value(QStringLiteral("display_name")).toString(provider.value(QStringLiteral("name")).toString()),
                                      provider.value(QStringLiteral("name")).toString());
            }
        }
        shepherdRules_ = shepherdData.value(QStringLiteral("semantic_rules")).toArray();
        while (QLayoutItem* item = rulesLayout_->takeAt(0)) {
            if (item->widget() != nullptr) item->widget()->deleteLater();
            delete item;
        }
        ruleChecks_.clear();
        for (const QJsonValue& value : shepherdRules_) {
            appendRuleCard(value.toObject());
        }
        tokenLimit_->setText(QString::number(data.value(QStringLiteral("single_session_token_limit")).toInt()));
        dailyTokenLimit_->setText(QString::number(data.value(QStringLiteral("daily_token_limit")).toInt()));
        auditOnly_->setChecked(data.value(QStringLiteral("audit_only")).toBool());
        userInputDetection_->setChecked(data.value(QStringLiteral("user_input_detection_enabled")).toBool(true));
        sandboxEnabled_->setChecked(data.value(QStringLiteral("sandbox_enabled")).toBool());
        const QJsonObject pathPermission = embeddedObject(data.value(QStringLiteral("path_permission")));
        const QJsonObject networkPermission = embeddedObject(data.value(QStringLiteral("network_permission")));
        const QJsonObject inbound = networkPermission.value(QStringLiteral("inbound")).toObject();
        const QJsonObject outbound = networkPermission.value(QStringLiteral("outbound")).toObject();
        const QJsonObject shellPermission = embeddedObject(data.value(QStringLiteral("shell_permission")));
        selectData(pathMode_, pathPermission.value(QStringLiteral("mode")).toString(QStringLiteral("blacklist")));
        paths_->setText(joinedValues(pathPermission.value(QStringLiteral("paths")).toArray()));
        selectData(inboundMode_, inbound.value(QStringLiteral("mode")).toString(QStringLiteral("blacklist")));
        inboundAddresses_->setText(joinedValues(inbound.value(QStringLiteral("addresses")).toArray()));
        selectData(outboundMode_, outbound.value(QStringLiteral("mode")).toString(QStringLiteral("blacklist")));
        outboundAddresses_->setText(joinedValues(outbound.value(QStringLiteral("addresses")).toArray()));
        selectData(shellMode_, shellPermission.value(QStringLiteral("mode")).toString(QStringLiteral("blacklist")));
        shellCommands_->setText(joinedValues(shellPermission.value(QStringLiteral("commands")).toArray()));
        const QJsonObject botConfig = embeddedObject(data.value(QStringLiteral("bot_model_config")));
        selectData(botProvider_, botConfig.value(QStringLiteral("provider")).toString(QStringLiteral("openai")));
        botBaseUrl_->setText(botConfig.value(QStringLiteral("base_url")).toString());
        botApiKey_->setText(botConfig.value(QStringLiteral("api_key")).toString());
        botModel_->setText(botConfig.value(QStringLiteral("model")).toString());
        botSecretKey_->setText(botConfig.value(QStringLiteral("secret_key")).toString());
        tabs_->setEnabled(true);
        saveButton_->setEnabled(true);
        saveButton_->setText(QStringLiteral("确认开启"));
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, id = asset_.id]() {
        return QJsonObject{{QStringLiteral("config"), bridge->call("GetProtectionConfigFFI", id)},
                           {QStringLiteral("rules"), bridge->call("GetShepherdRulesFFI", id)},
                           {QStringLiteral("providers"), bridge->call("GetSupportedProviders", QStringLiteral("bot"))}};
    }));
}

void ProtectionConfigDialog::saveConfig() {
    if (bridge_ == nullptr) return accept();
    if (botProvider_->currentData().toString().isEmpty() || botBaseUrl_->text().trimmed().isEmpty() || botModel_->text().trimmed().isEmpty()) {
        UiDialogs::showWarning(this, QStringLiteral("Bot 模型未配置"), QStringLiteral("请填写模型供应商、基础 URL 和 Bot 模型。"));
        tabs_->setCurrentIndex(3);
        return;
    }
    const QJsonObject pathPermission{{QStringLiteral("mode"), pathMode_->currentData().toString()}, {QStringLiteral("paths"), splitValues(paths_->text())}};
    const QJsonObject inboundRule{{QStringLiteral("mode"), inboundMode_->currentData().toString()}, {QStringLiteral("addresses"), splitValues(inboundAddresses_->text())}};
    const QJsonObject outboundRule{{QStringLiteral("mode"), outboundMode_->currentData().toString()}, {QStringLiteral("addresses"), splitValues(outboundAddresses_->text())}};
    const QJsonObject networkPermission{{QStringLiteral("inbound"), inboundRule}, {QStringLiteral("outbound"), outboundRule}};
    const QJsonObject shellPermission{{QStringLiteral("mode"), shellMode_->currentData().toString()}, {QStringLiteral("commands"), splitValues(shellCommands_->text())}};
    const QJsonObject botConfig{{QStringLiteral("provider"), botProvider_->currentData().toString()},
                                {QStringLiteral("base_url"), botBaseUrl_->text().trimmed()},
                                {QStringLiteral("api_key"), botApiKey_->text().trimmed()},
                                {QStringLiteral("model"), botModel_->text().trimmed()},
                                {QStringLiteral("secret_key"), botSecretKey_->text().trimmed()}};
    const QJsonObject payload{{QStringLiteral("asset_id"), asset_.id},
                              {QStringLiteral("inherits_default_policy"), false}, {QStringLiteral("enabled"), true},
                              {QStringLiteral("audit_only"), auditOnly_->isChecked()}, {QStringLiteral("sandbox_enabled"), sandboxEnabled_->isChecked()},
                              {QStringLiteral("user_input_detection_enabled"), userInputDetection_->isChecked()},
                              {QStringLiteral("single_session_token_limit"), tokenLimit_->text().toInt()}, {QStringLiteral("daily_token_limit"), dailyTokenLimit_->text().toInt()},
                              {QStringLiteral("path_permission"), QString::fromUtf8(QJsonDocument(pathPermission).toJson(QJsonDocument::Compact))},
                              {QStringLiteral("network_permission"), QString::fromUtf8(QJsonDocument(networkPermission).toJson(QJsonDocument::Compact))},
                              {QStringLiteral("shell_permission"), QString::fromUtf8(QJsonDocument(shellPermission).toJson(QJsonDocument::Compact))},
                              {QStringLiteral("bot_model_config"), botConfig}};
    const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
    QJsonArray rules;
    for (int index = 0; index < shepherdRules_.size(); ++index) {
        QJsonObject rule = shepherdRules_.at(index).toObject();
        rule.insert(QStringLiteral("enabled"), index < ruleChecks_.size() && ruleChecks_.at(index)->isChecked());
        rules.append(rule);
    }
    const QJsonObject rulesPayload{{QStringLiteral("asset_id"), asset_.id}, {QStringLiteral("semantic_rules"), rules}};
    const QString rulesJson = QString::fromUtf8(QJsonDocument(rulesPayload).toJson(QJsonDocument::Compact));
    saveButton_->setEnabled(false);
    saveButton_->setText(QStringLiteral("启动中…"));
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        saveButton_->setEnabled(true);
        saveButton_->setText(QStringLiteral("确认开启"));
        const QJsonObject result = watcher->result();
        if (!result.value(QStringLiteral("success")).toBool()) UiDialogs::showWarning(this, QStringLiteral("防护启动失败"), result.value(QStringLiteral("error")).toString());
        else {
            sessionId_ = result.value(QStringLiteral("session_id")).toString();
            accept();
        }
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, configJson = json, rulesJson, id = asset_.id, botConfig,
                                         auditOnly = auditOnly_->isChecked(), userInputDetection = userInputDetection_->isChecked(),
                                         singleTokenLimit = tokenLimit_->text().toInt(), dailyTokenLimit = dailyTokenLimit_->text().toInt()]() {
        QJsonObject result = bridge->call("SaveProtectionConfigFFI", configJson);
        if (!result.value(QStringLiteral("success")).toBool()) return result;
        result = bridge->call("SaveShepherdRulesFFI", rulesJson);
        if (!result.value(QStringLiteral("success")).toBool()) return result;
        const QJsonObject securityModel = unwrap(bridge->call("GetSecurityModelConfigFFI"));
        if (securityModel.value(QStringLiteral("provider")).toString().isEmpty() || securityModel.value(QStringLiteral("model")).toString().isEmpty()) {
            return QJsonObject{{QStringLiteral("success"), false},
                               {QStringLiteral("error"), QStringLiteral("请先在全局设置中配置安全模型。")}};
        }
        const QJsonObject runtime{{QStringLiteral("audit_only"), auditOnly},
                                  {QStringLiteral("single_session_token_limit"), singleTokenLimit},
                                  {QStringLiteral("daily_token_limit"), dailyTokenLimit},
                                  {QStringLiteral("user_input_detection_enabled"), userInputDetection}};
        const QJsonObject proxyConfig{{QStringLiteral("asset_id"), id},
                                      {QStringLiteral("security_model"), securityModel}, {QStringLiteral("bot_model"), botConfig},
                                      {QStringLiteral("runtime"), runtime}};
        return bridge->call("StartProtectionProxy", QString::fromUtf8(QJsonDocument(proxyConfig).toJson(QJsonDocument::Compact)));
    }));
}

SkillScanResultsDialog::SkillScanResultsDialog(GoBridge* bridge, QWidget* parent) : QDialog(parent) {
    setWindowTitle(QStringLiteral("技能检测历史"));
    setProperty("tone", DialogChrome::toneName(DialogChrome::Tone::Info));
    DialogChrome::prepare(this, QSize(660, 560), QSize(600, 500));
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(26, 24, 26, 22);
    root->setSpacing(18);
    root->addWidget(DialogChrome::createHeader(this, QStringLiteral("⌕"), QStringLiteral("技能检测历史"),
                                                QStringLiteral("查看 Skill 扫描结果与处置状态"), DialogChrome::Tone::Info));
    auto* list = new QListWidget(this);
    list->setObjectName(QStringLiteral("skillScanList"));
    list->setSpacing(10);
    list->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    list->addItem(QStringLiteral("正在加载技能扫描记录..."));
    if (bridge != nullptr) {
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [list, watcher]() {
            list->clear();
            const QJsonArray rows = watcher->result().value(QStringLiteral("data")).toArray();
            for (const QJsonValue& value : rows) {
                const QJsonObject row = value.toObject();
                const bool deleted = !row.value(QStringLiteral("deleted_at")).toString().isEmpty();
                const bool trusted = row.value(QStringLiteral("trusted")).toBool();
                const bool safe = row.value(QStringLiteral("safe")).toBool();
                QString color = QStringLiteral("#EF4444");
                QString background = QStringLiteral("rgba(239,68,68,20)");
                QString border = QStringLiteral("rgba(239,68,68,64)");
                QString status = QStringLiteral("检测到风险");
                QString glyph = QStringLiteral("!");
                if (deleted) {
                    color = QStringLiteral("#9CA3AF");
                    background = QStringLiteral("rgba(156,163,175,20)");
                    border = QStringLiteral("rgba(156,163,175,64)");
                    status = QStringLiteral("已删除");
                    glyph = QStringLiteral("×");
                } else if (trusted) {
                    color = QStringLiteral("#3B82F6");
                    background = QStringLiteral("rgba(59,130,246,20)");
                    border = QStringLiteral("rgba(59,130,246,64)");
                    status = QStringLiteral("可信");
                    glyph = QStringLiteral("✓");
                } else if (safe) {
                    color = QStringLiteral("#22C55E");
                    background = QStringLiteral("rgba(34,197,94,20)");
                    border = QStringLiteral("rgba(34,197,94,64)");
                    status = QStringLiteral("安全");
                    glyph = QStringLiteral("✓");
                } else {
                    const QString level = row.value(QStringLiteral("risk_level")).toString().toLower();
                    if (level == QStringLiteral("critical")) status = QStringLiteral("严重风险");
                    else if (level == QStringLiteral("high")) status = QStringLiteral("高风险");
                    else if (level == QStringLiteral("medium")) status = QStringLiteral("中风险");
                    else if (level == QStringLiteral("low")) status = QStringLiteral("低风险");
                }

                auto* card = new QFrame(list);
                card->setObjectName(QStringLiteral("skillScanCard"));
                card->setStyleSheet(QStringLiteral("QFrame#skillScanCard { background:%1; border:1px solid %2; border-radius:10px; }")
                                        .arg(background, border));
                auto* cardLayout = new QVBoxLayout(card);
                cardLayout->setContentsMargins(14, 12, 14, 12);
                cardLayout->setSpacing(6);
                auto* top = new QHBoxLayout;
                auto* icon = new QLabel(glyph, card);
                icon->setAlignment(Qt::AlignCenter);
                icon->setFixedSize(20, 20);
                icon->setStyleSheet(QStringLiteral("color:%1;font-size:16px;font-weight:700;").arg(color));
                top->addWidget(icon);
                auto* name = titleLabel(row.value(QStringLiteral("skill_name")).toString(), card, "sectionTitle");
                top->addWidget(name, 1);
                auto* badge = new QLabel(status, card);
                badge->setStyleSheet(QStringLiteral("padding:3px 8px;border-radius:4px;background:%1;color:%2;font-size:10px;font-weight:600;")
                                         .arg(color + QStringLiteral("33"), color));
                top->addWidget(badge);
                cardLayout->addLayout(top);

                const QString scannedAt = row.value(QStringLiteral("scanned_at")).toString();
                if (!scannedAt.isEmpty()) {
                    const QDateTime parsed = QDateTime::fromString(scannedAt, Qt::ISODate);
                    auto* time = new QLabel(QStringLiteral("扫描时间：%1").arg(parsed.isValid() ? parsed.toLocalTime().toString(QStringLiteral("yyyy-MM-dd HH:mm")) : scannedAt), card);
                    time->setObjectName(QStringLiteral("subtle"));
                    cardLayout->addWidget(time);
                }
                const QString skillPath = row.value(QStringLiteral("skill_path")).toString();
                if (!skillPath.isEmpty()) {
                    auto* path = new QLabel(QStringLiteral("▱  %1").arg(skillPath), card);
                    path->setObjectName(QStringLiteral("skillPath"));
                    path->setText(path->fontMetrics().elidedText(path->text(), Qt::ElideMiddle, 455));
                    path->setToolTip(skillPath);
                    path->setTextInteractionFlags(Qt::TextSelectableByMouse);
                    cardLayout->addWidget(path);
                }

                const QJsonArray issues = row.value(QStringLiteral("issues")).toArray();
                if (!safe && !trusted && !issues.isEmpty()) {
                    auto* issueToggle = new QPushButton(QStringLiteral("›  %1 个问题").arg(issues.size()), card);
                    issueToggle->setObjectName(QStringLiteral("skillIssueToggle"));
                    issueToggle->setCheckable(true);
                    cardLayout->addWidget(issueToggle, 0, Qt::AlignLeft);
                    auto* issueBody = new QWidget(card);
                    auto* issueLayout = new QVBoxLayout(issueBody);
                    issueLayout->setContentsMargins(18, 0, 0, 0);
                    issueLayout->setSpacing(4);
                    for (const QJsonValue& issue : issues) {
                        auto* issueLabel = new QLabel(QStringLiteral("- %1").arg(issue.toString()), issueBody);
                        issueLabel->setObjectName(QStringLiteral("subtle"));
                        issueLabel->setWordWrap(true);
                        issueLayout->addWidget(issueLabel);
                    }
                    issueBody->hide();
                    cardLayout->addWidget(issueBody);
                    connect(issueToggle, &QPushButton::toggled, card, [list, card, issueToggle, issueBody](bool expanded) {
                        issueToggle->setText(QStringLiteral("%1  %2 个问题").arg(expanded ? QStringLiteral("⌄") : QStringLiteral("›")).arg(issueBody->findChildren<QLabel*>().size()));
                        issueBody->setVisible(expanded);
                        if (QListWidgetItem* item = list->itemAt(card->pos())) item->setSizeHint(card->sizeHint());
                    });
                }
                auto* item = new QListWidgetItem(list);
                item->setFlags(Qt::ItemIsEnabled);
                item->setSizeHint(card->sizeHint());
                list->setItemWidget(item, card);
            }
            if (list->count() == 0) {
                auto* empty = new QListWidgetItem(QStringLiteral("⌕\n\n暂无技能扫描记录"), list);
                empty->setTextAlignment(Qt::AlignCenter);
                empty->setSizeHint(QSize(0, 220));
                empty->setFlags(Qt::ItemIsEnabled);
            }
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge->workerPool(), [bridge]() { return bridge->call("GetAllSkillScansFFI"); }));
    }
    root->addWidget(list, 1);
    auto* done = new QPushButton(QStringLiteral("完成"), this);
    done->setObjectName(QStringLiteral("primaryButton"));
    connect(done, &QPushButton::clicked, this, &QDialog::accept);
    root->addWidget(done, 0, Qt::AlignRight);
}

OnboardingDialog::OnboardingDialog(GoBridge* bridge, QWidget* parent) : QDialog(parent), bridge_(bridge) {
    setWindowTitle(QStringLiteral("快速开始"));
    setProperty("tone", DialogChrome::toneName(DialogChrome::Tone::Accent));
    DialogChrome::prepare(this, QSize(800, 700), QSize(720, 640));
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(28, 24, 28, 22);
    root->setSpacing(18);
    auto* header = DialogChrome::createHeader(this, QStringLiteral("◇"), QStringLiteral("快速开始"),
                                               QStringLiteral("四步完成安全模型与 Bot 防护配置"));
    stepLabel_ = new QLabel(QStringLiteral("1 / 4"), header);
    stepLabel_->setObjectName(QStringLiteral("dialogStepBadge"));
    qobject_cast<QHBoxLayout*>(header->layout())->insertWidget(header->layout()->count() - 1, stepLabel_, 0, Qt::AlignTop);
    root->addWidget(header);
    pages_ = new QStackedWidget(this);
    auto addStep = [this](const QString& title, const QString& description, const QStringList& items) {
        auto* page = new QWidget(pages_);
        auto* layout = new QVBoxLayout(page);
        layout->setContentsMargins(10, 28, 10, 10);
        layout->setSpacing(16);
        layout->addWidget(titleLabel(title, page, "pageTitle"));
        auto* desc = new QLabel(description, page);
        desc->setWordWrap(true);
        desc->setObjectName(QStringLiteral("muted"));
        layout->addWidget(desc);
        for (const QString& item : items) layout->addWidget(actionRow(QStringLiteral("◇"), item.section('|', 0, 0), item.section('|', 1), page));
        pages_->addWidget(page);
        return qMakePair(page, layout);
    };
    auto welcome = addStep(QStringLiteral("欢迎使用"), QStringLiteral("ClawdSecbot —— AI Bot 时代的守护神。完成以下引导后即可开始使用安全防护能力。"),
                           {QStringLiteral("实时防护与意图偏离检测|持续分析代理行为并阻断高风险动作"), QStringLiteral("权限设置与工具/技能管控|按资产配置最小权限和 Skill 扫描"), QStringLiteral("防护监控与审计追溯|查看实时决策、Token 和审计链路")});
    welcome.second->addStretch();

    auto botStep = addStep(QStringLiteral("Bot 模型登记"), QStringLiteral("登记 Openclaw 当前使用的模型，防护代理会按相同协议继续转发。"), {});
    onboardingBotProvider_ = new QComboBox(botStep.first);
    onboardingBotProvider_->addItem(QStringLiteral("OpenAI 兼容"), QStringLiteral("openai"));
    onboardingBotProvider_->addItem(QStringLiteral("Anthropic"), QStringLiteral("anthropic"));
    onboardingBotProvider_->addItem(QStringLiteral("MiniMax"), QStringLiteral("minimax"));
    onboardingBotProvider_->addItem(QStringLiteral("DeepSeek"), QStringLiteral("deepseek"));
    onboardingBotProvider_->addItem(QStringLiteral("Ollama"), QStringLiteral("ollama"));
    onboardingBotBaseUrl_ = new QLineEdit(botStep.first);
    onboardingBotApiKey_ = new QLineEdit(botStep.first);
    onboardingBotApiKey_->setEchoMode(QLineEdit::Password);
    onboardingBotModel_ = new QLineEdit(botStep.first);
    botStep.second->addWidget(fieldBlock(QStringLiteral("模型供应商"), onboardingBotProvider_, botStep.first));
    botStep.second->addWidget(fieldBlock(QStringLiteral("基础 URL"), onboardingBotBaseUrl_, botStep.first));
    botStep.second->addWidget(fieldBlock(QStringLiteral("API 密钥"), onboardingBotApiKey_, botStep.first));
    botStep.second->addWidget(fieldBlock(QStringLiteral("模型名称"), onboardingBotModel_, botStep.first));
    botStep.second->addStretch();

    auto securityStep = addStep(QStringLiteral("安全模型配置"), QStringLiteral("配置用于 ShepherdGate 风险研判的独立安全模型。"),
                                {QStringLiteral("供应商与模型|支持 MiniMax、OpenAI、Anthropic 及兼容服务"),
                                 QStringLiteral("连通性验证|保存前通过 Go 模型服务验证连接")});
    auto* openSecuritySettings = new QPushButton(QStringLiteral("打开安全模型设置"), securityStep.first);
    openSecuritySettings->setObjectName(QStringLiteral("primaryButton"));
    connect(openSecuritySettings, &QPushButton::clicked, this, [this]() {
        auto* dialog = new SettingsDialog(bridge_, this);
        dialog->setAttribute(Qt::WA_DeleteOnClose);
        dialog->open();
    });
    securityStep.second->addWidget(openSecuritySettings, 0, Qt::AlignLeft);
    securityStep.second->addStretch();

    auto updateStep = addStep(QStringLiteral("Bot 配置更新"), QStringLiteral("启动防护后，将 Bot 模型端点切换到本地代理。"),
                              {QStringLiteral("1. 打开 Dashboard|执行 openclaw dashboard --no-open 并进入 Settings > Config"),
                               QStringLiteral("2. 添加代理供应商|Base URL 使用 http://127.0.0.1:13436"),
                               QStringLiteral("3. 更新默认模型|将 Primary Model 指向 clawdsecbot provider"),
                               QStringLiteral("4. 保存并重载|点击 Save 后再点击 Reload")});
    auto* openGuide = new QPushButton(QStringLiteral("打开配置引导"), updateStep.first);
    openGuide->setObjectName(QStringLiteral("primaryButton"));
    connect(openGuide, &QPushButton::clicked, this, [this]() {
        auto* dialog = new AppStoreGuideDialog(this);
        dialog->setAttribute(Qt::WA_DeleteOnClose);
        dialog->open();
    });
    updateStep.second->addWidget(openGuide, 0, Qt::AlignLeft);
    updateStep.second->addStretch();
    root->addWidget(pages_, 1);
    auto* buttons = new QHBoxLayout;
    auto* back = new QPushButton(QStringLiteral("上一步"), this);
    nextButton_ = new QPushButton(QStringLiteral("下一步"), this);
    nextButton_->setObjectName(QStringLiteral("primaryButton"));
    connect(back, &QPushButton::clicked, this, [this]() { moveStep(-1); });
    connect(nextButton_, &QPushButton::clicked, this, [this]() {
        if (pages_->currentIndex() == 1) return saveBotAndContinue();
        if (pages_->currentIndex() == pages_->count() - 1) {
            if (bridge_ != nullptr) {
                const QJsonObject payload{{QStringLiteral("key"), QStringLiteral("is_first_launch")}, {QStringLiteral("value"), false}};
                const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
                [[maybe_unused]] const QFuture<void> saveFuture = QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, json]() { bridge->call("SaveAppSettingFFI", json); });
            }
            return accept();
        }
        moveStep(1);
    });
    buttons->addWidget(back);
    buttons->addStretch();
    buttons->addWidget(nextButton_);
    root->addLayout(buttons);

    if (bridge_ != nullptr && bridge_->isReady()) {
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
            const QJsonObject result = watcher->result();
            onboardingAssetId_ = result.value(QStringLiteral("asset_id")).toString();
            const QJsonObject data = unwrap(result.value(QStringLiteral("config")).toObject());
            selectData(onboardingBotProvider_, data.value(QStringLiteral("provider")).toString());
            onboardingBotBaseUrl_->setText(data.value(QStringLiteral("base_url")).toString());
            onboardingBotApiKey_->setText(data.value(QStringLiteral("api_key")).toString());
            onboardingBotModel_->setText(data.value(QStringLiteral("model")).toString());
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_]() {
            const ScanResultModel scan = ScanResultModel::fromResponse(bridge->call("GetLatestScanResult"));
            AssetModel selected;
            for (const AssetModel& asset : scan.assets) {
                if (selected.id.isEmpty() || asset.name.compare(QStringLiteral("Openclaw"), Qt::CaseInsensitive) == 0) selected = asset;
                if (asset.name.compare(QStringLiteral("Openclaw"), Qt::CaseInsensitive) == 0) break;
            }
            return QJsonObject{{QStringLiteral("asset_id"), selected.id},
                               {QStringLiteral("config"), selected.id.isEmpty() ? QJsonObject{} : bridge->call("GetBotModelConfigFFI", selected.id)}};
        }));
    }
}

void OnboardingDialog::moveStep(int delta) {
    const int index = qBound(0, pages_->currentIndex() + delta, pages_->count() - 1);
    pages_->setCurrentIndex(index);
    stepLabel_->setText(QStringLiteral("%1 / %2").arg(index + 1).arg(pages_->count()));
    nextButton_->setText(index == pages_->count() - 1 ? QStringLiteral("完成") : QStringLiteral("下一步"));
}

void OnboardingDialog::saveBotAndContinue() {
    if (bridge_ == nullptr) return moveStep(1);
    if (onboardingAssetId_.isEmpty()) {
        UiDialogs::showWarning(this, QStringLiteral("未发现 Bot"), QStringLiteral("请先完成资产扫描，再登记 Bot 模型。"));
        return;
    }
    if (onboardingBotBaseUrl_->text().trimmed().isEmpty() || onboardingBotModel_->text().trimmed().isEmpty()) {
        UiDialogs::showWarning(this, QStringLiteral("Bot 模型未配置"), QStringLiteral("请填写基础 URL 和模型名称。"));
        return;
    }
    const QJsonObject payload{{QStringLiteral("asset_id"), onboardingAssetId_},
                              {QStringLiteral("provider"), onboardingBotProvider_->currentData().toString()},
                              {QStringLiteral("base_url"), onboardingBotBaseUrl_->text().trimmed()},
                              {QStringLiteral("api_key"), onboardingBotApiKey_->text().trimmed()},
                              {QStringLiteral("model"), onboardingBotModel_->text().trimmed()}};
    const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
    nextButton_->setEnabled(false);
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        nextButton_->setEnabled(true);
        const QJsonObject result = watcher->result();
        if (result.value(QStringLiteral("success")).toBool()) moveStep(1);
        else UiDialogs::showWarning(this, QStringLiteral("保存失败"), result.value(QStringLiteral("error")).toString());
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, json]() { return bridge->call("SaveBotModelConfigFFI", json); }));
}

MitigationDialog::MitigationDialog(const RiskModel& risk, GoBridge* bridge, QWidget* parent) : QDialog(parent) {
    setWindowTitle(QStringLiteral("风险处置"));
    setProperty("tone", DialogChrome::toneName(DialogChrome::Tone::Warning));
    DialogChrome::prepare(this, QSize(660, 610), QSize(600, 540));
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(26, 24, 26, 22);
    root->setSpacing(18);
    root->addWidget(DialogChrome::createHeader(this, QStringLiteral("△"), QStringLiteral("风险处置"),
                                                QStringLiteral("确认风险信息并选择安全修复动作"), DialogChrome::Tone::Warning));

    auto* riskCard = new QFrame(this);
    riskCard->setObjectName(QStringLiteral("mitigationRiskCard"));
    auto* riskLayout = new QVBoxLayout(riskCard);
    riskLayout->setContentsMargins(14, 12, 14, 12);
    riskLayout->setSpacing(6);
    riskLayout->addWidget(titleLabel(mitigationRiskTitle(risk), riskCard, "sectionTitle"));
    auto* description = new QLabel(mitigationRiskDescription(risk), riskCard);
    description->setWordWrap(true);
    description->setObjectName(QStringLiteral("muted"));
    riskLayout->addWidget(description);
    root->addWidget(riskCard);
    auto* form = new QWidget(this);
    auto* formLayout = new QVBoxLayout(form);
    formLayout->setContentsMargins(0, 8, 0, 8);
    formLayout->setSpacing(12);
    const QJsonArray schema = risk.mitigation.value(QStringLiteral("form_schema")).toArray();
    for (const QJsonValue& value : schema) {
        const QJsonObject item = value.toObject();
        const QString key = item.value(QStringLiteral("key")).toString();
        QString label = item.value(QStringLiteral("label")).toString();
        if (key == QStringLiteral("fix_permission")) label = QStringLiteral("修复目录/文件权限（Unix: chmod）");
        const QString type = item.value(QStringLiteral("type")).toString(QStringLiteral("text"));
        if (type == QStringLiteral("boolean")) {
            auto* check = new QCheckBox(label, form);
            check->setProperty("fieldKey", key);
            check->setChecked(item.value(QStringLiteral("default_value")).toBool());
            formLayout->addWidget(check);
        } else if (type == QStringLiteral("select")) {
            auto* combo = new QComboBox(form);
            combo->setProperty("fieldKey", key);
            for (const QJsonValue& option : item.value(QStringLiteral("options")).toArray()) combo->addItem(option.toString(), option.toString());
            selectData(combo, item.value(QStringLiteral("default_value")).toVariant().toString());
            formLayout->addWidget(fieldBlock(label, combo, form));
        } else {
            auto* edit = new QLineEdit(item.value(QStringLiteral("default_value")).toVariant().toString(), form);
            edit->setProperty("fieldKey", key);
            edit->setProperty("required", item.value(QStringLiteral("required")).toBool());
            if (type == QStringLiteral("password")) edit->setEchoMode(QLineEdit::Password);
            formLayout->addWidget(fieldBlock(label, edit, form));
        }
    }
    const QJsonArray suggestions = risk.mitigation.value(QStringLiteral("suggestions")).toArray();
    for (const QJsonValue& groupValue : suggestions) {
        const QJsonObject group = groupValue.toObject();
        const QString priority = group.value(QStringLiteral("priority")).toString();
        formLayout->addWidget(titleLabel(QStringLiteral("%1%2").arg(priority.isEmpty() ? QString() : priority + QStringLiteral(" - "), group.value(QStringLiteral("category")).toString()), form, "sectionTitle"));
        int suggestionIndex = 0;
        for (const QJsonValue& itemValue : group.value(QStringLiteral("items")).toArray()) {
            const QJsonObject item = itemValue.toObject();
            ++suggestionIndex;
            auto* suggestion = actionRow(QString::number(suggestionIndex), item.value(QStringLiteral("action")).toString(), item.value(QStringLiteral("detail")).toString(), form);
            const QString command = item.value(QStringLiteral("command")).toString();
            if (!command.isEmpty()) {
                auto* copy = new QPushButton(QStringLiteral("复制命令"), suggestion);
                copy->setObjectName(QStringLiteral("secondaryButton"));
                connect(copy, &QPushButton::clicked, suggestion, [command]() { QGuiApplication::clipboard()->setText(command); });
                static_cast<QHBoxLayout*>(suggestion->layout())->insertWidget(suggestion->layout()->count() - 1, copy);
            }
            formLayout->addWidget(suggestion);
        }
    }
    if (schema.isEmpty() && suggestions.isEmpty()) {
        auto* confirmation = new QLabel(QStringLiteral("确定要执行自动修复吗？"), form);
        confirmation->setObjectName(QStringLiteral("muted"));
        formLayout->addWidget(confirmation);
    }
    formLayout->addStretch();
    root->addWidget(scrollPage(form, this), 1);
    auto* buttons = new QDialogButtonBox(QDialogButtonBox::Cancel | QDialogButtonBox::Apply, this);
    DialogChrome::styleButtonBox(buttons);
    const bool suggestionOnly = risk.mitigation.value(QStringLiteral("type")).toString() == QStringLiteral("suggestion");
    buttons->button(QDialogButtonBox::Cancel)->setText(suggestionOnly ? QStringLiteral("关闭") : QStringLiteral("取消"));
    buttons->button(QDialogButtonBox::Apply)->setText(QStringLiteral("执行修复"));
    buttons->button(QDialogButtonBox::Apply)->setObjectName(QStringLiteral("primaryButton"));
    buttons->button(QDialogButtonBox::Apply)->setVisible(!suggestionOnly);
    connect(buttons, &QDialogButtonBox::rejected, this, &QDialog::reject);
    auto* applyButton = buttons->button(QDialogButtonBox::Apply);
    connect(buttons, &QDialogButtonBox::accepted, this, [this, bridge, risk, form, applyButton]() {
        QJsonObject values;
        for (QLineEdit* edit : form->findChildren<QLineEdit*>()) {
            const QString key = edit->property("fieldKey").toString();
            if (key.isEmpty()) continue;
            if (edit->property("required").toBool() && edit->text().trimmed().isEmpty()) {
                UiDialogs::showWarning(this, QStringLiteral("请完善配置"), QStringLiteral("必填项不能为空。"));
                edit->setFocus();
                return;
            }
            values.insert(key, edit->text());
        }
        for (QCheckBox* check : form->findChildren<QCheckBox*>()) {
            const QString key = check->property("fieldKey").toString();
            if (!key.isEmpty()) values.insert(key, check->isChecked());
        }
        for (QComboBox* combo : form->findChildren<QComboBox*>()) {
            const QString key = combo->property("fieldKey").toString();
            if (!key.isEmpty()) values.insert(key, combo->currentData().toString());
        }
        QJsonObject payload = risk.raw;
        payload.insert(QStringLiteral("form_values"), values);
        if (bridge == nullptr) return accept();
        const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
        applyButton->setEnabled(false);
        applyButton->setText(QStringLiteral("修复中…"));
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, applyButton]() {
            applyButton->setEnabled(true);
            applyButton->setText(QStringLiteral("执行修复"));
            const QJsonObject response = watcher->result();
            if (!response.value(QStringLiteral("success")).toBool()) UiDialogs::showWarning(this, QStringLiteral("修复失败"), response.value(QStringLiteral("error")).toString());
            else accept();
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge->workerPool(), [bridge, json]() { return bridge->call("MitigateRiskFFI", json); }));
    });
    root->addWidget(buttons);
}
