#include "ui/MainWindow.h"

#include "bridge/GoBridge.h"
#include "service/ProtectionService.h"
#include "service/ScanService.h"
#include "ui/AuditLogWindow.h"
#include "ui/Dialogs.h"
#include "ui/ProtectionMonitorWindow.h"
#include "ui/widgets/AssetCardWidget.h"
#include "ui/widgets/GradientWidget.h"

#include <QDateTime>
#include <QDialog>
#include <QEvent>
#include <QFutureWatcher>
#include <QFrame>
#include <QHBoxLayout>
#include <QHash>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QMenu>
#include <QMessageBox>
#include <QMouseEvent>
#include <QProgressBar>
#include <QPointer>
#include <QPixmap>
#include <QPushButton>
#include <QStyle>
#include <QScrollArea>
#include <QSet>
#include <QStackedWidget>
#include <QStringList>
#include <QTimer>
#include <QVBoxLayout>
#include <QWindow>
#include <QtConcurrent>

namespace {
QLabel* namedLabel(const QString& text, const char* name, QWidget* parent) {
    auto* label = new QLabel(text, parent);
    label->setObjectName(QString::fromLatin1(name));
    return label;
}

QFrame* sectionHeader(const QString& icon, const QString& title, QWidget* parent) {
    auto* frame = new QFrame(parent);
    auto* layout = new QHBoxLayout(frame);
    layout->setContentsMargins(0, 4, 0, 4);
    auto* glyph = new QLabel(icon, frame);
    glyph->setStyleSheet(QStringLiteral("color:#818CF8;font-size:16px;"));
    layout->addWidget(glyph);
    layout->addWidget(namedLabel(title, "sectionTitle", frame));
    layout->addStretch();
    return frame;
}

QString riskObjectName(const QString& level) {
    if (level.compare(QStringLiteral("medium"), Qt::CaseInsensitive) == 0 || level == QStringLiteral("1")) return QStringLiteral("riskMedium");
    if (level.compare(QStringLiteral("low"), Qt::CaseInsensitive) == 0 || level == QStringLiteral("0")) return QStringLiteral("riskLow");
    return QStringLiteral("riskHigh");
}

QString riskDisplay(const QString& level) {
    if (level.compare(QStringLiteral("critical"), Qt::CaseInsensitive) == 0 || level == QStringLiteral("3")) return QStringLiteral("严重");
    if (level.compare(QStringLiteral("high"), Qt::CaseInsensitive) == 0 || level == QStringLiteral("2")) return QStringLiteral("高");
    if (level.compare(QStringLiteral("medium"), Qt::CaseInsensitive) == 0 || level == QStringLiteral("1")) return QStringLiteral("中");
    return QStringLiteral("低");
}

QString localizedRiskTitle(const RiskModel& risk) {
    static const QHash<QString, QString> titles{
        {QStringLiteral("gateway_bind_unsafe"), QStringLiteral("非回环地址绑定")},
        {QStringLiteral("gateway_auth_disabled"), QStringLiteral("未配置认证")},
        {QStringLiteral("gateway_auth_password_mode"), QStringLiteral("网关启用了密码模式")},
        {QStringLiteral("gateway_weak_password"), QStringLiteral("认证密码太弱")},
        {QStringLiteral("gateway_weak_token"), QStringLiteral("网关 Token 强度不足")},
        {QStringLiteral("config_perm_unsafe"), QStringLiteral("配置文件权限不安全")},
        {QStringLiteral("config_dir_perm_unsafe"), QStringLiteral("配置目录权限不安全")},
        {QStringLiteral("sandbox_disabled_default"), QStringLiteral("默认沙箱已禁用")},
        {QStringLiteral("sandbox_disabled_agent"), QStringLiteral("Agent 沙箱已禁用")},
        {QStringLiteral("logging_redact_off"), QStringLiteral("敏感数据脱敏已禁用")},
        {QStringLiteral("audit_disabled"), QStringLiteral("安全审计日志已禁用")},
        {QStringLiteral("autonomy_workspace_unrestricted"), QStringLiteral("工作区访问范围未限制")},
        {QStringLiteral("log_dir_perm_unsafe"), QStringLiteral("日志目录权限不安全")},
        {QStringLiteral("plaintext_secrets"), QStringLiteral("配置文件中发现明文密钥")},
        {QStringLiteral("skills_not_scanned"), QStringLiteral("Skills 未进行提示词注入扫描")},
        {QStringLiteral("openclaw_insecure_or_dangerous_flags"), QStringLiteral("OpenClaw 网关危险开关已启用")},
        {QStringLiteral("openclaw_config_patch_outdated"), QStringLiteral("OpenClaw 配置安全补丁缺失")},
        {QStringLiteral("terminal_backend_local"), QStringLiteral("终端后端为本地执行")},
        {QStringLiteral("approvals_mode_disabled"), QStringLiteral("审批模式已禁用")},
        {QStringLiteral("redact_secrets_disabled"), QStringLiteral("密钥脱敏已禁用")},
        {QStringLiteral("model_base_url_public"), QStringLiteral("自定义模型地址暴露公网")},
        {QStringLiteral("process_running_as_root"), QStringLiteral("进程以 root 身份运行")},
        {QStringLiteral("memory_dir_perm_unsafe"), QStringLiteral("memory 目录权限不安全")},
        {QStringLiteral("skill_agent_risk"), QStringLiteral("检测到高风险 Skill")},
    };
    return titles.value(risk.id, risk.title);
}

QString localizedRiskDescription(const RiskModel& risk) {
    const QJsonObject args = risk.raw.value(QStringLiteral("args")).toObject();
    if (risk.id == QStringLiteral("openclaw_insecure_or_dangerous_flags")) {
        QStringList flags;
        for (const QJsonValue& value : args.value(QStringLiteral("flags")).toArray()) flags.append(value.toString());
        return QStringLiteral("OpenClaw 网关开关削弱了认证或来源信任保护：%1。除非有明确且已验证的威胁模型例外，否则应关闭这些开关。")
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

MainWindow::MainWindow(GoBridge* bridge, QWidget* parent) : QMainWindow(parent), bridge_(bridge) {
#if !defined(Q_OS_LINUX)
    setWindowFlags(windowFlags() | Qt::FramelessWindowHint);
#endif
    setWindowTitle(QStringLiteral("ClawdSecbot"));
    setMinimumSize(610, 780);
    resize(610, 780);
    buildUi();
}

void MainWindow::initializeData() {
    loadLatestResult();
    showOnboardingIfNeeded();
}

void MainWindow::showOnboardingIfNeeded() {
    if (bridge_ == nullptr || !bridge_->isReady()) return;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonObject response = watcher->result();
        const QJsonValue raw = response.value(QStringLiteral("data"));
        const QJsonValue stored = raw.isObject() ? raw.toObject().value(QStringLiteral("value")) : raw;
        const QString storedText = stored.toVariant().toString().trimmed().toLower();
        const bool firstLaunch = stored.isUndefined() || stored.isNull() || storedText.isEmpty()
            || (stored.isBool() ? stored.toBool() : storedText != QStringLiteral("false") && storedText != QStringLiteral("0"));
        watcher->deleteLater();
        if (firstLaunch) OnboardingDialog(bridge_, this).exec();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_]() { return bridge->call("GetAppSettingFFI", QStringLiteral("is_first_launch")); }));
}

