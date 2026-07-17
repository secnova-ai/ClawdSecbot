#include "ui/widgets/AuditTimelineWidget.h"

#include <QApplication>
#include <QClipboard>
#include <QDateTime>
#include <QFrame>
#include <QHBoxLayout>
#include <QLabel>
#include <QPushButton>
#include <QSizePolicy>
#include <QVBoxLayout>

#include <algorithm>

namespace {
struct TimelineItem {
    QString role;
    QString content;
};

QString displayAction(const QString& action) {
    const QString normalized = action.trimmed().toUpper();
    if (normalized == QStringLiteral("BLOCK") || normalized == QStringLiteral("HARD_BLOCK")) return QStringLiteral("已拦截");
    if (normalized == QStringLiteral("WARN")) return QStringLiteral("有风险");
    if (normalized == QStringLiteral("NEEDS_CONFIRMATION")) return QStringLiteral("需确认");
    return QStringLiteral("已允许");
}

QString displayRole(const QString& role) {
    const QString normalized = role.trimmed().toLower();
    if (normalized == QStringLiteral("user")) return QStringLiteral("User");
    if (normalized == QStringLiteral("assistant")) return QStringLiteral("Assistant");
    if (normalized == QStringLiteral("toolcall") || normalized == QStringLiteral("tool_call") || normalized == QStringLiteral("tool call")) return QStringLiteral("ToolCall");
    if (normalized == QStringLiteral("toolresult") || normalized == QStringLiteral("tool_result") || normalized == QStringLiteral("tool result") || normalized == QStringLiteral("tool")) return QStringLiteral("ToolResult");
    if (role.isEmpty()) return QStringLiteral("Unknown");
    QString result = role;
    result[0] = result[0].toUpper();
    return result;
}

QString roleStyle(const QString& role) {
    const QString normalized = displayRole(role).toLower();
    if (normalized == QStringLiteral("user")) return QStringLiteral("User");
    if (normalized == QStringLiteral("assistant")) return QStringLiteral("Assistant");
    if (normalized == QStringLiteral("toolcall")) return QStringLiteral("ToolCall");
    if (normalized == QStringLiteral("toolresult")) return QStringLiteral("ToolResult");
    return QStringLiteral("Unknown");
}

QString roleGlyph(const QString& style) {
    if (style == QStringLiteral("User")) return QStringLiteral("●");
    if (style == QStringLiteral("Assistant")) return QStringLiteral("◆");
    if (style == QStringLiteral("ToolCall")) return QStringLiteral("⌘");
    if (style == QStringLiteral("ToolResult")) return QStringLiteral("◇");
    return QStringLiteral("•");
}

QList<TimelineItem> toolTimeline(const QList<AuditToolCallModel>& calls, int startIndex = 0) {
    QList<TimelineItem> items;
    for (int index = startIndex; index < calls.size(); ++index) {
        const AuditToolCallModel& call = calls.at(index);
        const QString arguments = call.arguments.trimmed().isEmpty() ? QStringLiteral("{}") : call.arguments.trimmed();
        items.append({QStringLiteral("ToolCall"), QStringLiteral("工具调用: %1\n参数:\n%2").arg(call.name, arguments)});
        if (!call.result.trimmed().isEmpty()) items.append({QStringLiteral("ToolResult"), call.result.trimmed()});
    }
    return items;
}

QList<TimelineItem> timelineForLog(const AuditLogModel& log) {
    QList<AuditMessageModel> messages;
    for (const AuditMessageModel& message : log.messages) {
        if (message.role.trimmed().compare(QStringLiteral("system"), Qt::CaseInsensitive) != 0) messages.append(message);
    }
    std::stable_sort(messages.begin(), messages.end(), [](const AuditMessageModel& left, const AuditMessageModel& right) {
        return left.index < right.index;
    });

    QList<TimelineItem> items;
    if (messages.isEmpty()) {
        if (!log.requestContent.trimmed().isEmpty()) items.append({QStringLiteral("User"), log.requestContent.trimmed()});
        items.append(toolTimeline(log.toolCalls));
        if (!log.outputContent.trimmed().isEmpty()) items.append({QStringLiteral("Assistant"), log.outputContent.trimmed()});
        return items;
    }

    const bool hasExplicitTools = std::any_of(messages.cbegin(), messages.cend(), [](const AuditMessageModel& message) {
        const QString role = message.role.trimmed().toLower();
        return role == QStringLiteral("tool") || role == QStringLiteral("toolcall") || role == QStringLiteral("tool_call");
    });
    int toolIndex = 0;
    for (const AuditMessageModel& message : messages) {
        if (!hasExplicitTools && message.role.trimmed().compare(QStringLiteral("assistant"), Qt::CaseInsensitive) == 0 && toolIndex < log.toolCalls.size()) {
            items.append(toolTimeline(log.toolCalls, toolIndex));
            toolIndex = log.toolCalls.size();
        }
        items.append({displayRole(message.role), message.content.trimmed().isEmpty() ? QStringLiteral("（空内容）") : message.content.trimmed()});
    }
    if (!hasExplicitTools && toolIndex < log.toolCalls.size()) items.append(toolTimeline(log.toolCalls, toolIndex));
    return items;
}

QString tickLabel(int index, int total) {
    if (index == 0) return QStringLiteral("开始");
    if (index == total - 1) return QStringLiteral("结束");
    return QStringLiteral("步骤 %1").arg(index + 1);
}

QString displayTimestamp(const QString& raw) {
    QDateTime timestamp = QDateTime::fromString(raw, Qt::ISODateWithMs);
    if (!timestamp.isValid()) timestamp = QDateTime::fromString(raw, Qt::ISODate);
    return timestamp.isValid() ? timestamp.toLocalTime().toString(QStringLiteral("HH:mm:ss.zzz")) : raw;
}

QWidget* detailRow(const QString& label, const QString& value, QWidget* parent) {
    auto* row = new QWidget(parent);
    auto* layout = new QHBoxLayout(row);
    layout->setContentsMargins(0, 0, 0, 0);
    layout->setSpacing(10);
    auto* labelWidget = new QLabel(label, row);
    labelWidget->setObjectName(QStringLiteral("auditDetailLabel"));
    labelWidget->setFixedWidth(86);
    QString displayValue = value.isEmpty() ? QStringLiteral("-") : value;
    if (displayValue.size() > 100) {
        const QString softBreak(QChar(0x200B));
        displayValue.replace(QLatin1Char(','), QStringLiteral(",") + softBreak);
        displayValue.replace(QLatin1Char(':'), QStringLiteral(":") + softBreak);
        displayValue.replace(QLatin1Char('/'), QStringLiteral("/") + softBreak);
        displayValue.replace(QLatin1Char('}'), QStringLiteral("}") + softBreak);
    }
    auto* valueWidget = new QLabel(displayValue, row);
    valueWidget->setObjectName(QStringLiteral("auditDetailValue"));
    valueWidget->setWordWrap(true);
    valueWidget->setTextInteractionFlags(Qt::TextSelectableByMouse);
    layout->addWidget(labelWidget, 0, Qt::AlignTop);
    layout->addWidget(valueWidget, 1);
    return row;
}

QWidget* sectionHeader(const QString& title, const QString& copyText, QWidget* parent) {
    auto* row = new QWidget(parent);
    auto* layout = new QHBoxLayout(row);
    layout->setContentsMargins(0, 0, 0, 0);
    auto* titleLabel = new QLabel(title, row);
    titleLabel->setObjectName(QStringLiteral("auditSectionTitle"));
    layout->addWidget(titleLabel);
    auto* copy = new QPushButton(QStringLiteral("▣"), row);
    copy->setObjectName(QStringLiteral("auditCopyButton"));
    copy->setToolTip(QStringLiteral("复制本节内容"));
    QObject::connect(copy, &QPushButton::clicked, row, [copyText]() { QApplication::clipboard()->setText(copyText); });
    layout->addWidget(copy);
    layout->addStretch();
    return row;
}
}

