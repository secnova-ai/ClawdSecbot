#include "ui/widgets/AnalysisLogView.h"

#include <QColor>
#include <QDateTime>
#include <QFrame>
#include <QHBoxLayout>
#include <QJsonDocument>
#include <QLabel>
#include <QRegularExpression>
#include <QTextCharFormat>
#include <QTextCursor>
#include <QVBoxLayout>

#include <algorithm>

namespace {
constexpr qsizetype kMaxRenderedRecords = 100;

QString clipped(QString value, const qsizetype limit = 360) {
    value = value.trimmed();
    if (value.size() <= limit) return value;
    return value.left(limit).trimmed() + QStringLiteral("…");
}

QString sanitize(QString value) {
    static const QList<QRegularExpression> blocks{
        QRegularExpression(QStringLiteral("<thinking>[\\s\\S]*?</thinking>"), QRegularExpression::CaseInsensitiveOption),
        QRegularExpression(QStringLiteral("<tool_use>[\\s\\S]*?</tool_use>"), QRegularExpression::CaseInsensitiveOption),
        QRegularExpression(QStringLiteral("<tool_result>[\\s\\S]*?</tool_result>"), QRegularExpression::CaseInsensitiveOption)};
    for (const QRegularExpression& expression : blocks) value.remove(expression);
    value.remove(QRegularExpression(QStringLiteral("</?(summary|analysis|final)>"), QRegularExpression::CaseInsensitiveOption));
    value.replace(QRegularExpression(QStringLiteral("\\n{3,}")), QStringLiteral("\n\n"));
    return clipped(value);
}

QString localTime(const QString& raw) {
    QDateTime parsed = QDateTime::fromString(raw, Qt::ISODateWithMs);
    if (!parsed.isValid()) parsed = QDateTime::fromString(raw, Qt::ISODate);
    if (!parsed.isValid()) return raw;
    return parsed.toLocalTime().toString(QStringLiteral("yyyy-MM-dd HH:mm:ss"));
}

QColor rawLogColor(const QString& text) {
    if (text.contains(QStringLiteral("Error")) || text.contains(QStringLiteral("BLOCKED")) || text.contains(QStringLiteral("CRITICAL"))) {
        return QColor(QStringLiteral("#EF4444"));
    }
    if (text.contains(QStringLiteral("Warning")) || text.contains(QStringLiteral("DANGEROUS")) || text.contains(QStringLiteral("SUSPICIOUS"))) {
        return QColor(QStringLiteral("#F59E0B"));
    }
    if (text.contains(QStringLiteral("SAFE")) || text.contains(QStringLiteral("ALLOW"))) return QColor(QStringLiteral("#22C55E"));
    if (text.contains(QStringLiteral("[Protection Agent]"))) return QColor(QStringLiteral("#818CF8"));
    return QColor(255, 255, 255, 179);
}

bool isNormalDecision(const QString& value) {
    const QString status = value.trimmed().toUpper();
    return status == QStringLiteral("ALLOW") || status == QStringLiteral("ALLOWED") || status == QStringLiteral("COMPLETED");
}

struct Presentation {
    QString text;
    QColor color;
};

Presentation statusPresentation(const QJsonObject& record) {
    const QJsonObject decision = record.value(QStringLiteral("decision")).toObject();
    const QString action = decision.value(QStringLiteral("action")).toString().trimmed().toUpper();
    if (action == QStringLiteral("BLOCK") || action == QStringLiteral("HARD_BLOCK")) {
        return {QStringLiteral("已拦截"), QColor(QStringLiteral("#EF4444"))};
    }
    if (!action.isEmpty() && !isNormalDecision(action) && action != QStringLiteral("QUOTA_EXCEEDED")) {
        return {action, QColor(QStringLiteral("#F59E0B"))};
    }
    const QString phase = record.value(QStringLiteral("phase")).toString().trimmed().toLower();
    if (phase == QStringLiteral("completed")) return {QStringLiteral("已完成"), QColor(QStringLiteral("#10B981"))};
    if (phase == QStringLiteral("stopped")) return {QStringLiteral("已停止"), QColor(QStringLiteral("#94A3B8"))};
    for (const QJsonValue& value : record.value(QStringLiteral("tool_calls")).toArray()) {
        if (value.toObject().value(QStringLiteral("source")).toString() == QStringLiteral("response")) {
            return {QStringLiteral("工具调用中"), QColor(QStringLiteral("#EC4899"))};
        }
    }
    if (phase == QStringLiteral("starting")) return {QStringLiteral("处理中"), QColor(QStringLiteral("#3B82F6"))};
    return {phase.isEmpty() ? QStringLiteral("活动中") : phase.toUpper(), QColor(QStringLiteral("#94A3B8"))};
}

QLabel* textLabel(const QString& text, const QString& objectName, QWidget* parent) {
    auto* label = new QLabel(text, parent);
    label->setObjectName(objectName);
    label->setWordWrap(true);
    label->setTextInteractionFlags(Qt::TextSelectableByMouse);
    return label;
}

QWidget* sectionHeader(const QString& glyph, const QString& title, QWidget* parent) {
    auto* widget = new QWidget(parent);
    auto* layout = new QHBoxLayout(widget);
    layout->setContentsMargins(0, 0, 0, 0);
    layout->setSpacing(6);
    auto* icon = new QLabel(glyph, widget);
    icon->setObjectName(QStringLiteral("analysisSectionIcon"));
    layout->addWidget(icon);
    auto* label = new QLabel(title, widget);
    label->setObjectName(QStringLiteral("analysisSectionTitle"));
    layout->addWidget(label);
    layout->addStretch();
    return widget;
}

QFrame* divider(QWidget* parent) {
    auto* line = new QFrame(parent);
    line->setObjectName(QStringLiteral("analysisDivider"));
    line->setFixedHeight(1);
    return line;
}

QWidget* roleRow(const QString& role, const QString& content, QWidget* parent) {
    const QString normalized = role.trimmed().toLower();
    QString label = role;
    QString color = QStringLiteral("#CBD5E1");
    if (normalized == QStringLiteral("user")) {
        label = QStringLiteral("用户");
        color = QStringLiteral("#22C55E");
    } else if (normalized == QStringLiteral("assistant")) {
        label = QStringLiteral("助手");
        color = QStringLiteral("#818CF8");
    } else if (normalized == QStringLiteral("tool_request")) {
        label = QStringLiteral("请求工具");
        color = QStringLiteral("#EC4899");
    } else if (normalized == QStringLiteral("tool_result") || normalized == QStringLiteral("tool")) {
        label = QStringLiteral("工具结果");
        color = QStringLiteral("#14B8A6");
    }
    auto* row = new QWidget(parent);
    row->setObjectName(QStringLiteral("analysisRoleRow"));
    auto* layout = new QHBoxLayout(row);
    layout->setContentsMargins(0, 2, 0, 2);
    layout->setSpacing(8);
    auto* roleLabel = new QLabel(label, row);
    roleLabel->setObjectName(QStringLiteral("analysisRoleLabel"));
    roleLabel->setFixedWidth(64);
    roleLabel->setStyleSheet(QStringLiteral("color:%1;").arg(color));
    layout->addWidget(roleLabel, 0, Qt::AlignTop);
    layout->addWidget(textLabel(clipped(content, normalized == QStringLiteral("assistant") ? 520 : 360),
                                QStringLiteral("analysisRoleContent"), row), 1);
    return row;
}

QWidget* metaChip(const QString& glyph, const QString& title, const QString& value, const QColor& color, QWidget* parent) {
    auto* chip = new QFrame(parent);
    chip->setObjectName(QStringLiteral("analysisMetaChip"));
    chip->setStyleSheet(QStringLiteral(
        "QFrame#analysisMetaChip{background:rgba(%1,%2,%3,15);border:1px solid rgba(%1,%2,%3,42);border-radius:6px;}"
        "QLabel{background:transparent;border:0;}")
                            .arg(color.red()).arg(color.green()).arg(color.blue()));
    auto* layout = new QVBoxLayout(chip);
    layout->setContentsMargins(8, 6, 8, 6);
    layout->setSpacing(2);
    auto* header = new QLabel(QStringLiteral("%1  %2").arg(glyph, title), chip);
    header->setObjectName(QStringLiteral("analysisMetaTitle"));
    header->setStyleSheet(QStringLiteral("color:%1;").arg(color.name()));
    layout->addWidget(header);
    layout->addWidget(textLabel(clipped(value, 300), QStringLiteral("analysisMetaValue"), chip));
    return chip;
}
}

