#include "ui/ProtectionMonitorWindow.h"

#include "bridge/GoBridge.h"
#include "ui/UiDialogs.h"
#include "ui/widgets/AnalysisLogView.h"
#include "ui/widgets/TrendChartWidget.h"
#include "ui/widgets/GradientWidget.h"
#include "ui/widgets/SecurityEventListWidget.h"

#include <QCheckBox>
#include <QColor>
#include <QFutureWatcher>
#include <QFrame>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QPushButton>
#include <QSplitter>
#include <QStackedWidget>
#include <QTimer>
#include <QVBoxLayout>
#include <QtConcurrent>

namespace {
QFrame* metricCard(const QString& title, const QString& glyph, QLabel** value, const QColor& color, QWidget* parent) {
    auto* card = new QFrame(parent);
    card->setObjectName(QStringLiteral("monitorMetricCard"));
    card->setMinimumHeight(58);
    card->setStyleSheet(QStringLiteral(
        "QFrame#monitorMetricCard { background:rgba(%1,%2,%3,26); border:1px solid rgba(%1,%2,%3,51); border-radius:8px; }"
        "QLabel { background:transparent; border:0; }")
                            .arg(color.red()).arg(color.green()).arg(color.blue()));
    auto* layout = new QHBoxLayout(card);
    layout->setContentsMargins(12, 8, 12, 8);
    layout->setSpacing(8);
    auto* icon = new QLabel(glyph, card);
    icon->setObjectName(QStringLiteral("monitorMetricIcon"));
    icon->setAlignment(Qt::AlignCenter);
    icon->setFixedSize(24, 24);
    icon->setStyleSheet(QStringLiteral("color:%1;font-size:16px;font-weight:600;").arg(color.name()));
    layout->addWidget(icon);
    auto* textLayout = new QVBoxLayout;
    textLayout->setSpacing(0);
    auto* label = new QLabel(title, card);
    label->setObjectName(QStringLiteral("monitorMetricLabel"));
    *value = new QLabel(QStringLiteral("0"), card);
    (*value)->setObjectName(QStringLiteral("monitorMetricValue"));
    textLayout->addWidget(*value);
    textLayout->addWidget(label);
    layout->addLayout(textLayout, 1);
    return card;
}

QFrame* divider(QWidget* parent) {
    auto* line = new QFrame(parent);
    line->setObjectName(QStringLiteral("monitorDivider"));
    line->setFrameShape(QFrame::HLine);
    line->setFixedHeight(1);
    return line;
}

QPushButton* iconButton(const QString& glyph, const QString& tooltip, QWidget* parent) {
    auto* button = new QPushButton(glyph, parent);
    button->setObjectName(QStringLiteral("monitorIconButton"));
    button->setToolTip(tooltip);
    button->setFixedSize(28, 28);
    return button;
}
}

ProtectionMonitorWindow::ProtectionMonitorWindow(const AssetModel& asset, GoBridge* bridge, const QString& sessionId, QWidget* parent)
    : QMainWindow(parent), asset_(asset), bridge_(bridge), sessionId_(sessionId) {
    setAttribute(Qt::WA_DeleteOnClose);
    setWindowTitle(QStringLiteral("防护监控中心"));
    setMinimumSize(1200, 700);
    resize(1440, 900);
    buildUi();
    refreshTimer_ = new QTimer(this);
    refreshTimer_->setInterval(1800);
    connect(refreshTimer_, &QTimer::timeout, this, &ProtectionMonitorWindow::refreshStatus);
    connect(refreshTimer_, &QTimer::timeout, this, &ProtectionMonitorWindow::refreshLogs);
    connect(refreshTimer_, &QTimer::timeout, this, &ProtectionMonitorWindow::refreshSecurityEvents);
    refreshTimer_->start();
    refreshStatus();
    refreshLogs();
    refreshSecurityEvents();
}

