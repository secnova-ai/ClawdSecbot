#include "ui/widgets/SecurityEventListWidget.h"

#include "ui/DialogChrome.h"

#include <QApplication>
#include <QClipboard>
#include <QColor>
#include <QDateTime>
#include <QDialog>
#include <QFrame>
#include <QHBoxLayout>
#include <QJsonDocument>
#include <QLabel>
#include <QPushButton>
#include <QScrollArea>
#include <QVBoxLayout>

namespace {
QColor eventColor(const QString& type) {
    if (type == QStringLiteral("blocked")) return QColor(QStringLiteral("#EF4444"));
    if (type == QStringLiteral("needs_confirmation")) return QColor(QStringLiteral("#F59E0B"));
    if (type == QStringLiteral("tool_execution")) return QColor(QStringLiteral("#6366F1"));
    return QColor(QStringLiteral("#F59E0B"));
}

QString eventGlyph(const QString& type) {
    if (type == QStringLiteral("blocked")) return QStringLiteral("◇");
    if (type == QStringLiteral("needs_confirmation")) return QStringLiteral("△");
    if (type == QStringLiteral("tool_execution")) return QStringLiteral("⌘");
    return QStringLiteral("△");
}

QString eventTypeLabel(const QString& raw) {
    const QString value = raw.trimmed().toLower();
    if (value == QStringLiteral("blocked")) return QStringLiteral("已拦截");
    if (value == QStringLiteral("needs_confirmation")) return QStringLiteral("需要确认");
    if (value == QStringLiteral("tool_execution")) return QStringLiteral("工具执行");
    if (value == QStringLiteral("rewritten")) return QStringLiteral("已重写");
    if (value == QStringLiteral("redacted")) return QStringLiteral("已脱敏");
    if (value == QStringLiteral("allowed")) return QStringLiteral("已允许");
    if (value == QStringLiteral("warning")) return QStringLiteral("警告");
    if (value == QStringLiteral("other")) return QStringLiteral("其他事件");
    return raw;
}

QString riskLabel(const QString& raw) {
    const QString value = raw.trimmed().toUpper();
    const QHash<QString, QString> labels{
        {QStringLiteral("PROMPT_INJECTION_DIRECT"), QStringLiteral("直接提示词注入")},
        {QStringLiteral("PROMPT_INJECTION_INDIRECT"), QStringLiteral("间接提示词注入")},
        {QStringLiteral("SENSITIVE_DATA_EXFILTRATION"), QStringLiteral("敏感数据外泄")},
        {QStringLiteral("HIGH_RISK_OPERATION"), QStringLiteral("高危操作")},
        {QStringLiteral("PRIVILEGE_ABUSE"), QStringLiteral("权限滥用")},
        {QStringLiteral("UNEXPECTED_CODE_EXECUTION"), QStringLiteral("非预期代码执行")},
        {QStringLiteral("CONTEXT_POISONING"), QStringLiteral("上下文污染")},
        {QStringLiteral("SUPPLY_CHAIN_RISK"), QStringLiteral("供应链风险")},
        {QStringLiteral("HUMAN_TRUST_EXPLOITATION"), QStringLiteral("人类信任利用")},
        {QStringLiteral("CASCADING_FAILURE"), QStringLiteral("级联故障风险")},
        {QStringLiteral("QUOTA"), QStringLiteral("Token 配额")},
        {QStringLiteral("SANDBOX_BLOCKED"), QStringLiteral("沙箱拦截")},
        {QStringLiteral("NEEDS_CONFIRMATION"), QStringLiteral("需要确认")}};
    return labels.value(value, raw);
}

QString actionLabel(const QString& raw) {
    const QHash<QString, QString> labels{
        {QStringLiteral("Historical blocked tool result rewritten"), QStringLiteral("历史已拦截工具结果已重写")},
        {QStringLiteral("Conversation token quota exceeded"), QStringLiteral("会话 Token 配额已超限")},
        {QStringLiteral("Daily token quota exceeded"), QStringLiteral("每日 Token 配额已超限")},
        {QStringLiteral("Final result references quarantined tool result"), QStringLiteral("最终输出引用了已隔离的工具结果")},
        {QStringLiteral("Final result sensitive data redacted"), QStringLiteral("最终输出中的敏感数据已脱敏")},
        {QStringLiteral("Risk detected by security detector"), QStringLiteral("安全检测器发现风险")},
        {QStringLiteral("User input risk detected by ShepherdGate semantic analysis"), QStringLiteral("ShepherdGate 语义分析发现用户输入风险")},
        {QStringLiteral("Tool call risk detected by ShepherdGate ReAct analysis"), QStringLiteral("ShepherdGate ReAct 分析发现工具调用风险")},
        {QStringLiteral("Guard output format violation requires human confirmation."), QStringLiteral("安全模型输出格式异常，需要人工确认")},
        {QStringLiteral("Tool call matches user-defined semantic rule"), QStringLiteral("工具调用命中用户自定义语义规则")},
        {QStringLiteral("Final output violates user rule"), QStringLiteral("最终输出违反用户自定义规则")},
        {QStringLiteral("Direct prompt injection in user input"), QStringLiteral("用户输入存在直接提示词注入")}};
    return labels.value(raw.trimmed(), raw.trimmed());
}

QString localTime(const QString& raw) {
    QDateTime parsed = QDateTime::fromString(raw, Qt::ISODateWithMs);
    if (!parsed.isValid()) parsed = QDateTime::fromString(raw, Qt::ISODate);
    return parsed.isValid() ? parsed.toLocalTime().toString(QStringLiteral("yyyy-MM-dd HH:mm:ss")) : raw;
}

QWidget* eventCard(const QJsonObject& event, QWidget* parent) {
    const QString type = event.value(QStringLiteral("event_type")).toString(QStringLiteral("other"));
    const QColor color = eventColor(type);
    auto* card = new QFrame(parent);
    card->setObjectName(QStringLiteral("monitorEventCard"));
    card->setStyleSheet(QStringLiteral(
        "QFrame#monitorEventCard{background:rgba(255,255,255,8);border:0;border-left:2px solid %1;border-radius:6px;}"
        "QLabel{background:transparent;border:0;}").arg(color.name()));
    auto* layout = new QVBoxLayout(card);
    layout->setContentsMargins(8, 7, 8, 7);
    layout->setSpacing(4);
    auto* firstRow = new QWidget(card);
    auto* firstLayout = new QHBoxLayout(firstRow);
    firstLayout->setContentsMargins(0, 0, 0, 0);
    firstLayout->setSpacing(6);
    auto* icon = new QLabel(eventGlyph(type), firstRow);
    icon->setObjectName(QStringLiteral("monitorEventTypeIcon"));
    icon->setStyleSheet(QStringLiteral("color:%1;").arg(color.name()));
    firstLayout->addWidget(icon, 0, Qt::AlignTop);
    const QString rawAction = event.value(QStringLiteral("action_desc")).toString();
    auto* action = new QLabel(rawAction.isEmpty() ? eventTypeLabel(type) : actionLabel(rawAction), firstRow);
    action->setObjectName(QStringLiteral("monitorEventAction"));
    action->setWordWrap(true);
    firstLayout->addWidget(action, 1);
    const bool agent = event.value(QStringLiteral("source")).toString() == QStringLiteral("react_agent");
    auto* source = new QLabel(agent ? QStringLiteral("AI") : QStringLiteral("H"), firstRow);
    source->setObjectName(agent ? QStringLiteral("monitorEventSourceAgent") : QStringLiteral("monitorEventSourceHeuristic"));
    firstLayout->addWidget(source, 0, Qt::AlignTop);
    layout->addWidget(firstRow);
    auto* secondRow = new QWidget(card);
    auto* secondLayout = new QHBoxLayout(secondRow);
    secondLayout->setContentsMargins(19, 0, 0, 0);
    secondLayout->setSpacing(6);
    const QString risk = event.value(QStringLiteral("risk_type")).toString();
    if (!risk.isEmpty()) {
        auto* badge = new QLabel(riskLabel(risk), secondRow);
        badge->setObjectName(QStringLiteral("monitorEventRiskBadge"));
        badge->setStyleSheet(QStringLiteral("color:%1;background:rgba(%2,%3,%4,38);border-radius:3px;padding:1px 5px;")
                                 .arg(color.name()).arg(color.red()).arg(color.green()).arg(color.blue()));
        secondLayout->addWidget(badge);
    }
    auto* time = new QLabel(localTime(event.value(QStringLiteral("timestamp")).toString()), secondRow);
    time->setObjectName(QStringLiteral("monitorEventTime"));
    secondLayout->addWidget(time);
    secondLayout->addStretch();
    layout->addWidget(secondRow);
    return card;
}

void addDetailRow(QVBoxLayout* layout, const QString& label, const QString& value, QWidget* parent) {
    if (value.trimmed().isEmpty()) return;
    auto* block = new QWidget(parent);
    auto* blockLayout = new QVBoxLayout(block);
    blockLayout->setContentsMargins(0, 0, 0, 0);
    blockLayout->setSpacing(2);
    auto* key = new QLabel(label, block);
    key->setObjectName(QStringLiteral("eventDetailKey"));
    blockLayout->addWidget(key);
    auto* text = new QLabel(value, block);
    text->setObjectName(QStringLiteral("eventDetailValue"));
    text->setWordWrap(true);
    text->setTextInteractionFlags(Qt::TextSelectableByMouse);
    blockLayout->addWidget(text);
    layout->addWidget(block);
}
}