RawLogView::RawLogView(QWidget* parent) : QPlainTextEdit(parent) {
    setObjectName(QStringLiteral("monitorLogView"));
    setReadOnly(true);
    setPlaceholderText(QStringLiteral("暂无原始日志"));
    setMaximumBlockCount(1000);
}

void RawLogView::appendLogLines(const QStringList& lines) {
    if (lines.isEmpty()) return;
    QTextCursor cursor(document());
    cursor.movePosition(QTextCursor::End);
    for (const QString& line : lines) {
        if (!document()->isEmpty()) cursor.insertBlock();
        QTextCharFormat format;
        format.setForeground(rawLogColor(line));
        format.setFontFamilies({QStringLiteral("Menlo")});
        format.setFontPointSize(11);
        cursor.insertText(QStringLiteral("[%1] %2").arg(QDateTime::currentDateTime().toString(QStringLiteral("HH:mm:ss")), line), format);
    }
    setTextCursor(cursor);
    ensureCursorVisible();
}

AnalysisLogView::AnalysisLogView(QWidget* parent) : QScrollArea(parent) {
    setObjectName(QStringLiteral("analysisLogView"));
    setWidgetResizable(true);
    setFrameShape(QFrame::NoFrame);
    content_ = new QWidget(this);
    content_->setObjectName(QStringLiteral("analysisLogContent"));
    layout_ = new QVBoxLayout(content_);
    layout_->setContentsMargins(12, 12, 12, 12);
    layout_->setSpacing(12);
    layout_->setSizeConstraint(QLayout::SetMinAndMaxSize);
    setWidget(content_);
    rebuild();
}