void ProtectionMonitorWindow::buildUi() {
    auto* shell = new GradientWidget(this);
    auto* root = new QVBoxLayout(shell);
    root->setContentsMargins(0, 0, 0, 0);
    root->setSpacing(0);
    auto* titleBar = new QWidget(shell);
    titleBar->setObjectName(QStringLiteral("titleBar"));
    titleBar->setFixedHeight(48);
    auto* titleLayout = new QHBoxLayout(titleBar);
    titleLayout->setContentsMargins(16, 0, 16, 0);
    titleLayout->setSpacing(10);
    auto* icon = new QLabel(QStringLiteral("◇"), titleBar);
    icon->setAlignment(Qt::AlignCenter);
    icon->setFixedSize(28, 28);
    icon->setStyleSheet(QStringLiteral("color:#818CF8;background:rgba(99,102,241,51);border-radius:8px;font-size:17px;font-weight:700;"));
    titleLayout->addWidget(icon);
    auto* titleBlock = new QVBoxLayout;
    titleBlock->setSpacing(0);
    auto* title = new QLabel(QStringLiteral("防护监控中心"), titleBar);
    title->setObjectName(QStringLiteral("appTitle"));
    titleBlock->addWidget(title);
    auto* asset = new QLabel(asset_.name, titleBar);
    asset->setObjectName(QStringLiteral("monitorAssetName"));
    titleBlock->addWidget(asset);
    titleLayout->addLayout(titleBlock);
    titleLayout->addStretch();

    root->addWidget(titleBar);

    auto* content = new QWidget(shell);
    content->setObjectName(QStringLiteral("monitorContent"));
    auto* contentLayout = new QVBoxLayout(content);
    contentLayout->setContentsMargins(16, 16, 16, 16);
    contentLayout->setSpacing(12);

    statusCard_ = new QFrame(content);
    statusCard_->setObjectName(QStringLiteral("monitorStatusCard"));
    statusCard_->setMinimumHeight(62);
    auto* statusLayout = new QHBoxLayout(statusCard_);
    statusLayout->setContentsMargins(16, 10, 16, 10);
    statusLayout->setSpacing(12);
    statusDot_ = new QLabel(statusCard_);
    statusDot_->setObjectName(QStringLiteral("monitorStatusDot"));
    statusDot_->setFixedSize(12, 12);
    statusLayout->addWidget(statusDot_);
    auto* statusText = new QVBoxLayout;
    statusText->setSpacing(0);
    auto* statusCaption = new QLabel(QStringLiteral("防护状态"), statusCard_);
    statusCaption->setObjectName(QStringLiteral("monitorStatusCaption"));
    statusText->addWidget(statusCaption);
    stateLabel_ = new QLabel(QStringLiteral("正在连接"), statusCard_);
    stateLabel_->setObjectName(QStringLiteral("monitorStatusValue"));
    statusText->addWidget(stateLabel_);
    statusLayout->addLayout(statusText, 1);
    auto* realtime = new QLabel(QStringLiteral("⌁  实时监控"), statusCard_);
    realtime->setObjectName(QStringLiteral("monitorRealtimePill"));
    statusLayout->addWidget(realtime);
    auditOnly_ = new QCheckBox(QStringLiteral("仅审计"), statusCard_);
    auditOnly_->setObjectName(QStringLiteral("monitorAuditSwitch"));
    connect(auditOnly_, &QCheckBox::toggled, this, [this](bool checked) {
        if (bridge_ == nullptr) return;
        auditOnly_->setEnabled(false);
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, checked]() {
            const QJsonObject result = watcher->result();
            auditOnly_->setEnabled(true);
            if (!result.value(QStringLiteral("success")).toBool()) {
                auditOnly_->blockSignals(true);
                auditOnly_->setChecked(!checked);
                auditOnly_->blockSignals(false);
            }
            watcher->deleteLater();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, id = asset_.id, checked]() {
            return bridge->call("SetProtectionProxyAuditOnlyByAsset", id, checked ? 1 : 0);
        }));
    });
    statusLayout->addWidget(auditOnly_);
    contentLayout->addWidget(statusCard_);

    auto* metrics = new QGridLayout;
    metrics->setSpacing(12);
    metrics->addWidget(metricCard(QStringLiteral("分析次数"), QStringLiteral("⌁"), &requestCount_, QColor(QStringLiteral("#6366F1")), content), 0, 0);
    metrics->addWidget(metricCard(QStringLiteral("消息数量"), QStringLiteral("□"), &messageCount_, QColor(QStringLiteral("#8B5CF6")), content), 0, 1);
    metrics->addWidget(metricCard(QStringLiteral("风险次数"), QStringLiteral("△"), &riskCount_, QColor(QStringLiteral("#F59E0B")), content), 0, 2);
    metrics->addWidget(metricCard(QStringLiteral("拦截次数"), QStringLiteral("◇"), &blockedCount_, QColor(QStringLiteral("#22C55E")), content), 0, 3);
    metrics->addWidget(metricCard(QStringLiteral("Token 总量"), QStringLiteral("◎"), &tokenCount_, QColor(QStringLiteral("#10B981")), content), 1, 0);
    metrics->addWidget(metricCard(QStringLiteral("输入 Token"), QStringLiteral("↑"), &promptTokenCount_, QColor(QStringLiteral("#3B82F6")), content), 1, 1);
    metrics->addWidget(metricCard(QStringLiteral("输出 Token"), QStringLiteral("↓"), &completionTokenCount_, QColor(QStringLiteral("#8B5CF6")), content), 1, 2);
    metrics->addWidget(metricCard(QStringLiteral("工具调用"), QStringLiteral("⌘"), &toolCallCount_, QColor(QStringLiteral("#EC4899")), content), 1, 3);
    metrics->addWidget(metricCard(QStringLiteral("安全分析 Token"), QStringLiteral("◇"), &auditTokenCount_, QColor(QStringLiteral("#6366F1")), content), 2, 0);
    metrics->addWidget(metricCard(QStringLiteral("安全输入 Token"), QStringLiteral("↑"), &auditPromptTokenCount_, QColor(QStringLiteral("#8B5CF6")), content), 2, 1);
    metrics->addWidget(metricCard(QStringLiteral("安全输出 Token"), QStringLiteral("↓"), &auditCompletionTokenCount_, QColor(QStringLiteral("#EC4899")), content), 2, 2);
    auto* metricSpacer = new QWidget(content);
    metricSpacer->setObjectName(QStringLiteral("monitorMetricSpacer"));
    metrics->addWidget(metricSpacer, 2, 3);
    for (int column = 0; column < 4; ++column) metrics->setColumnStretch(column, 1);
    contentLayout->addLayout(metrics);

    auto* splitter = new QSplitter(Qt::Horizontal, content);
    splitter->setObjectName(QStringLiteral("monitorMainSplitter"));
    splitter->setHandleWidth(12);
    auto* trends = new QWidget(splitter);
    trends->setObjectName(QStringLiteral("monitorTrendColumn"));
    auto* trendsLayout = new QVBoxLayout(trends);
    trendsLayout->setContentsMargins(0, 0, 0, 0);
    trendsLayout->setSpacing(12);
    tokenTrend_ = new TrendChartWidget(QStringLiteral("Token 使用趋势"), QColor(QStringLiteral("#10B981")), TrendChartWidget::Mode::Line, trends);
    toolTrend_ = new TrendChartWidget(QStringLiteral("工具调用趋势"), QColor(QStringLiteral("#EC4899")), TrendChartWidget::Mode::Bars, trends);
    trendsLayout->addWidget(tokenTrend_, 1);
    trendsLayout->addWidget(toolTrend_, 1);
    splitter->addWidget(trends);

    auto* activity = new QFrame(splitter);
    activity->setObjectName(QStringLiteral("monitorPanel"));
    activity->setMinimumWidth(420);
    auto* activityLayout = new QVBoxLayout(activity);
    activityLayout->setContentsMargins(0, 0, 0, 0);
    activityLayout->setSpacing(0);
    auto* activityHeader = new QWidget(activity);
    activityHeader->setObjectName(QStringLiteral("monitorPanelHeader"));
    auto* activityHeaderLayout = new QHBoxLayout(activityHeader);
    activityHeaderLayout->setContentsMargins(12, 8, 12, 8);
    activityHeaderLayout->setSpacing(8);
    auto* terminalIcon = new QLabel(QStringLiteral(">_"), activityHeader);
    terminalIcon->setObjectName(QStringLiteral("monitorPanelIcon"));
    activityHeaderLayout->addWidget(terminalIcon);
    auto* activityTitle = new QLabel(QStringLiteral("分析日志"), activityHeader);
    activityTitle->setObjectName(QStringLiteral("monitorPanelTitle"));
    activityHeaderLayout->addWidget(activityTitle);
    auto* logStack = new QStackedWidget(activity);
    auto* groupedButton = new QPushButton(QStringLiteral("分组"), activityHeader);
    auto* rawButton = new QPushButton(QStringLiteral("原始"), activityHeader);
    for (QPushButton* button : {groupedButton, rawButton}) {
        button->setObjectName(QStringLiteral("monitorSegmentButton"));
        button->setCheckable(true);
    }
    groupedButton->setChecked(true);
    activityHeaderLayout->addWidget(groupedButton);
    activityHeaderLayout->addWidget(rawButton);
    activityHeaderLayout->addStretch();
    auto* clearLogs = new QPushButton(QStringLiteral("清空"), activityHeader);
    clearLogs->setObjectName(QStringLiteral("monitorTextButton"));
    activityHeaderLayout->addWidget(clearLogs);
    activityLayout->addWidget(activityHeader);
    activityLayout->addWidget(divider(activity));
    groupedLog_ = new AnalysisLogView(logStack);
    rawLog_ = new RawLogView(logStack);
    logStack->addWidget(groupedLog_);
    logStack->addWidget(rawLog_);
    activityLayout->addWidget(logStack, 1);
    connect(groupedButton, &QPushButton::clicked, this, [logStack, groupedButton, rawButton]() {
        logStack->setCurrentIndex(0);
        groupedButton->setChecked(true);
        rawButton->setChecked(false);
    });
    connect(rawButton, &QPushButton::clicked, this, [logStack, groupedButton, rawButton]() {
        logStack->setCurrentIndex(1);
        groupedButton->setChecked(false);
        rawButton->setChecked(true);
    });
    connect(clearLogs, &QPushButton::clicked, this, [this]() {
        groupedLog_->clearRecords();
        rawLog_->clear();
    });
    splitter->addWidget(activity);

    auto* events = new QFrame(splitter);
    events->setObjectName(QStringLiteral("monitorPanel"));
    events->setMinimumWidth(260);
    events->setMaximumWidth(420);
    auto* eventsLayout = new QVBoxLayout(events);
    eventsLayout->setContentsMargins(0, 0, 0, 0);
    eventsLayout->setSpacing(0);
    auto* eventsHeader = new QWidget(events);
    eventsHeader->setObjectName(QStringLiteral("monitorPanelHeader"));
    auto* eventsHeaderLayout = new QHBoxLayout(eventsHeader);
    eventsHeaderLayout->setContentsMargins(12, 8, 12, 8);
    eventsHeaderLayout->setSpacing(8);
    auto* eventIcon = new QLabel(QStringLiteral("◇"), eventsHeader);
    eventIcon->setObjectName(QStringLiteral("monitorPanelIcon"));
    eventsHeaderLayout->addWidget(eventIcon);
    auto* eventTitle = new QLabel(QStringLiteral("安全事件"), eventsHeader);
    eventTitle->setObjectName(QStringLiteral("monitorPanelTitle"));
    eventsHeaderLayout->addWidget(eventTitle);
    eventCount_ = new QLabel(QStringLiteral("0"), eventsHeader);
    eventCount_->setObjectName(QStringLiteral("monitorEventCount"));
    eventsHeaderLayout->addWidget(eventCount_);
    eventsHeaderLayout->addStretch();
    auto* refreshEvents = iconButton(QStringLiteral("↻"), QStringLiteral("刷新"), eventsHeader);
    clearEventsButton_ = iconButton(QStringLiteral("⌫"), QStringLiteral("清空全部"), eventsHeader);
    clearEventsButton_->setEnabled(false);
    eventsHeaderLayout->addWidget(refreshEvents);
    eventsHeaderLayout->addWidget(clearEventsButton_);
    eventsLayout->addWidget(eventsHeader);
    eventsLayout->addWidget(divider(events));
    eventStack_ = new QStackedWidget(events);
    eventStack_->setObjectName(QStringLiteral("monitorEventStack"));
    auto* emptyEvents = new QWidget(eventStack_);
    auto* emptyEventsLayout = new QVBoxLayout(emptyEvents);
    emptyEventsLayout->setContentsMargins(12, 12, 12, 12);
    emptyEventsLayout->addStretch();
    auto* emptyEventsIcon = new QLabel(QStringLiteral("◇"), emptyEvents);
    emptyEventsIcon->setObjectName(QStringLiteral("monitorEmptyIcon"));
    emptyEventsIcon->setAlignment(Qt::AlignCenter);
    emptyEventsLayout->addWidget(emptyEventsIcon);
    auto* emptyEventsText = new QLabel(QStringLiteral("暂无安全事件"), emptyEvents);
    emptyEventsText->setObjectName(QStringLiteral("monitorEmptyText"));
    emptyEventsText->setAlignment(Qt::AlignCenter);
    emptyEventsLayout->addWidget(emptyEventsText);
    emptyEventsLayout->addStretch();
    eventsList_ = new SecurityEventListWidget(eventStack_);
    eventStack_->addWidget(emptyEvents);
    eventStack_->addWidget(eventsList_);
    eventsLayout->addWidget(eventStack_, 1);
    connect(refreshEvents, &QPushButton::clicked, this, &ProtectionMonitorWindow::refreshSecurityEvents);
    connect(clearEventsButton_, &QPushButton::clicked, this, &ProtectionMonitorWindow::clearSecurityEvents);
    connect(eventsList_, &SecurityEventListWidget::eventActivated, this, [this](const QJsonObject& event) {
        showSecurityEventDetail(event, this);
    });
    connect(eventsList_, &SecurityEventListWidget::eventCountChanged, this, [this](int) { updateEventCount(); });
    splitter->addWidget(events);
    splitter->setStretchFactor(0, 2);
    splitter->setStretchFactor(1, 3);
    splitter->setStretchFactor(2, 2);
    splitter->setSizes({320, 480, 320});
    contentLayout->addWidget(splitter, 1);
    root->addWidget(content, 1);
    setCentralWidget(shell);
}