void MainWindow::buildUi() {
    auto* shell = new GradientWidget(this);
    shell->setObjectName(QStringLiteral("appShell"));
    auto* root = new QVBoxLayout(shell);
    root->setContentsMargins(0, 0, 0, 0);
    root->setSpacing(0);
    root->addWidget(buildTitleBar());
    pages_ = new QStackedWidget(shell);
    pages_->addWidget(buildIdlePage());
    pages_->addWidget(buildScanningPage());
    pages_->addWidget(buildResultsPage());
    root->addWidget(pages_, 1);
    setCentralWidget(shell);
}

QWidget* MainWindow::buildTitleBar() {
    auto* bar = new QWidget(this);
    bar->setObjectName(QStringLiteral("titleBar"));
    bar->setFixedHeight(48);
    bar->installEventFilter(this);
    auto* layout = new QHBoxLayout(bar);
    layout->setContentsMargins(16, 0, 16, 0);
    layout->setSpacing(8);
    auto* icon = new QLabel(bar);
    icon->setAlignment(Qt::AlignCenter);
    icon->setFixedSize(28, 28);
    icon->setPixmap(QPixmap(QStringLiteral(":/images/app_icon.png")).scaled(28, 28, Qt::KeepAspectRatio, Qt::SmoothTransformation));
    layout->addWidget(icon);
    layout->addWidget(namedLabel(QStringLiteral("ClawdSecbot"), "appTitle", bar));
    layout->addStretch();

    auto* audit = new QPushButton(QStringLiteral("♙"), bar);
    audit->setObjectName(QStringLiteral("iconButton"));
    audit->setToolTip(QStringLiteral("审计日志"));
    connect(audit, &QPushButton::clicked, this, &MainWindow::openAuditLog);
    auto* settings = new QPushButton(QStringLiteral("⚙"), bar);
    settings->setObjectName(QStringLiteral("iconButton"));
    settings->setToolTip(QStringLiteral("全局设置"));
    connect(settings, &QPushButton::clicked, this, &MainWindow::openSettings);
    auto* more = new QPushButton(bar);
    more->setText(QStringLiteral("⋯"));
    more->setObjectName(QStringLiteral("iconButton"));
    connect(more, &QPushButton::clicked, more, [this, more]() {
        QDialog menu(this, Qt::Dialog | Qt::FramelessWindowHint);
        menu.setWindowTitle(QStringLiteral("更多操作"));
        menu.setObjectName(QStringLiteral("compactMenu"));
        menu.setFixedWidth(220);
        auto* menuLayout = new QVBoxLayout(&menu);
        menuLayout->setContentsMargins(6, 6, 6, 6);
        menuLayout->setSpacing(3);
        auto addMenuAction = [&menu, menuLayout](const QString& text) {
            auto* button = new QPushButton(text, &menu);
            button->setObjectName(QStringLiteral("compactMenuItem"));
            menuLayout->addWidget(button);
            return button;
        };
        auto* onboarding = addMenuAction(QStringLiteral("快速开始"));
        auto* guide = addMenuAction(QStringLiteral("配置引导"));
        auto* skills = addMenuAction(QStringLiteral("AI 技能安全分析"));
        connect(onboarding, &QPushButton::clicked, &menu, [&menu]() { menu.done(1); });
        connect(guide, &QPushButton::clicked, &menu, [&menu]() { menu.done(2); });
        connect(skills, &QPushButton::clicked, &menu, [&menu]() { menu.done(3); });
        const QPoint popup = more->mapToGlobal(QPoint(more->width() - menu.width(), more->height() + 4));
        menu.move(popup);
        const int action = menu.exec();
        if (action == 1) OnboardingDialog(bridge_, this).exec();
        else if (action == 2) AppStoreGuideDialog(this).exec();
        else if (action == 3) SkillScanDialog(bridge_, {}, this).exec();
    });
    layout->addWidget(audit);
    layout->addWidget(settings);
    layout->addWidget(more);
#if !defined(Q_OS_LINUX)
    auto* minimize = new QPushButton(QStringLiteral("−"), bar);
    minimize->setObjectName(QStringLiteral("iconButton"));
    connect(minimize, &QPushButton::clicked, this, &QWidget::showMinimized);
    auto* close = new QPushButton(QStringLiteral("×"), bar);
    close->setObjectName(QStringLiteral("closeButton"));
    connect(close, &QPushButton::clicked, this, &QWidget::hide);
    layout->addWidget(minimize);
    layout->addWidget(close);
#endif
    return bar;
}