void AnalysisLogView::applySnapshots(const QJsonArray& snapshots) {
    bool changed = false;
    for (const QJsonValue& value : snapshots) {
        const QJsonObject record = value.toObject();
        const QString requestId = record.value(QStringLiteral("request_id")).toString().trimmed();
        if (requestId.isEmpty()) continue;
        if (!records_.contains(requestId)) order_.append(requestId);
        records_.insert(requestId, record);
        changed = true;
    }
    if (!changed) return;
    std::sort(order_.begin(), order_.end(), [this](const QString& left, const QString& right) {
        const QString leftTime = records_.value(left).value(QStringLiteral("started_at")).toString();
        const QString rightTime = records_.value(right).value(QStringLiteral("started_at")).toString();
        if (leftTime == rightTime) return left < right;
        return leftTime < rightTime;
    });
    while (order_.size() > kMaxRenderedRecords) records_.remove(order_.takeFirst());
    rebuild();
}

void AnalysisLogView::clearRecords() {
    records_.clear();
    order_.clear();
    rebuild();
}

int AnalysisLogView::recordCount() const { return records_.size(); }

void AnalysisLogView::rebuild() {
    while (QLayoutItem* item = layout_->takeAt(0)) {
        delete item->widget();
        delete item;
    }
    if (order_.isEmpty()) {
        layout_->addStretch();
        auto* emptyIcon = new QLabel(QStringLiteral("▤"), content_);
        emptyIcon->setObjectName(QStringLiteral("analysisEmptyIcon"));
        emptyIcon->setAlignment(Qt::AlignCenter);
        layout_->addWidget(emptyIcon);
        auto* emptyText = new QLabel(QStringLiteral("等待代理请求…"), content_);
        emptyText->setObjectName(QStringLiteral("analysisEmptyText"));
        emptyText->setAlignment(Qt::AlignCenter);
        layout_->addWidget(emptyText);
        layout_->addStretch();
        return;
    }
    for (const QString& requestId : std::as_const(order_)) layout_->addWidget(createRecordCard(records_.value(requestId)));
    layout_->addStretch();
}