void ProtectionMonitorWindow::refreshStatus() {
    if (bridge_ == nullptr || refreshInFlight_) return;
    refreshInFlight_ = true;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonObject response = watcher->result();
        const QJsonObject status = response.value(QStringLiteral("status")).toObject();
        const QJsonObject metrics = response.value(QStringLiteral("metrics")).toObject().value(QStringLiteral("data")).toObject();
        const QString previousSessionId = sessionId_;
        sessionId_ = status.value(QStringLiteral("session_id")).toString(status.value(QStringLiteral("proxy_session_id")).toString(sessionId_));
        const bool running = status.value(QStringLiteral("running")).toBool(status.value(QStringLiteral("is_running")).toBool());
        auditOnly_->blockSignals(true);
        auditOnly_->setChecked(status.value(QStringLiteral("audit_only")).toBool());
        auditOnly_->blockSignals(false);
        const QString statusColor = running ? QStringLiteral("#22C55E") : QStringLiteral("#F59E0B");
        stateLabel_->setText(running ? QStringLiteral("防护已启用") : QStringLiteral("防护未启用"));
        stateLabel_->setStyleSheet(QStringLiteral("color:%1;").arg(statusColor));
        statusDot_->setStyleSheet(QStringLiteral("background:%1;border-radius:6px;").arg(statusColor));
        statusCard_->setStyleSheet(running
            ? QStringLiteral("QFrame#monitorStatusCard{background:rgba(34,197,94,26);border:1px solid rgba(34,197,94,77);border-radius:12px;}")
            : QStringLiteral("QFrame#monitorStatusCard{background:rgba(245,158,11,26);border:1px solid rgba(245,158,11,77);border-radius:12px;}"));
        QList<int> tokenValues;
        for (const QJsonValue& value : metrics.value(QStringLiteral("token_trend")).toArray()) tokenValues.append(value.toObject().value(QStringLiteral("tokens")).toInt());
        QList<int> toolValues;
        for (const QJsonValue& value : metrics.value(QStringLiteral("tool_call_trend")).toArray()) toolValues.append(value.toObject().value(QStringLiteral("count")).toInt());
        tokenTrend_->setValues(tokenValues);
        toolTrend_->setValues(toolValues);
        refreshInFlight_ = false;
        watcher->deleteLater();
        if (previousSessionId.isEmpty() && !sessionId_.isEmpty()) QTimer::singleShot(0, this, &ProtectionMonitorWindow::refreshLogs);
    });
    watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, id = asset_.id]() {
        const QJsonObject query{{QStringLiteral("duration_seconds"), 86400}, {QStringLiteral("asset_id"), id}};
        return QJsonObject{{QStringLiteral("status"), bridge->call("GetProtectionProxyStatusByAsset", id)},
                           {QStringLiteral("metrics"), bridge->call("GetApiStatisticsFFI", QString::fromUtf8(QJsonDocument(query).toJson(QJsonDocument::Compact)))}};
    }));
}