QWidget* MainWindow::buildIdlePage() {
    auto* page = new QWidget(this);
    auto* layout = new QVBoxLayout(page);
    layout->setContentsMargins(32, 32, 32, 32);
    layout->addStretch(2);
    auto* shield = new QLabel(QStringLiteral("♢"), page);
    shield->setAlignment(Qt::AlignCenter);
    shield->setFixedSize(104, 104);
    shield->setStyleSheet(QStringLiteral("font-size:48px;color:#818CF8;background:rgba(99,102,241,32);border:2px solid rgba(99,102,241,75);border-radius:52px;"));
    layout->addWidget(shield, 0, Qt::AlignCenter);
    layout->addSpacing(28);
    auto* title = namedLabel(QStringLiteral("ClawdSecbot 龙虾卫士"), "pageTitle", page);
    title->setAlignment(Qt::AlignCenter);
    layout->addWidget(title);
    auto* subtitle = namedLabel(QStringLiteral("扫描您的 Clawdbot 配置以查找安全风险"), "muted", page);
    subtitle->setAlignment(Qt::AlignCenter);
    layout->addWidget(subtitle);
    layout->addSpacing(34);
    scanButton_ = new QPushButton(QStringLiteral("⌕  开始安全扫描"), page);
    scanButton_->setObjectName(QStringLiteral("primaryButton"));
    scanButton_->setMinimumHeight(52);
    connect(scanButton_, &QPushButton::clicked, this, &MainWindow::startScan);
    layout->addWidget(scanButton_, 0, Qt::AlignCenter);
    auto* history = new QPushButton(QStringLiteral("⌕  技能检测历史"), page);
    history->setStyleSheet(QStringLiteral("background:transparent;color:rgba(255,255,255,100);"));
    connect(history, &QPushButton::clicked, this, [this]() { SkillScanResultsDialog(bridge_, this).exec(); });
    layout->addWidget(history, 0, Qt::AlignCenter);
    layout->addStretch(3);
    return page;
}

