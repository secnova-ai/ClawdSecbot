#include "ui/widgets/AssetCardWidget.h"

#include <QEvent>
#include <QHash>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonObject>
#include <QLabel>
#include <QMouseEvent>
#include <QPushButton>
#include <QStyle>
#include <QVBoxLayout>

namespace {
QLabel* label(const QString& text, const char* objectName, QWidget* parent) {
    auto* value = new QLabel(text, parent);
    value->setObjectName(QString::fromLatin1(objectName));
    return value;
}

QString localizePluginText(const QString& text) {
    static const QHash<QString, QString> values{
        {QStringLiteral("Gateway Configuration"), QStringLiteral("网关配置")},
        {QStringLiteral("Sandbox"), QStringLiteral("沙箱")},
        {QStringLiteral("Logging"), QStringLiteral("日志")},
        {QStringLiteral("Config"), QStringLiteral("配置")},
        {QStringLiteral("Runtime"), QStringLiteral("运行时")},
        {QStringLiteral("Asset Status"), QStringLiteral("资产状态")},
        {QStringLiteral("Basic Info"), QStringLiteral("基本信息")},
        {QStringLiteral("Process Info"), QStringLiteral("进程信息")},
        {QStringLiteral("Bind"), QStringLiteral("绑定地址")},
        {QStringLiteral("Port"), QStringLiteral("端口")},
        {QStringLiteral("Auth"), QStringLiteral("认证")},
        {QStringLiteral("Mode"), QStringLiteral("模式")},
        {QStringLiteral("Redact"), QStringLiteral("脱敏")},
        {QStringLiteral("Path"), QStringLiteral("路径")},
        {QStringLiteral("Runtime Listeners"), QStringLiteral("运行时监听")},
        {QStringLiteral("Listener"), QStringLiteral("监听地址")},
        {QStringLiteral("Listener Address"), QStringLiteral("监听地址")},
        {QStringLiteral("Installation"), QStringLiteral("安装信息")},
        {QStringLiteral("Root"), QStringLiteral("安装根目录")},
        {QStringLiteral("Status"), QStringLiteral("状态")},
        {QStringLiteral("Version"), QStringLiteral("版本号")},
        {QStringLiteral("Install Path"), QStringLiteral("安装路径")},
        {QStringLiteral("Config File"), QStringLiteral("配置文件")},
        {QStringLiteral("Log Path"), QStringLiteral("日志路径")},
        {QStringLiteral("Process Name"), QStringLiteral("进程名称")},
        {QStringLiteral("Main PID"), QStringLiteral("主进程 ID")},
        {QStringLiteral("PID"), QStringLiteral("进程 ID")},
        {QStringLiteral("Image Path"), QStringLiteral("可执行路径")},
        {QStringLiteral("Audit"), QStringLiteral("审计")},
        {QStringLiteral("Host"), QStringLiteral("主机")},
        {QStringLiteral("Pairing Required"), QStringLiteral("配对认证")},
        {QStringLiteral("Allow Public Bind"), QStringLiteral("允许公网绑定")},
        {QStringLiteral("Backend"), QStringLiteral("后端")},
        {QStringLiteral("Workspace Only"), QStringLiteral("仅工作区")},
        {QStringLiteral("Enabled"), QStringLiteral("启用状态")},
    };
    return values.value(text, text);
}

QString localizeValue(const QString& text) {
    static const QHash<QString, QString> values{
        {QStringLiteral("Enabled"), QStringLiteral("已启用")},
        {QStringLiteral("Disabled"), QStringLiteral("已禁用")},
        {QStringLiteral("Token"), QStringLiteral("Token 认证")},
        {QStringLiteral("Password"), QStringLiteral("密码认证")},
        {QStringLiteral("none"), QStringLiteral("未启用")},
        {QStringLiteral("on"), QStringLiteral("已开启")},
        {QStringLiteral("off"), QStringLiteral("已关闭")},
        {QStringLiteral("frontend_mode_running"), QStringLiteral("前端模式运行中")},
        {QStringLiteral("browser_mode_running"), QStringLiteral("浏览器模式运行中")},
        {QStringLiteral("cli_mode_running"), QStringLiteral("命令行模式运行中")},
        {QStringLiteral("installed_not_running"), QStringLiteral("已安装未运行")},
    };
    return values.value(text, text);
}

QString statusColor(const QString& status) {
    if (status == QStringLiteral("safe")) return QStringLiteral("#22C55E");
    if (status == QStringLiteral("danger")) return QStringLiteral("#EF4444");
    if (status == QStringLiteral("warning")) return QStringLiteral("#F59E0B");
    return QStringLiteral("rgba(255,255,255,180)");
}
}