void ProtectionMonitorWindow::refreshLogs() {
    if (bridge_ == nullptr || sessionId_.isEmpty() || logsInFlight_) return;
    logsInFlight_ = true;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonObject response = watcher->result();
        const QJsonObject data = response.value(QStringLiteral("data")).isObject() ? response.value(QStringLiteral("data")).toObject() : response;
        const QJsonArray logs = data.value(QStringLiteral("logs")).toArray();
        QStringList logLines;
        for (const QJsonValue& value : logs) logLines.append(value.toString());
        if (!logLines.isEmpty()) {
            rawLog_->appendLogLines(logLines);
        }
        requestCount_->setText(QString::number(data.value(QStringLiteral("analysis_count")).toInteger()));
        messageCount_->setText(QString::number(data.value(QStringLiteral("request_count")).toInteger()));
        riskCount_->setText(QString::number(data.value(QStringLiteral("warning_count")).toInteger()));
        blockedCount_->setText(QString::number(data.value(QStringLiteral("blocked_count")).toInteger()));
        tokenCount_->setText(QString::number(data.value(QStringLiteral("total_tokens")).toInteger()));
        promptTokenCount_->setText(QString::number(data.value(QStringLiteral("total_prompt_tokens")).toInteger()));
        completionTokenCount_->setText(QString::number(data.value(QStringLiteral("total_completion_tokens")).toInteger()));
        toolCallCount_->setText(QString::number(data.value(QStringLiteral("total_tool_calls")).toInteger()));
        auditPromptTokenCount_->setText(QString::number(data.value(QStringLiteral("audit_prompt_tokens")).toInteger()));
        auditCompletionTokenCount_->setText(QString::number(data.value(QStringLiteral("audit_completion_tokens")).toInteger()));
        auditTokenCount_->setText(QString::number(data.value(QStringLiteral("audit_prompt_tokens")).toInteger() + data.value(QStringLiteral("audit_completion_tokens")).toInteger()));
        const QJsonArray requestViews = data.value(QStringLiteral("request_views")).toArray();
        groupedLog_->applySnapshots(requestViews);
        logsInFlight_ = false;
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, session = sessionId_]() { return bridge->call("GetProtectionProxyLogs", session); }));
}