QWidget* MainWindow::buildScanningPage() {
    auto* page = new QWidget(this);
    auto* layout = new QVBoxLayout(page);
    layout->setContentsMargins(24, 24, 24, 24);
    auto* heading = sectionHeader(QStringLiteral("◌"), QStringLiteral("扫描中..."), page);
    layout->addWidget(heading);
    layout->addSpacing(12);
    auto* card = new QFrame(page);
    card->setObjectName(QStringLiteral("card"));
    auto* cardLayout = new QVBoxLayout(card);
    cardLayout->setContentsMargins(18, 18, 18, 18);
    cardLayout->setSpacing(10);
    cardLayout->addWidget(namedLabel(QStringLiteral("扫描进度"), "sectionTitle", card));
    scanStep_ = namedLabel(QStringLiteral("正在检测中，请稍后。"), "muted", card);
    cardLayout->addWidget(scanStep_);
    scanProgress_ = new QProgressBar(card);
    scanProgress_->setRange(0, 100);
    scanProgress_->setValue(6);
    scanProgress_->setTextVisible(false);
    cardLayout->addWidget(scanProgress_);
    layout->addWidget(card);
    layout->addSpacing(16);
    for (const QString& stage : {QStringLiteral("发现已安装的 Bot"), QStringLiteral("检查安全基线与已知漏洞"), QStringLiteral("检测 Skill 提示词注入和供应链风险")}) {
        auto* row = new QFrame(page);
        row->setObjectName(QStringLiteral("card"));
        auto* rowLayout = new QHBoxLayout(row);
        rowLayout->addWidget(new QLabel(QStringLiteral("◇"), row));
        rowLayout->addWidget(namedLabel(stage, "muted", row), 1);
        rowLayout->addWidget(new QLabel(QStringLiteral("…"), row));
        layout->addWidget(row);
    }
    layout->addStretch();
    return page;
}

QWidget* MainWindow::buildResultsPage() {
    auto* scroll = new QScrollArea(this);
    scroll->setWidgetResizable(true);
    scroll->setFrameShape(QFrame::NoFrame);
    scroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    resultContent_ = new QWidget(scroll);
    resultLayout_ = new QVBoxLayout(resultContent_);
    resultLayout_->setContentsMargins(20, 20, 20, 24);
    resultLayout_->setSpacing(12);
    scroll->setWidget(resultContent_);
    return scroll;
}