AssetCardWidget::AssetCardWidget(const AssetModel& asset, bool initiallyExpanded, QWidget* parent)
    : QFrame(parent), asset_(asset), expanded_(initiallyExpanded) {
    setObjectName(QStringLiteral("assetCard"));
    setCursor(Qt::PointingHandCursor);
    buildUi();
    updateExpandedState();
    updateProtectionState();
}

const AssetModel& AssetCardWidget::asset() const { return asset_; }

void AssetCardWidget::setProtected(bool protectedState) {
    if (protected_ == protectedState) return;
    protected_ = protectedState;
    updateProtectionState();
}

void AssetCardWidget::buildUi() {
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(16, 16, 16, 16);
    root->setSpacing(0);

    header_ = new QPushButton(this);
    header_->setObjectName(QStringLiteral("assetCardHeader"));
    header_->setCursor(Qt::PointingHandCursor);
    header_->setMinimumHeight(36);
    header_->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Minimum);
    connect(qobject_cast<QPushButton*>(header_), &QPushButton::clicked, this, &AssetCardWidget::toggleExpanded);
    auto* headerLayout = new QVBoxLayout(header_);
    headerLayout->setContentsMargins(0, 0, 0, 0);
    headerLayout->setSpacing(4);
    auto* summary = new QHBoxLayout;
    summary->setContentsMargins(0, 0, 0, 0);
    summary->setSpacing(8);

    iconButton_ = new QPushButton(QStringLiteral("▣"), this);
    iconButton_->setObjectName(QStringLiteral("assetIconButton"));
    iconButton_->setFixedSize(36, 36);
    iconButton_->setToolTip(QStringLiteral("选择 Bot 图标"));
    connect(iconButton_, &QPushButton::clicked, this, &AssetCardWidget::iconPickerRequested);
    summary->addWidget(iconButton_);

    const QString titleText = asset_.version.trimmed().isEmpty() ? asset_.name : QStringLiteral("%1 | %2").arg(asset_.name, asset_.version);
    auto* title = label(titleText, "assetTitle", this);
    title->setSizePolicy(QSizePolicy::Expanding, QSizePolicy::Preferred);
    summary->addWidget(title, 1);
    statusBadge_ = label(QStringLiteral("未防护"), "pill", this);
    summary->addWidget(statusBadge_);
    summary->addWidget(label(assetTypeText(), "pill", this));
    chevron_ = label(QStringLiteral("›"), "assetChevron", this);
    chevron_->setAlignment(Qt::AlignCenter);
    chevron_->setFixedWidth(16);
    summary->addWidget(chevron_);
    headerLayout->addLayout(summary);

    const QString address = bindPortSummary();
    if (!address.isEmpty()) {
        auto* addressLabel = label(address, "assetAddress", this);
        addressLabel->setContentsMargins(48, 0, 0, 0);
        headerLayout->addWidget(addressLabel);
    }
    root->addWidget(header_);

    details_ = buildDetails();
    root->addWidget(details_);
}

