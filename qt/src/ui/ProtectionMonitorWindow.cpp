#include "ui/ProtectionMonitorWindow.h"

#include "bridge/GoBridge.h"
#include "ui/widgets/TrendChartWidget.h"
#include "ui/widgets/GradientWidget.h"

#include <QCheckBox>
#include <QDateTime>
#include <QFutureWatcher>
#include <QFrame>
#include <QGridLayout>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QListWidget>
#include <QMessageBox>
#include <QPlainTextEdit>
#include <QProgressBar>
#include <QPushButton>
#include <QSplitter>
#include <QTabWidget>
#include <QTimer>
#include <QVBoxLayout>
#include <QtConcurrent>

namespace {
QFrame* metricCard(const QString& title, QLabel** value, const QString& color, QWidget* parent) {
    auto* card = new QFrame(parent);
    card->setObjectName(QStringLiteral("card"));
    auto* layout = new QVBoxLayout(card);
    layout->setContentsMargins(14, 12, 14, 12);
    auto* label = new QLabel(title, card);
    label->setObjectName(QStringLiteral("muted"));
    layout->addWidget(label);
    *value = new QLabel(QStringLiteral("0"), card);
    (*value)->setStyleSheet(QStringLiteral("font-size:22px;font-weight:700;color:%1;").arg(color));
    layout->addWidget(*value);
    return card;
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
    titleBar->setFixedHeight(56);
    auto* titleLayout = new QHBoxLayout(titleBar);
    titleLayout->setContentsMargins(18, 0, 18, 0);
    auto* icon = new QLabel(QStringLiteral("♢"), titleBar);
    icon->setStyleSheet(QStringLiteral("background:#6366F1;padding:7px;border-radius:8px;font-size:18px;"));
    titleLayout->addWidget(icon);
    auto* title = new QLabel(QStringLiteral("防护监控中心"), titleBar);
    title->setObjectName(QStringLiteral("appTitle"));
    titleLayout->addWidget(title);
    auto* asset = new QLabel(QStringLiteral("%1 · %2").arg(asset_.name, asset_.version), titleBar);
    asset->setObjectName(QStringLiteral("muted"));
    titleLayout->addWidget(asset);
    titleLayout->addStretch();
    auditOnly_ = new QCheckBox(QStringLiteral("仅审计"), titleBar);
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
        watcher->setFuture(QtConcurrent::run([bridge = bridge_, id = asset_.id, checked]() {
            return bridge->call("SetProtectionProxyAuditOnlyByAsset", id, checked ? 1 : 0);
        }));
    });
    titleLayout->addWidget(auditOnly_);
    stateLabel_ = new QLabel(QStringLiteral("● 正在连接"), titleBar);
    stateLabel_->setStyleSheet(QStringLiteral("color:#F59E0B;padding:5px 10px;border-radius:10px;background:rgba(245,158,11,25);"));
    titleLayout->addWidget(stateLabel_);
    root->addWidget(titleBar);

    auto* content = new QWidget(shell);
    auto* contentLayout = new QVBoxLayout(content);
    contentLayout->setContentsMargins(18, 16, 18, 18);
    contentLayout->setSpacing(14);
    auto* metrics = new QGridLayout;
    metrics->setSpacing(12);
    metrics->addWidget(metricCard(QStringLiteral("分析请求"), &requestCount_, QStringLiteral("#818CF8"), content), 0, 0);
    metrics->addWidget(metricCard(QStringLiteral("风险次数"), &riskCount_, QStringLiteral("#F59E0B"), content), 0, 1);
    metrics->addWidget(metricCard(QStringLiteral("拦截次数"), &blockedCount_, QStringLiteral("#EF4444"), content), 0, 2);
    metrics->addWidget(metricCard(QStringLiteral("Token 用量"), &tokenCount_, QStringLiteral("#22C55E"), content), 0, 3);
    metrics->addWidget(metricCard(QStringLiteral("输入 Token"), &promptTokenCount_, QStringLiteral("#3B82F6"), content), 1, 0);
    metrics->addWidget(metricCard(QStringLiteral("输出 Token"), &completionTokenCount_, QStringLiteral("#8B5CF6"), content), 1, 1);
    metrics->addWidget(metricCard(QStringLiteral("工具调用"), &toolCallCount_, QStringLiteral("#EC4899"), content), 1, 2);
    metrics->addWidget(metricCard(QStringLiteral("安全分析 Token"), &auditTokenCount_, QStringLiteral("#A5B4FC"), content), 1, 3);
    contentLayout->addLayout(metrics);

    auto* trends = new QHBoxLayout;
    trends->setSpacing(12);
    tokenTrend_ = new TrendChartWidget(QStringLiteral("Token 使用趋势"), QColor(QStringLiteral("#10B981")), TrendChartWidget::Mode::Line, content);
    toolTrend_ = new TrendChartWidget(QStringLiteral("工具调用趋势"), QColor(QStringLiteral("#EC4899")), TrendChartWidget::Mode::Bars, content);
    trends->addWidget(tokenTrend_, 1);
    trends->addWidget(toolTrend_, 1);
    contentLayout->addLayout(trends);

    auto* splitter = new QSplitter(Qt::Horizontal, content);
    auto* activity = new QFrame(splitter);
    activity->setObjectName(QStringLiteral("card"));
    auto* activityLayout = new QVBoxLayout(activity);
    auto* activityTitle = new QLabel(QStringLiteral("实时防护日志"), activity);
    activityTitle->setObjectName(QStringLiteral("sectionTitle"));
    activityLayout->addWidget(activityTitle);
    auto* logTabs = new QTabWidget(activity);
    groupedLog_ = new QPlainTextEdit(logTabs);
    groupedLog_->setReadOnly(true);
    groupedLog_->setPlaceholderText(QStringLiteral("等待代理请求..."));
    rawLog_ = new QPlainTextEdit(logTabs);
    rawLog_->setReadOnly(true);
    rawLog_->setPlaceholderText(QStringLiteral("暂无原始日志"));
    logTabs->addTab(groupedLog_, QStringLiteral("分组视图"));
    logTabs->addTab(rawLog_, QStringLiteral("原始日志"));
    activityLayout->addWidget(logTabs, 1);
    splitter->addWidget(activity);

    auto* detail = new QFrame(splitter);
    detail->setObjectName(QStringLiteral("card"));
    detail->setMinimumWidth(380);
    detail->setMaximumWidth(520);
    auto* detailLayout = new QVBoxLayout(detail);
    auto* decisionTitle = new QLabel(QStringLiteral("安全决策"), detail);
    decisionTitle->setObjectName(QStringLiteral("sectionTitle"));
    detailLayout->addWidget(decisionTitle);
    decision_ = new QLabel(QStringLiteral("等待下一次研判\n\n请求进入防护代理后，这里会展示风险等级、置信度、动作与理由。"), detail);
    decision_->setWordWrap(true);
    decision_->setObjectName(QStringLiteral("muted"));
    decision_->setAlignment(Qt::AlignTop);
    detailLayout->addWidget(decision_);
    auto* divider = new QFrame(detail);
    divider->setFrameShape(QFrame::HLine);
    detailLayout->addWidget(divider);
    auto* eventTitle = new QLabel(QStringLiteral("安全事件"), detail);
    eventTitle->setObjectName(QStringLiteral("sectionTitle"));
    detailLayout->addWidget(eventTitle);
    eventsList_ = new QListWidget(detail);
    eventsList_->setObjectName(QStringLiteral("securityEventList"));
    eventsList_->setWordWrap(true);
    eventsList_->addItem(QStringLiteral("暂无安全事件"));
    detailLayout->addWidget(eventsList_, 1);
    connect(eventsList_, &QListWidget::itemClicked, this, [this](QListWidgetItem* item) {
        const QByteArray raw = item->data(Qt::UserRole).toByteArray();
        if (raw.isEmpty()) return;
        const QJsonObject event = QJsonDocument::fromJson(raw).object();
        const QString detailText = QStringLiteral("时间：%1\n类型：%2\n动作：%3\n风险：%4\n来源：%5\n\n%6\n\nID：%7")
            .arg(event.value(QStringLiteral("timestamp")).toString(), event.value(QStringLiteral("event_type")).toString(),
                 event.value(QStringLiteral("action_desc")).toString(), event.value(QStringLiteral("risk_type")).toString(),
                 event.value(QStringLiteral("source")).toString(), event.value(QStringLiteral("detail")).toString(),
                 event.value(QStringLiteral("id")).toVariant().toString());
        QMessageBox::information(this, QStringLiteral("安全事件详情"), detailText);
    });
    splitter->addWidget(detail);
    splitter->setStretchFactor(0, 1);
    splitter->setStretchFactor(1, 0);
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
        sessionId_ = status.value(QStringLiteral("session_id")).toString(status.value(QStringLiteral("proxy_session_id")).toString(sessionId_));
        const bool running = status.value(QStringLiteral("running")).toBool(status.value(QStringLiteral("is_running")).toBool());
        auditOnly_->blockSignals(true);
        auditOnly_->setChecked(status.value(QStringLiteral("audit_only")).toBool());
        auditOnly_->blockSignals(false);
        stateLabel_->setText(running ? QStringLiteral("● 防护中") : QStringLiteral("● 未防护"));
        stateLabel_->setStyleSheet(running ? QStringLiteral("color:#22C55E;padding:5px 10px;border-radius:10px;background:rgba(34,197,94,25);") : QStringLiteral("color:#F59E0B;padding:5px 10px;border-radius:10px;background:rgba(245,158,11,25);"));
        requestCount_->setText(QString::number(metrics.value(QStringLiteral("request_count")).toInteger()));
        blockedCount_->setText(QString::number(metrics.value(QStringLiteral("blocked_count")).toInteger()));
        tokenCount_->setText(QString::number(metrics.value(QStringLiteral("total_tokens")).toInteger()));
        promptTokenCount_->setText(QString::number(metrics.value(QStringLiteral("total_prompt_tokens")).toInteger()));
        completionTokenCount_->setText(QString::number(metrics.value(QStringLiteral("total_completion_tokens")).toInteger()));
        toolCallCount_->setText(QString::number(metrics.value(QStringLiteral("total_tool_calls")).toInteger()));
        QList<int> tokenValues;
        for (const QJsonValue& value : metrics.value(QStringLiteral("token_trend")).toArray()) tokenValues.append(value.toObject().value(QStringLiteral("tokens")).toInt());
        QList<int> toolValues;
        for (const QJsonValue& value : metrics.value(QStringLiteral("tool_call_trend")).toArray()) toolValues.append(value.toObject().value(QStringLiteral("count")).toInt());
        tokenTrend_->setValues(tokenValues);
        toolTrend_->setValues(toolValues);
        refreshInFlight_ = false;
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, id = asset_.id]() {
        const QJsonObject query{{QStringLiteral("duration_seconds"), 86400}, {QStringLiteral("asset_id"), id}};
        return QJsonObject{{QStringLiteral("status"), bridge->call("GetProtectionProxyStatusByAsset", id)},
                           {QStringLiteral("metrics"), bridge->call("GetApiStatisticsFFI", QString::fromUtf8(QJsonDocument(query).toJson(QJsonDocument::Compact)))}};
    }));
}