void MainWindow::loadLatestResult() {
    if (bridge_ == nullptr || !bridge_->isReady()) return;
    auto* watcher = new QFutureWatcher<ScanResultModel>(this);
    connect(watcher, &QFutureWatcher<ScanResultModel>::finished, this, [this, watcher]() {
        const ScanResultModel result = watcher->result();
        if (result.valid && (!result.assets.isEmpty() || !result.risks.isEmpty())) {
            renderResult(result);
            pages_->setCurrentIndex(2);
        }
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_]() { return ScanService::loadLatest(*bridge); }));
}

void MainWindow::startScan() {
    if (bridge_ == nullptr || !bridge_->isReady()) {
        QMessageBox::warning(this, QStringLiteral("无法扫描"), QStringLiteral("Go 业务动态库尚未初始化。"));
        return;
    }
    pages_->setCurrentIndex(1);
    scanProgress_->setValue(8);
    scanStep_->setText(QStringLiteral("正在发现已安装的 Bot..."));
    auto* visualTimer = new QTimer(this);
    visualTimer->setInterval(180);
    connect(visualTimer, &QTimer::timeout, this, [this]() { if (scanProgress_->value() < 88) scanProgress_->setValue(scanProgress_->value() + 2); });
    visualTimer->start();
    auto* watcher = new QFutureWatcher<ScanResultModel>(this);
    connect(watcher, &QFutureWatcher<ScanResultModel>::finished, this, [this, watcher, visualTimer]() {
        visualTimer->stop();
        visualTimer->deleteLater();
        const ScanResultModel result = watcher->result();
        if (!result.valid) {
            pages_->setCurrentIndex(0);
            QMessageBox::warning(this, QStringLiteral("扫描失败"), QStringLiteral("Go 扫描接口未返回有效结果。"));
        } else {
            scanProgress_->setValue(100);
            renderResult(result);
            pages_->setCurrentIndex(2);
        }
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_]() { return ScanService::runScan(*bridge); }));
}