void AssetCardWidget::setIconAppearance(const QString& iconName, const QString& glyph, const QColor& color) {
    iconName_ = iconName.isEmpty() ? QStringLiteral("package") : iconName;
    iconColor_ = color.isValid() ? color : QColor(QStringLiteral("#6366F1"));
    if (iconButton_ == nullptr) return;
    iconButton_->setText(glyph);
    iconButton_->setStyleSheet(QStringLiteral("color:%1;background:rgba(%2,%3,%4,35);")
                                   .arg(iconColor_.name())
                                   .arg(iconColor_.red())
                                   .arg(iconColor_.green())
                                   .arg(iconColor_.blue()));
}

QString AssetCardWidget::iconName() const { return iconName_; }

QColor AssetCardWidget::iconColor() const { return iconColor_; }

QWidget* AssetCardWidget::buildDetails() {
    auto* details = new QWidget(this);
    details->setAttribute(Qt::WA_TransparentForMouseEvents, false);
    details->setCursor(Qt::ArrowCursor);
    auto* layout = new QVBoxLayout(details);
    layout->setContentsMargins(0, 12, 0, 0);
    layout->setSpacing(0);

    if (!asset_.serviceName.isEmpty()) {
        auto* row = new QHBoxLayout;
        auto* key = label(QStringLiteral("服务名称"), "assetDetailKey", details);
        key->setFixedWidth(100);
        row->addWidget(key);
        row->addWidget(label(asset_.serviceName, "assetDetailValue", details), 1);
        layout->addLayout(row);
        layout->addSpacing(4);
    }

    const QJsonArray sections = asset_.raw.value(QStringLiteral("display_sections")).toArray();
    if (!sections.isEmpty()) {
        auto* divider = new QFrame(details);
        divider->setObjectName(QStringLiteral("assetDivider"));
        divider->setFrameShape(QFrame::HLine);
        layout->addWidget(divider);
        layout->addSpacing(10);
    }
    for (int sectionIndex = 0; sectionIndex < sections.size(); ++sectionIndex) {
        const QJsonObject section = sections.at(sectionIndex).toObject();
        if (sectionIndex > 0) layout->addSpacing(10);
        auto* heading = label(QStringLiteral("◇  %1").arg(localizePluginText(section.value(QStringLiteral("title")).toString())), "assetSectionTitle", details);
        layout->addWidget(heading);
        layout->addSpacing(7);
        for (const QJsonValue& itemValue : section.value(QStringLiteral("items")).toArray()) {
            const QJsonObject item = itemValue.toObject();
            auto* row = new QHBoxLayout;
            row->setContentsMargins(0, 0, 0, 0);
            auto* key = label(localizePluginText(item.value(QStringLiteral("label")).toString()), "assetDetailKey", details);
            key->setFixedWidth(100);
            row->addWidget(key);
            auto* value = label(localizeValue(item.value(QStringLiteral("value")).toVariant().toString()), "assetDetailValue", details);
            value->setWordWrap(true);
            value->setStyleSheet(QStringLiteral("color:%1;").arg(statusColor(item.value(QStringLiteral("status")).toString())));
            row->addWidget(value, 1);
            layout->addLayout(row);
            layout->addSpacing(4);
        }
    }

    actionsLayout_ = new QVBoxLayout;
    actionsLayout_->setContentsMargins(0, 12, 0, 0);
    actionsLayout_->setSpacing(8);
    layout->addLayout(actionsLayout_);
    return details;
}