void ProtectionMonitorWindow::refreshSecurityEvents() {
    if (bridge_ == nullptr) return;
    if (eventsInFlight_ || clearEventsInFlight_) {
        eventsRefreshPending_ = true;
        return;
    }
    eventsInFlight_ = true;
    const quint64 generation = eventsGeneration_;
    const QJsonObject filter{{QStringLiteral("limit"), 50}, {QStringLiteral("offset"), 0}, {QStringLiteral("asset_id"), asset_.id}};
    const QString payload = QString::fromUtf8(QJsonDocument(filter).toJson(QJsonDocument::Compact));
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher, generation]() {
        const QJsonObject response = watcher->result();
        if (generation == eventsGeneration_ && !clearEventsInFlight_) {
            if (response.value(QStringLiteral("success")).toBool()) {
                eventsList_->setEvents(response.value(QStringLiteral("data")).toArray());
            }
        }
        eventsInFlight_ = false;
        watcher->deleteLater();
        if (eventsRefreshPending_ && !clearEventsInFlight_) {
            eventsRefreshPending_ = false;
            QTimer::singleShot(0, this, &ProtectionMonitorWindow::refreshSecurityEvents);
        }
    });
    watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, payload]() { return bridge->call("GetSecurityEventsFFI", payload); }));
}