void MainWindow::renderResult(const ScanResultModel& result) {
    result_ = result;
    assetCards_.clear();
    while (QLayoutItem* item = resultLayout_->takeAt(0)) {
        if (item->widget() != nullptr) item->widget()->deleteLater();
        delete item;
    }
    auto* header = new QHBoxLayout;
    auto* statusIcon = new QLabel(QStringLiteral("✓"), resultContent_);
    statusIcon->setAlignment(Qt::AlignCenter);
    statusIcon->setFixedSize(44, 44);
    statusIcon->setStyleSheet(QStringLiteral("font-size:24px;color:#22C55E;background:rgba(34,197,94,35);border-radius:10px;"));
    header->addWidget(statusIcon);
    auto* headerText = new QWidget(resultContent_);
    auto* headerTextLayout = new QVBoxLayout(headerText);
    headerTextLayout->setContentsMargins(0, 0, 0, 0);
    headerTextLayout->setSpacing(2);
    resultTitle_ = namedLabel(QStringLiteral("扫描完成"), "pageTitle", headerText);
    const QDateTime scannedAt = QDateTime::fromString(result.scannedAt, Qt::ISODateWithMs);
    const QString displayedTime = scannedAt.isValid() ? scannedAt.toLocalTime().toString(QStringLiteral("yyyy-MM-dd HH:mm:ss")) : result.scannedAt.left(19).replace('T', ' ');
    resultTime_ = namedLabel(QStringLiteral("上次检测: %1").arg(displayedTime), "muted", headerText);
    headerTextLayout->addWidget(resultTitle_);
    headerTextLayout->addWidget(resultTime_);
    header->addWidget(headerText);
    header->addStretch();
    auto* skillHistory = new QPushButton(QStringLiteral("♙  技能检测历史"), resultContent_);
    connect(skillHistory, &QPushButton::clicked, this, [this]() { SkillScanResultsDialog(bridge_, this).exec(); });
    auto* rescanControl = new QFrame(resultContent_);
    rescanControl->setObjectName(QStringLiteral("rescanControl"));
    auto* rescanLayout = new QHBoxLayout(rescanControl);
    rescanLayout->setContentsMargins(0, 0, 0, 0);
    rescanLayout->setSpacing(0);
    auto* rescan = new QPushButton(rescanControl);
    rescan->setObjectName(QStringLiteral("rescanMainButton"));
    rescan->setText(QStringLiteral("↻  重新扫描"));
    auto* rescanDropdown = new QPushButton(QStringLiteral("⌄"), rescanControl);
    rescanDropdown->setObjectName(QStringLiteral("rescanDropdownButton"));
    rescanDropdown->setAccessibleName(QStringLiteral("选择扫描方式"));
    rescanDropdown->setToolTip(QStringLiteral("选择扫描方式"));
    auto* rescanMenu = new QMenu(rescanControl);
    rescanMenu->setObjectName(QStringLiteral("rescanMenu"));
    auto* discoveryScan = rescanMenu->addAction(QStringLiteral("安全发现扫描"));
    auto* fullScan = rescanMenu->addAction(QStringLiteral("全量扫描"));
    rescanLayout->addWidget(rescan);
    rescanLayout->addWidget(rescanDropdown);
    connect(rescan, &QPushButton::clicked, this, &MainWindow::startScan);
    connect(rescanDropdown, &QPushButton::clicked, rescanControl, [rescanControl, rescanMenu]() {
        const int popupX = qMax(0, rescanControl->width() - rescanMenu->sizeHint().width());
        rescanMenu->popup(rescanControl->mapToGlobal(QPoint(popupX, rescanControl->height() + 4)));
    });
    connect(discoveryScan, &QAction::triggered, this, &MainWindow::startScan);
    connect(fullScan, &QAction::triggered, this, &MainWindow::startScan);
    header->addWidget(skillHistory);
    header->addWidget(rescanControl);
    resultLayout_->addLayout(header);
    resultLayout_->addSpacing(6);
    resultLayout_->addWidget(sectionHeader(QStringLiteral("◇"), QStringLiteral("检测到的Bot"), resultContent_));

    for (const AssetModel& asset : result.assets) {
        auto* card = new AssetCardWidget(asset, result.assets.size() <= 1, resultContent_);
        connect(card, &AssetCardWidget::configureRequested, this, [this, asset]() { openProtectionConfig(asset); });
        connect(card, &AssetCardWidget::monitorRequested, this, [this, asset]() { openProtectionMonitor(asset); });
        connect(card, &AssetCardWidget::stopRequested, this, [this, asset]() { stopProtection(asset); });
        if (bridge_ != nullptr && bridge_->isReady()) {
            auto* iconWatcher = new QFutureWatcher<QJsonObject>(card);
            const QPointer<AssetCardWidget> safeCard(card);
            connect(iconWatcher, &QFutureWatcher<QJsonObject>::finished, card, [iconWatcher, safeCard]() {
                const QString saved = iconWatcher->result().value(QStringLiteral("data")).toString();
                const QJsonObject value = QJsonDocument::fromJson(saved.toUtf8()).object();
                if (safeCard != nullptr && !value.isEmpty()) {
                    const QString name = value.value(QStringLiteral("icon")).toString(QStringLiteral("package"));
                    const quint32 rgba = static_cast<quint32>(value.value(QStringLiteral("color")).toDouble(0xFF6366F1));
                    safeCard->setIconAppearance(name, BotIconPickerDialog::glyphForName(name), QColor::fromRgba(rgba));
                }
                iconWatcher->deleteLater();
            });
            iconWatcher->setFuture(QtConcurrent::run([bridge = bridge_, asset]() { return bridge->call("GetAppSettingFFI", QStringLiteral("asset_icon_%1").arg(asset.id)); }));
        }
        connect(card, &AssetCardWidget::iconPickerRequested, this, [this, card, asset]() {
            BotIconPickerDialog picker(card->iconName(), card->iconColor().rgba(), this);
            if (picker.exec() != QDialog::Accepted) return;
            card->setIconAppearance(picker.selectedIcon(), BotIconPickerDialog::glyphForName(picker.selectedIcon()), QColor::fromRgba(picker.selectedColor()));
            if (bridge_ == nullptr || !bridge_->isReady()) return;
            const QJsonObject iconValue{{QStringLiteral("icon"), picker.selectedIcon()},
                                        {QStringLiteral("color"), static_cast<double>(picker.selectedColor())}};
            const QJsonObject payload{{QStringLiteral("key"), QStringLiteral("asset_icon_%1").arg(asset.id)},
                                      {QStringLiteral("value"), QString::fromUtf8(QJsonDocument(iconValue).toJson(QJsonDocument::Compact))}};
            const QString json = QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact));
            auto* saveWatcher = new QFutureWatcher<QJsonObject>(this);
            connect(saveWatcher, &QFutureWatcher<QJsonObject>::finished, this, [this, saveWatcher]() {
                if (!saveWatcher->result().value(QStringLiteral("success")).toBool())
                    QMessageBox::warning(this, QStringLiteral("保存失败"), saveWatcher->result().value(QStringLiteral("error")).toString());
                saveWatcher->deleteLater();
            });
            saveWatcher->setFuture(QtConcurrent::run([bridge = bridge_, json]() { return bridge->call("SaveAppSettingFFI", json); }));
        });
        assetCards_.append(card);
        resultLayout_->addWidget(card);
    }
    if (result.assets.isEmpty()) resultLayout_->addWidget(namedLabel(QStringLiteral("未检测到已安装的 Bot"), "muted", resultContent_));
    resultLayout_->addSpacing(12);
    resultLayout_->addWidget(sectionHeader(QStringLiteral("△"), QStringLiteral("安全发现 (%1)").arg(result.risks.size()), resultContent_));
    for (const RiskModel& risk : result.risks) {
        auto* card = new QFrame(resultContent_);
        card->setObjectName(riskObjectName(risk.level));
        auto* layout = new QHBoxLayout(card);
        layout->setContentsMargins(16, 16, 16, 16);
        layout->setSpacing(12);
        const bool highRisk = riskObjectName(risk.level) == QStringLiteral("riskHigh");
        const bool mediumRisk = riskObjectName(risk.level) == QStringLiteral("riskMedium");
        auto* riskIcon = namedLabel(highRisk ? QStringLiteral("△") : mediumRisk ? QStringLiteral("i") : QStringLiteral("✓"),
                                    highRisk ? "riskIconHigh" : mediumRisk ? "riskIconMedium" : "riskIconLow", card);
        riskIcon->setAlignment(Qt::AlignCenter);
        riskIcon->setFixedSize(36, 36);
        layout->addWidget(riskIcon, 0, Qt::AlignTop);
        auto* content = new QVBoxLayout;
        content->setContentsMargins(0, 0, 0, 0);
        content->setSpacing(6);
        auto* top = new QHBoxLayout;
        top->addWidget(namedLabel(localizedRiskTitle(risk), "sectionTitle", card), 1);
        auto* level = namedLabel(riskDisplay(risk.level), riskObjectName(risk.level) == QStringLiteral("riskHigh") ? "dangerPill" : "warningPill", card);
        top->addWidget(level);
        content->addLayout(top);
        QString assetName = risk.sourcePlugin;
        for (const AssetModel& asset : result.assets) if (asset.id == risk.assetId || asset.sourcePlugin == risk.sourcePlugin) { assetName = asset.name; break; }
        content->addWidget(namedLabel(QStringLiteral("Bot名称: %1").arg(assetName), "pill", card), 0, Qt::AlignLeft);
        auto* description = namedLabel(localizedRiskDescription(risk), "muted", card);
        description->setWordWrap(true);
        content->addWidget(description);
        auto* fix = new QPushButton(QStringLiteral("⌘  修复"), card);
        fix->setObjectName(QStringLiteral("riskFixButton"));
        connect(fix, &QPushButton::clicked, this, [this, risk]() {
            if (risk.id == QStringLiteral("skills_not_scanned")) {
                SkillScanDialog(bridge_, risk.sourcePlugin, this).exec();
                loadLatestResult();
                return;
            }
            if (MitigationDialog(risk, bridge_, this).exec() == QDialog::Accepted) loadLatestResult();
        });
        content->addWidget(fix, 0, Qt::AlignLeft);
        layout->addLayout(content, 1);
        resultLayout_->addWidget(card);
    }
    if (result.risks.isEmpty()) {
        auto* safe = new QFrame(resultContent_);
        safe->setObjectName(QStringLiteral("riskLow"));
        auto* layout = new QVBoxLayout(safe);
        layout->addWidget(namedLabel(QStringLiteral("✓ 未发现明显安全风险"), "sectionTitle", safe));
        layout->addWidget(namedLabel(QStringLiteral("所有已发现资产均通过当前安全检查。"), "muted", safe));
        resultLayout_->addWidget(safe);
    }
    resultLayout_->addStretch();
    refreshProtectionStates();
}