void AssetCardWidget::updateProtectionState() {
    statusBadge_->setText(protected_ ? QStringLiteral("防护中") : QStringLiteral("未防护"));
    statusBadge_->setObjectName(protected_ ? QStringLiteral("successPill") : QStringLiteral("pill"));
    statusBadge_->style()->unpolish(statusBadge_);
    statusBadge_->style()->polish(statusBadge_);

    while (QLayoutItem* item = actionsLayout_->takeAt(0)) {
        if (item->widget() != nullptr) item->widget()->deleteLater();
        delete item;
    }
    auto* row = new QHBoxLayout;
    row->setContentsMargins(0, 0, 0, 0);
    row->setSpacing(8);
    if (!protected_) {
        auto* protect = new QPushButton(QStringLiteral("◇  一键防护"), details_);
        protect->setObjectName(QStringLiteral("assetPrimaryButton"));
        protect->setEnabled(isAssetRunning());
        if (!isAssetRunning()) protect->setToolTip(QStringLiteral("Bot 未运行，暂时无法开启防护"));
        connect(protect, &QPushButton::clicked, this, &AssetCardWidget::configureRequested);
        row->addWidget(protect);
        row->addStretch();
    } else {
        auto* monitor = new QPushButton(QStringLiteral("◇  防护监控"), details_);
        monitor->setObjectName(QStringLiteral("assetMonitorButton"));
        connect(monitor, &QPushButton::clicked, this, &AssetCardWidget::monitorRequested);
        row->addWidget(monitor);
        auto* stop = new QPushButton(QStringLiteral("◉  停止防护"), details_);
        stop->setObjectName(QStringLiteral("assetStopButton"));
        connect(stop, &QPushButton::clicked, this, &AssetCardWidget::stopRequested);
        row->addWidget(stop);
        auto* config = new QPushButton(QStringLiteral("⚙  配置"), details_);
        config->setObjectName(QStringLiteral("assetSecondaryButton"));
        connect(config, &QPushButton::clicked, this, &AssetCardWidget::configureRequested);
        row->addWidget(config);
        row->addStretch();
    }
    actionsLayout_->addLayout(row);
}

void AssetCardWidget::mousePressEvent(QMouseEvent* event) {
    if (event->button() == Qt::LeftButton && header_->geometry().contains(event->position().toPoint())) {
        toggleExpanded();
        event->accept();
        return;
    }
    QFrame::mousePressEvent(event);
}

void AssetCardWidget::toggleExpanded() {
    expanded_ = !expanded_;
    updateExpandedState();
}

void AssetCardWidget::updateExpandedState() {
    details_->setVisible(expanded_);
    chevron_->setText(expanded_ ? QStringLiteral("⌄") : QStringLiteral("›"));
}

QString AssetCardWidget::bindPortSummary() const {
    QStringList listeners;
    QString bind;
    for (const QJsonValue& sectionValue : asset_.raw.value(QStringLiteral("display_sections")).toArray()) {
        for (const QJsonValue& itemValue : sectionValue.toObject().value(QStringLiteral("items")).toArray()) {
            const QJsonObject item = itemValue.toObject();
            const QString key = item.value(QStringLiteral("label")).toString();
            const QString value = item.value(QStringLiteral("value")).toString();
            if ((key == QStringLiteral("Listener") || key == QStringLiteral("Listener Address")) && !value.isEmpty() && !value.startsWith(QStringLiteral("N/A"))) listeners.append(value);
            if (key == QStringLiteral("Bind") && bind.isEmpty()) bind = value;
        }
    }
    if (!listeners.isEmpty()) return listeners.join(QStringLiteral(", "));
    QStringList values;
    for (const QJsonValue& value : asset_.raw.value(QStringLiteral("ports")).toArray()) {
        values.append(QStringLiteral("%1:%2").arg(bind, QString::number(value.toInt())));
    }
    return values.join(QStringLiteral(", "));
}

QString AssetCardWidget::assetTypeText() const {
    if (asset_.type.compare(QStringLiteral("service"), Qt::CaseInsensitive) == 0) return QStringLiteral("服务");
    return asset_.type;
}

bool AssetCardWidget::isAssetRunning() const {
    const QString status = asset_.raw.value(QStringLiteral("metadata")).toObject().value(QStringLiteral("asset_status")).toString();
    return status.isEmpty() || status.endsWith(QStringLiteral("_running"));
}