void ProtectionMonitorWindow::refreshLogs() {
    if (bridge_ == nullptr || sessionId_.isEmpty()) return;
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonObject response = watcher->result();
        const QJsonObject data = response.value(QStringLiteral("data")).isObject() ? response.value(QStringLiteral("data")).toObject() : response;
        const QJsonArray logs = data.value(QStringLiteral("logs")).toArray();
        QStringList logLines;
        for (const QJsonValue& value : logs) logLines.append(value.toString());
        if (!logLines.isEmpty()) {
            groupedLog_->appendPlainText(logLines.join(QLatin1Char('\n')));
            rawLog_->appendPlainText(QString::fromUtf8(QJsonDocument(logs).toJson(QJsonDocument::Indented)));
        }
        requestCount_->setText(QString::number(data.value(QStringLiteral("request_count")).toInteger()));
        riskCount_->setText(QString::number(data.value(QStringLiteral("warning_count")).toInteger()));
        blockedCount_->setText(QString::number(data.value(QStringLiteral("blocked_count")).toInteger()));
        tokenCount_->setText(QString::number(data.value(QStringLiteral("total_tokens")).toInteger()));
        promptTokenCount_->setText(QString::number(data.value(QStringLiteral("total_prompt_tokens")).toInteger()));
        completionTokenCount_->setText(QString::number(data.value(QStringLiteral("total_completion_tokens")).toInteger()));
        toolCallCount_->setText(QString::number(data.value(QStringLiteral("total_tool_calls")).toInteger()));
        auditTokenCount_->setText(QString::number(data.value(QStringLiteral("audit_tokens")).toInteger()));
        const QJsonArray requestViews = data.value(QStringLiteral("request_views")).toArray();
        if (!requestViews.isEmpty()) {
            const QJsonObject latest = requestViews.last().toObject();
            decision_->setText(QString::fromUtf8(QJsonDocument(latest).toJson(QJsonDocument::Indented)));
            const QString action = latest.value(QStringLiteral("action")).toString(latest.value(QStringLiteral("decision")).toString());
            const QString risk = latest.value(QStringLiteral("risk_level")).toString();
        }
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, session = sessionId_]() { return bridge->call("GetProtectionProxyLogs", session); }));
}