void MainWindow::refreshProtectionStates() {
    if (bridge_ == nullptr || !bridge_->isReady()) return;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        QSet<QString> protectedIds;
        for (const QJsonValue& value : watcher->result().value(QStringLiteral("data")).toArray()) {
            protectedIds.insert(value.toObject().value(QStringLiteral("asset_id")).toString());
        }
        for (AssetCardWidget* card : assetCards_) {
            if (card != nullptr) card->setProtected(protectedIds.contains(card->asset().id));
        }
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_]() { return bridge->call("GetEnabledProtectionConfigsFFI"); }));
}

void MainWindow::stopProtection(const AssetModel& asset) {
    if (bridge_ == nullptr || QMessageBox::question(this, QStringLiteral("停止防护"),
                                                     QStringLiteral("停止 %1 的防护并恢复 Bot 原始配置？").arg(asset.name)) != QMessageBox::Yes) return;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    setEnabled(false);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        setEnabled(true);
        const QJsonObject result = watcher->result();
        if (!result.value(QStringLiteral("success")).toBool()) {
            QMessageBox::warning(this, QStringLiteral("停止防护失败"), result.value(QStringLiteral("error")).toString());
        }
        refreshProtectionStates();
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, asset]() { return ProtectionService::stopAndRestore(*bridge, asset); }));
}