void ProtectionMonitorWindow::clearSecurityEvents() {
    if (bridge_ == nullptr || clearEventsInFlight_ || eventsList_->eventCount() == 0) return;
    UiDialogs::confirm(this, QStringLiteral("清空安全事件"),
                       QStringLiteral("确定清空当前 Bot 的全部安全事件吗？此操作无法撤销。"), [this]() {
        if (clearEventsInFlight_) return;
        clearEventsInFlight_ = true;
        ++eventsGeneration_;
        clearEventsButton_->setEnabled(false);
        const QJsonObject filter{{QStringLiteral("asset_id"), asset_.id}};
        const QString payload = QString::fromUtf8(QJsonDocument(filter).toJson(QJsonDocument::Compact));
        auto* watcher = new QFutureWatcher<QJsonObject>(this);
        connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
            const QJsonObject response = watcher->result();
            clearEventsInFlight_ = false;
            watcher->deleteLater();
            if (!response.value(QStringLiteral("success")).toBool()) {
                UiDialogs::showWarning(this, QStringLiteral("清空失败"), response.value(QStringLiteral("error")).toString());
            }
            if (eventsInFlight_) eventsRefreshPending_ = true;
            else {
                eventsRefreshPending_ = false;
                refreshSecurityEvents();
            }
            updateEventCount();
        });
        watcher->setFuture(QtConcurrent::run(bridge_->workerPool(), [bridge = bridge_, payload]() { return bridge->call("ClearSecurityEventsFFI", payload); }));
    }, QStringLiteral("清空"));
}

void ProtectionMonitorWindow::updateEventCount() {
    if (eventCount_ == nullptr || eventsList_ == nullptr || eventStack_ == nullptr) return;
    const int count = eventsList_->count();
    eventCount_->setText(QString::number(count));
    eventStack_->setCurrentIndex(count == 0 ? 0 : 1);
    if (clearEventsButton_ != nullptr) clearEventsButton_->setEnabled(count > 0 && !clearEventsInFlight_);
}