void ProtectionMonitorWindow::refreshSecurityEvents() {
    if (bridge_ == nullptr) return;
    const QJsonObject filter{{QStringLiteral("limit"), 50}, {QStringLiteral("offset"), 0}, {QStringLiteral("asset_id"), asset_.id}};
    const QString payload = QString::fromUtf8(QJsonDocument(filter).toJson(QJsonDocument::Compact));
    auto* watcher = new QFutureWatcher<QJsonObject>(this);
    connect(watcher, &QFutureWatcher<QJsonObject>::finished, this, [this, watcher]() {
        const QJsonArray events = watcher->result().value(QStringLiteral("data")).toArray();
        eventsList_->clear();
        for (const QJsonValue& value : events) {
            const QJsonObject event = value.toObject();
            const QString type = event.value(QStringLiteral("event_type")).toString(QStringLiteral("安全事件"));
            const QString action = event.value(QStringLiteral("action_desc")).toString(event.value(QStringLiteral("detail")).toString());
            const QString time = QDateTime::fromString(event.value(QStringLiteral("timestamp")).toString(), Qt::ISODate).toLocalTime().toString(QStringLiteral("MM-dd HH:mm:ss"));
            auto* item = new QListWidgetItem(QStringLiteral("%1  %2\n%3").arg(time, type, action), eventsList_);
            item->setData(Qt::UserRole, QJsonDocument(event).toJson(QJsonDocument::Compact));
            item->setToolTip(QStringLiteral("点击查看事件详情"));
        }
        if (eventsList_->count() == 0) eventsList_->addItem(QStringLiteral("暂无安全事件"));
        watcher->deleteLater();
    });
    watcher->setFuture(QtConcurrent::run([bridge = bridge_, payload]() { return bridge->call("GetSecurityEventsFFI", payload); }));
}