SecurityEventListWidget::SecurityEventListWidget(QWidget* parent) : QListWidget(parent) {
    setObjectName(QStringLiteral("securityEventList"));
    setWordWrap(true);
    setSelectionMode(QAbstractItemView::NoSelection);
    setVerticalScrollMode(QAbstractItemView::ScrollPerPixel);
    connect(this, &QListWidget::itemClicked, this, [this](QListWidgetItem* item) {
        const QJsonObject event = QJsonDocument::fromJson(item->data(Qt::UserRole).toByteArray()).object();
        if (!event.isEmpty()) emit eventActivated(event);
    });
}

void SecurityEventListWidget::setEvents(const QJsonArray& events) {
    clear();
    for (const QJsonValue& value : events) {
        const QJsonObject event = value.toObject();
        auto* item = new QListWidgetItem;
        item->setData(Qt::UserRole, QJsonDocument(event).toJson(QJsonDocument::Compact));
        QWidget* card = eventCard(event, this);
        card->adjustSize();
        item->setSizeHint(QSize(0, qMax(64, card->sizeHint().height() + 4)));
        addItem(item);
        setItemWidget(item, card);
    }
    emit eventCountChanged(count());
}

int SecurityEventListWidget::eventCount() const { return count(); }

QDialog* showSecurityEventDetail(const QJsonObject& event, QWidget* parent) {
    const QString type = event.value(QStringLiteral("event_type")).toString(QStringLiteral("other"));
    const DialogChrome::Tone tone = type == QStringLiteral("blocked") ? DialogChrome::Tone::Danger
        : type == QStringLiteral("tool_execution") ? DialogChrome::Tone::Info : DialogChrome::Tone::Warning;
    auto* dialog = new QDialog(parent);
    dialog->setAttribute(Qt::WA_DeleteOnClose);
    dialog->setWindowTitle(QStringLiteral("安全事件详情"));
    dialog->setProperty("tone", DialogChrome::toneName(tone));
    DialogChrome::prepare(dialog, QSize(540, 520), QSize(480, 420));
    auto* root = new QVBoxLayout(dialog);
    root->setContentsMargins(26, 24, 26, 22);
    root->setSpacing(18);
    root->addWidget(DialogChrome::createHeader(dialog, eventGlyph(type), QStringLiteral("安全事件详情"),
                                                QStringLiteral("%1 · %2").arg(eventTypeLabel(type), riskLabel(event.value(QStringLiteral("risk_type")).toString())), tone));
    auto* scroll = new QScrollArea(dialog);
    scroll->setObjectName(QStringLiteral("dialogDetailScroll"));
    scroll->setWidgetResizable(true);
    auto* body = new QWidget(scroll);
    body->setObjectName(QStringLiteral("dialogDetailBody"));
    auto* bodyLayout = new QVBoxLayout(body);
    bodyLayout->setContentsMargins(0, 0, 8, 0);
    bodyLayout->setSpacing(10);
    addDetailRow(bodyLayout, QStringLiteral("时间"), localTime(event.value(QStringLiteral("timestamp")).toString()), body);
    addDetailRow(bodyLayout, QStringLiteral("动作"), actionLabel(event.value(QStringLiteral("action_desc")).toString()), body);
    addDetailRow(bodyLayout, QStringLiteral("风险类型"), riskLabel(event.value(QStringLiteral("risk_type")).toString()), body);
    addDetailRow(bodyLayout, QStringLiteral("来源"), event.value(QStringLiteral("source")).toString() == QStringLiteral("react_agent") ? QStringLiteral("ReAct Agent") : QStringLiteral("启发式检测"), body);
    addDetailRow(bodyLayout, QStringLiteral("事件类型"), eventTypeLabel(type), body);
    addDetailRow(bodyLayout, QStringLiteral("详情"), event.value(QStringLiteral("detail")).toString(), body);
    addDetailRow(bodyLayout, QStringLiteral("ID"), event.value(QStringLiteral("id")).toVariant().toString(), body);
    bodyLayout->addStretch();
    scroll->setWidget(body);
    root->addWidget(scroll, 1);
    auto* footer = new QHBoxLayout;
    footer->addStretch();
    auto* copy = new QPushButton(QStringLiteral("复制事件信息"), dialog);
    copy->setObjectName(QStringLiteral("secondaryButton"));
    footer->addWidget(copy);
    root->addLayout(footer);
    QObject::connect(copy, &QPushButton::clicked, dialog, [event]() {
        QStringList lines{
            QStringLiteral("Security Event: %1").arg(event.value(QStringLiteral("id")).toVariant().toString()),
            QStringLiteral("Time: %1").arg(localTime(event.value(QStringLiteral("timestamp")).toString())),
            QStringLiteral("Type: %1").arg(eventTypeLabel(event.value(QStringLiteral("event_type")).toString())),
            QStringLiteral("Action: %1").arg(actionLabel(event.value(QStringLiteral("action_desc")).toString())),
            QStringLiteral("Risk: %1").arg(riskLabel(event.value(QStringLiteral("risk_type")).toString())),
            QStringLiteral("Source: %1").arg(event.value(QStringLiteral("source")).toString()),
            QStringLiteral("Detail: %1").arg(event.value(QStringLiteral("detail")).toString())};
        QApplication::clipboard()->setText(lines.join(QLatin1Char('\n')));
    });
    dialog->open();
    return dialog;
}