void MainWindow::openSettings() { SettingsDialog(bridge_, this).exec(); }

void MainWindow::openAuditLog() {
    auto* window = new AuditLogWindow(bridge_);
    window->show();
    window->raise();
}

void MainWindow::openProtectionConfig(const AssetModel& asset) {
    ProtectionConfigDialog dialog(asset, bridge_, this);
    if (dialog.exec() == QDialog::Accepted) openProtectionMonitor(asset, dialog.sessionId());
}

void MainWindow::openProtectionMonitor(const AssetModel& asset, const QString& sessionId) {
    auto* window = new ProtectionMonitorWindow(asset, bridge_, sessionId);
    window->show();
    window->raise();
}

bool MainWindow::eventFilter(QObject* watched, QEvent* event) {
    if (!watched->property("botIconAssetId").toString().isEmpty() && event->type() == QEvent::MouseButtonPress) {
        BotIconPickerDialog picker(QStringLiteral("package"), 0xFF6366F1, this);
        if (picker.exec() == QDialog::Accepted) {
            auto* label = qobject_cast<QLabel*>(watched);
            if (label != nullptr) label->setText(BotIconPickerDialog::glyphForName(picker.selectedIcon()));
        }
        return true;
    }
    if (watched->objectName() == QStringLiteral("titleBar") && event->type() == QEvent::MouseButtonPress) {
        const auto* mouse = static_cast<QMouseEvent*>(event);
        if (mouse->button() == Qt::LeftButton && windowHandle() != nullptr) {
            windowHandle()->startSystemMove();
            return true;
        }
    }
    return QMainWindow::eventFilter(watched, event);
}