QWidget* AnalysisLogView::createRecordCard(const QJsonObject& record) {
    const QJsonObject decision = record.value(QStringLiteral("decision")).toObject();
    const QString action = decision.value(QStringLiteral("action")).toString().trimmed().toUpper();
    const bool blocked = action == QStringLiteral("BLOCK") || action == QStringLiteral("HARD_BLOCK");
    const QColor border = blocked ? QColor(QStringLiteral("#EF4444"))
                                  : (!action.isEmpty() && !isNormalDecision(action) ? QColor(QStringLiteral("#F59E0B"))
                                                                                    : QColor(255, 255, 255));
    const int borderAlpha = blocked ? 102 : (!action.isEmpty() && !isNormalDecision(action) ? 77 : 20);
    auto* card = new QFrame(content_);
    card->setObjectName(QStringLiteral("analysisRequestCard"));
    card->setStyleSheet(QStringLiteral(
        "QFrame#analysisRequestCard{background:rgba(0,0,0,76);border:1px solid rgba(%1,%2,%3,%4);border-radius:8px;}"
        "QLabel{background:transparent;border:0;}")
                            .arg(border.red()).arg(border.green()).arg(border.blue()).arg(borderAlpha));
    auto* cardLayout = new QVBoxLayout(card);
    cardLayout->setContentsMargins(12, 12, 12, 12);
    cardLayout->setSpacing(8);

    auto* header = new QWidget(card);
    auto* headerLayout = new QHBoxLayout(header);
    headerLayout->setContentsMargins(0, 0, 0, 0);
    headerLayout->setSpacing(8);
    auto* workflow = new QLabel(QStringLiteral("⌘"), header);
    workflow->setObjectName(QStringLiteral("analysisWorkflowIcon"));
    headerLayout->addWidget(workflow, 0, Qt::AlignTop);
    auto* identity = new QVBoxLayout;
    identity->setSpacing(2);
    const QString asset = record.value(QStringLiteral("asset_name")).toString(
        record.value(QStringLiteral("asset_id")).toString(QStringLiteral("Proxy")));
    const QString model = record.value(QStringLiteral("model")).toString(QStringLiteral("-"));
    identity->addWidget(textLabel(QStringLiteral("%1 / %2").arg(asset, model), QStringLiteral("analysisIdentity"), header));
    const Presentation presentation = statusPresentation(record);
    auto* timeRow = new QWidget(header);
    auto* timeLayout = new QHBoxLayout(timeRow);
    timeLayout->setContentsMargins(0, 0, 0, 0);
    timeLayout->setSpacing(6);
    auto* time = new QLabel(QStringLiteral("◷  %1").arg(localTime(record.value(QStringLiteral("started_at")).toString())), timeRow);
    time->setObjectName(QStringLiteral("analysisTime"));
    timeLayout->addWidget(time);
    auto* status = new QLabel(presentation.text, timeRow);
    status->setObjectName(QStringLiteral("analysisStatusChip"));
    status->setStyleSheet(QStringLiteral("color:%1;background:rgba(%2,%3,%4,31);border:1px solid rgba(%2,%3,%4,64);border-radius:4px;padding:2px 6px;")
                              .arg(presentation.color.name()).arg(presentation.color.red()).arg(presentation.color.green()).arg(presentation.color.blue()));
    timeLayout->addWidget(status);
    timeLayout->addStretch();
    identity->addWidget(timeRow);
    headerLayout->addLayout(identity, 1);
    cardLayout->addWidget(header);

    QList<QPair<QString, QString>> summaries;
    const QJsonArray messages = record.value(QStringLiteral("messages")).toArray();
    int lastUser = -1;
    for (qsizetype index = 0; index < messages.size(); ++index) {
        if (messages.at(index).toObject().value(QStringLiteral("role")).toString().toLower() == QStringLiteral("user")) lastUser = index;
    }
    const qsizetype first = qMax<qsizetype>(lastUser >= 0 ? lastUser : 0, messages.size() - 7);
    for (qsizetype index = first; index < messages.size(); ++index) {
        const QJsonObject message = messages.at(index).toObject();
        const QString role = message.value(QStringLiteral("role")).toString().toLower();
        if (role == QStringLiteral("system") || role == QStringLiteral("tool_request") || role == QStringLiteral("tool_result")) continue;
        const QString content = sanitize(message.value(QStringLiteral("content")).toString());
        if (content.isEmpty() && (role == QStringLiteral("assistant") || role == QStringLiteral("tool"))) continue;
        summaries.append({role, content});
    }
    const QString primaryType = record.value(QStringLiteral("primary_content_type")).toString();
    const QString primary = sanitize(record.value(QStringLiteral("primary_content")).toString());
    const bool hasAssistant = std::any_of(summaries.cbegin(), summaries.cend(), [](const auto& summary) {
        return summary.first == QStringLiteral("assistant");
    });
    if (!primary.isEmpty() && !hasAssistant &&
        (primaryType == QStringLiteral("assistant_response") || primaryType == QStringLiteral("security_warning"))) {
        summaries.append({QStringLiteral("assistant"), primary});
    }

    QStringList toolNames;
    QStringList toolArgs;
    QStringList toolResults;
    for (const QJsonValue& value : record.value(QStringLiteral("tool_calls")).toArray()) {
        const QJsonObject tool = value.toObject();
        const QString name = tool.value(QStringLiteral("name")).toString(QStringLiteral("tool"));
        toolNames.append(name);
        const QString source = tool.value(QStringLiteral("source")).toString();
        const QString arguments = sanitize(tool.value(QStringLiteral("arguments")).toString());
        const QString result = sanitize(tool.value(QStringLiteral("result")).toString());
        if (source == QStringLiteral("response")) {
            summaries.append({QStringLiteral("tool_request"), arguments.isEmpty() ? name : QStringLiteral("%1: %2").arg(name, arguments)});
            if (!arguments.isEmpty()) toolArgs.append(QStringLiteral("%1: %2").arg(name, arguments));
        }
        if (source == QStringLiteral("history") && tool.value(QStringLiteral("latest_round")).toBool() && !result.isEmpty()) {
            toolResults.append(QStringLiteral("%1: %2").arg(name, result));
        }
    }

    if (!summaries.isEmpty()) {
        cardLayout->addWidget(sectionHeader(QStringLiteral("□"), QStringLiteral("对话摘要"), card));
        auto* summaryCard = new QFrame(card);
        summaryCard->setObjectName(QStringLiteral("analysisSummaryCard"));
        auto* summaryLayout = new QVBoxLayout(summaryCard);
        summaryLayout->setContentsMargins(10, 8, 10, 8);
        summaryLayout->setSpacing(2);
        for (const auto& summary : std::as_const(summaries)) summaryLayout->addWidget(roleRow(summary.first, summary.second, summaryCard));
        cardLayout->addWidget(summaryCard);
    }
    if (!toolArgs.isEmpty()) cardLayout->addWidget(metaChip(QStringLiteral("{}"), QStringLiteral("工具参数"), toolArgs.mid(0, 5).join(QLatin1Char('\n')), QColor(QStringLiteral("#EC4899")), card));
    if (!toolResults.isEmpty()) cardLayout->addWidget(metaChip(QStringLiteral("⇥"), QStringLiteral("工具结果"), toolResults.mid(0, 5).join(QLatin1Char('\n')), QColor(QStringLiteral("#14B8A6")), card));

    const int promptTokens = record.value(QStringLiteral("prompt_tokens")).toInt();
    const int completionTokens = record.value(QStringLiteral("completion_tokens")).toInt();
    const bool hasDecision = !action.isEmpty() && !isNormalDecision(action);
    if (hasDecision || !toolNames.isEmpty() || promptTokens + completionTokens > 0) {
        cardLayout->addWidget(divider(card));
        if (hasDecision) {
            const QString reason = decision.value(QStringLiteral("reason")).toString();
            const QString risk = decision.value(QStringLiteral("risk_level")).toString();
            cardLayout->addWidget(metaChip(blocked ? QStringLiteral("◇") : QStringLiteral("△"), QStringLiteral("安全决策"),
                                               QStringLiteral("%1 · %2%3").arg(action, risk, reason.isEmpty() ? QString() : QStringLiteral(" · %1").arg(reason)),
                                               blocked ? QColor(QStringLiteral("#EF4444")) : QColor(QStringLiteral("#F59E0B")), card));
        }
        if (!toolNames.isEmpty()) {
            toolNames.removeDuplicates();
            cardLayout->addWidget(metaChip(QStringLiteral("⌘"), QStringLiteral("工具调用 %1").arg(record.value(QStringLiteral("tool_calls")).toArray().size()),
                                               toolNames.join(QStringLiteral(", ")), QColor(QStringLiteral("#EC4899")), card));
        }
        if (promptTokens + completionTokens > 0) {
            cardLayout->addWidget(metaChip(QStringLiteral("◎"), QStringLiteral("Token"),
                                               QStringLiteral("%1 / %2 / %3").arg(promptTokens).arg(completionTokens).arg(promptTokens + completionTokens),
                                               QColor(QStringLiteral("#10B981")), card));
        }
    }
    return card;
}