AuditTimelineWidget::AuditTimelineWidget(QWidget* parent) : QWidget(parent) {
    setObjectName(QStringLiteral("auditTimelineView"));
    contentLayout_ = new QVBoxLayout(this);
    contentLayout_->setContentsMargins(0, 0, 0, 0);
    contentLayout_->setSpacing(12);
}

void AuditTimelineWidget::clearContent() {
    while (QLayoutItem* item = contentLayout_->takeAt(0)) {
        if (item->widget() != nullptr) item->widget()->deleteLater();
        delete item;
    }
}

void AuditTimelineWidget::setLog(const AuditLogModel& log, const QList<SecurityEventModel>& securityEvents) {
    clearContent();

    auto* metadata = new QFrame(this);
    metadata->setObjectName(QStringLiteral("auditMetadataCard"));
    auto* metadataLayout = new QVBoxLayout(metadata);
    metadataLayout->setContentsMargins(12, 12, 12, 12);
    metadataLayout->setSpacing(7);
    const QDateTime timestamp = QDateTime::fromString(log.timestamp, Qt::ISODateWithMs);
    metadataLayout->addWidget(detailRow(QStringLiteral("日志 ID"), log.id, metadata));
    metadataLayout->addWidget(detailRow(QStringLiteral("时间"), timestamp.isValid() ? timestamp.toLocalTime().toString(QStringLiteral("yyyy-MM-dd HH:mm:ss")) : log.timestamp, metadata));
    metadataLayout->addWidget(detailRow(QStringLiteral("请求 ID"), log.requestId, metadata));
    if (!log.instructionChainId.isEmpty()) {
        metadataLayout->addWidget(detailRow(QStringLiteral("链路 ID"), log.instructionChainId, metadata));
    }
    metadataLayout->addWidget(detailRow(QStringLiteral("模型"), log.model, metadata));
    metadataLayout->addWidget(detailRow(QStringLiteral("动作"), displayAction(log.action), metadata));
    if (log.hasRisk) {
        metadataLayout->addWidget(detailRow(QStringLiteral("风险等级"), log.riskLevel.isEmpty() ? QStringLiteral("N/A") : log.riskLevel, metadata));
        metadataLayout->addWidget(detailRow(QStringLiteral("风险说明"), log.riskReason.isEmpty() ? QStringLiteral("N/A") : log.riskReason, metadata));
    }
    metadataLayout->addWidget(detailRow(QStringLiteral("耗时"), QStringLiteral("%1 ms").arg(log.durationMs), metadata));
    metadataLayout->addWidget(detailRow(QStringLiteral("Token"), QString::number(log.totalTokens), metadata));
    contentLayout_->addWidget(metadata);

    const QList<TimelineItem> items = timelineForLog(log);
    QStringList rawLines;
    for (const TimelineItem& item : items) rawLines.append(QStringLiteral("[%1]\n%2").arg(displayRole(item.role), item.content));
    contentLayout_->addWidget(sectionHeader(QStringLiteral("原始 (%1)").arg(items.size()), rawLines.join(QStringLiteral("\n\n")), this));

    auto* timeline = new QFrame(this);
    timeline->setObjectName(QStringLiteral("auditTimeline"));
    auto* timelineLayout = new QVBoxLayout(timeline);
    timelineLayout->setContentsMargins(10, 10, 10, 4);
    timelineLayout->setSpacing(0);
    for (int index = 0; index < items.size(); ++index) {
        const TimelineItem& item = items.at(index);
        const QString style = roleStyle(item.role);
        auto* row = new QWidget(timeline);
        row->setObjectName(QStringLiteral("auditTimelineRow"));
        auto* rowLayout = new QHBoxLayout(row);
        rowLayout->setContentsMargins(0, 0, 0, 8);
        rowLayout->setSpacing(8);

        auto* rail = new QWidget(row);
        rail->setFixedWidth(72);
        rail->setSizePolicy(QSizePolicy::Fixed, QSizePolicy::Expanding);
        auto* railLayout = new QVBoxLayout(rail);
        railLayout->setContentsMargins(0, 0, 0, 0);
        railLayout->setSpacing(4);
        auto* tick = new QLabel(tickLabel(index, items.size()), rail);
        tick->setObjectName(QStringLiteral("auditTimelineTick"));
        tick->setAlignment(Qt::AlignHCenter);
        railLayout->addWidget(tick);
        auto* node = new QLabel(roleGlyph(style), rail);
        node->setObjectName(QStringLiteral("auditTimelineNode%1").arg(style));
        node->setAlignment(Qt::AlignCenter);
        node->setFixedSize(24, 24);
        railLayout->addWidget(node, 0, Qt::AlignHCenter);
        if (index < items.size() - 1) {
            auto* connector = new QFrame(rail);
            connector->setObjectName(QStringLiteral("auditTimelineConnector%1").arg(style));
            connector->setFixedWidth(2);
            connector->setMinimumHeight(18);
            connector->setSizePolicy(QSizePolicy::Fixed, QSizePolicy::Expanding);
            railLayout->addWidget(connector, 1, Qt::AlignHCenter);
        } else {
            railLayout->addStretch();
        }
        rowLayout->addWidget(rail);

        auto* card = new QFrame(row);
        card->setObjectName(QStringLiteral("auditTimelineCard%1").arg(style));
        card->setProperty("timelineRole", displayRole(item.role));
        card->setProperty("timelineIndex", index);
        auto* cardLayout = new QVBoxLayout(card);
        cardLayout->setContentsMargins(10, 9, 10, 10);
        cardLayout->setSpacing(6);
        auto* cardHeader = new QHBoxLayout;
        auto* role = new QLabel(displayRole(item.role), card);
        role->setObjectName(QStringLiteral("auditTimelineRole%1").arg(style));
        auto* sequence = new QLabel(QStringLiteral("#%1").arg(index + 1), card);
        sequence->setObjectName(QStringLiteral("auditTimelineBadge%1").arg(style));
        cardHeader->addWidget(role);
        cardHeader->addWidget(sequence);
        cardHeader->addStretch();
        cardLayout->addLayout(cardHeader);
        auto* content = new QLabel(item.content, card);
        content->setObjectName(QStringLiteral("auditTimelineContent"));
        content->setWordWrap(true);
        content->setTextInteractionFlags(Qt::TextSelectableByMouse);
        cardLayout->addWidget(content);
        rowLayout->addWidget(card, 1);
        timelineLayout->addWidget(row);
    }
    if (items.isEmpty()) {
        auto* empty = new QLabel(QStringLiteral("暂无可回放的消息或工具调用"), timeline);
        empty->setObjectName(QStringLiteral("subtle"));
        empty->setAlignment(Qt::AlignCenter);
        timelineLayout->addWidget(empty);
    }
    contentLayout_->addWidget(timeline);

    contentLayout_->addWidget(sectionHeader(
        QStringLiteral("关联安全事件 (%1)").arg(securityEvents.size()),
        [&securityEvents]() {
            QStringList lines;
            for (const SecurityEventModel& event : securityEvents) {
                lines.append(QStringLiteral("[%1] %2\n%3\n%4")
                                 .arg(event.timestamp, event.eventType, event.actionDescription, event.detail));
            }
            return lines.join(QStringLiteral("\n\n"));
        }(),
        this));

    auto* eventTimeline = new QFrame(this);
    eventTimeline->setObjectName(QStringLiteral("auditSecurityEventTimeline"));
    auto* eventLayout = new QVBoxLayout(eventTimeline);
    eventLayout->setContentsMargins(10, 10, 10, 4);
    eventLayout->setSpacing(0);
    for (int index = 0; index < securityEvents.size(); ++index) {
        const SecurityEventModel& event = securityEvents.at(index);
        auto* row = new QWidget(eventTimeline);
        row->setObjectName(QStringLiteral("auditTimelineRow"));
        auto* rowLayout = new QHBoxLayout(row);
        rowLayout->setContentsMargins(0, 0, 0, 8);
        rowLayout->setSpacing(8);

        auto* rail = new QWidget(row);
        rail->setFixedWidth(88);
        auto* railLayout = new QVBoxLayout(rail);
        railLayout->setContentsMargins(0, 0, 0, 0);
        railLayout->setSpacing(4);
        auto* timestamp = new QLabel(displayTimestamp(event.timestamp), rail);
        timestamp->setObjectName(QStringLiteral("auditSecurityEventTime"));
        timestamp->setProperty("exactTimestamp", true);
        timestamp->setAlignment(Qt::AlignHCenter);
        railLayout->addWidget(timestamp);
        auto* node = new QLabel(QStringLiteral("!"), rail);
        node->setObjectName(QStringLiteral("auditSecurityEventNode"));
        node->setAlignment(Qt::AlignCenter);
        node->setFixedSize(24, 24);
        railLayout->addWidget(node, 0, Qt::AlignHCenter);
        if (index < securityEvents.size() - 1) {
            auto* connector = new QFrame(rail);
            connector->setObjectName(QStringLiteral("auditSecurityEventConnector"));
            connector->setFixedWidth(2);
            connector->setMinimumHeight(18);
            connector->setSizePolicy(QSizePolicy::Fixed, QSizePolicy::Expanding);
            railLayout->addWidget(connector, 1, Qt::AlignHCenter);
        } else {
            railLayout->addStretch();
        }
        rowLayout->addWidget(rail);

        auto* card = new QFrame(row);
        card->setObjectName(QStringLiteral("auditSecurityEventCard"));
        card->setProperty("securityEventIndex", index);
        auto* cardLayout = new QVBoxLayout(card);
        cardLayout->setContentsMargins(10, 9, 10, 10);
        cardLayout->setSpacing(6);
        auto* header = new QLabel(
            QStringLiteral("%1 · %2")
                .arg(event.eventType.isEmpty() ? QStringLiteral("安全事件") : event.eventType,
                     event.riskType.isEmpty() ? QStringLiteral("未分类") : event.riskType),
            card);
        header->setObjectName(QStringLiteral("auditSecurityEventTitle"));
        cardLayout->addWidget(header);
        if (!event.actionDescription.isEmpty()) {
            cardLayout->addWidget(detailRow(QStringLiteral("动作"), event.actionDescription, card));
        }
        if (!event.detail.isEmpty()) cardLayout->addWidget(detailRow(QStringLiteral("详情"), event.detail, card));
        if (!event.source.isEmpty()) cardLayout->addWidget(detailRow(QStringLiteral("来源"), event.source, card));
        rowLayout->addWidget(card, 1);
        eventLayout->addWidget(row);
    }
    if (securityEvents.isEmpty()) {
        auto* empty = new QLabel(QStringLiteral("该请求没有关联的安全事件"), eventTimeline);
        empty->setObjectName(QStringLiteral("subtle"));
        empty->setAlignment(Qt::AlignCenter);
        eventLayout->addWidget(empty);
    }
    contentLayout_->addWidget(eventTimeline);

    if (!log.toolCalls.isEmpty()) {
        QStringList toolText;
        for (const AuditToolCallModel& call : log.toolCalls) toolText.append(QStringLiteral("%1\n%2\n%3").arg(call.name, call.arguments, call.result));
        contentLayout_->addWidget(sectionHeader(QStringLiteral("工具调用 (%1)").arg(log.toolCalls.size()), toolText.join(QStringLiteral("\n\n")), this));
        for (const AuditToolCallModel& call : log.toolCalls) {
            auto* card = new QFrame(this);
            card->setObjectName(call.sensitive ? QStringLiteral("auditToolCardSensitive") : QStringLiteral("auditToolCard"));
            auto* layout = new QVBoxLayout(card);
            layout->setContentsMargins(11, 10, 11, 10);
            layout->setSpacing(6);
            auto* name = new QLabel(QStringLiteral("⌘  %1%2").arg(call.name, call.sensitive ? QStringLiteral("  ·  敏感") : QString()), card);
            name->setObjectName(call.sensitive ? QStringLiteral("auditToolNameSensitive") : QStringLiteral("auditToolName"));
            layout->addWidget(name);
            if (!call.arguments.isEmpty()) layout->addWidget(detailRow(QStringLiteral("参数"), call.arguments, card));
            if (!call.result.isEmpty()) layout->addWidget(detailRow(QStringLiteral("结果"), call.result, card));
            contentLayout_->addWidget(card);
        }
    }
    contentLayout_->addStretch();
}
