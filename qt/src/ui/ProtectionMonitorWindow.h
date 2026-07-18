#pragma once

#include "domain/Models.h"

#include <QMainWindow>

class GoBridge;
class QLabel;
class QCheckBox;
class QFrame;
class QPushButton;
class QStackedWidget;
class QTimer;
class AnalysisLogView;
class RawLogView;
class SecurityEventListWidget;
class TrendChartWidget;

class ProtectionMonitorWindow final : public QMainWindow {
    Q_OBJECT
public:
    ProtectionMonitorWindow(const AssetModel& asset, GoBridge* bridge, const QString& sessionId = {}, QWidget* parent = nullptr);
private:
    void buildUi();
    void refreshStatus();
    void refreshLogs();
    void refreshSecurityEvents();
    void clearSecurityEvents();
    void updateEventCount();
    void updateErrorBanner();

    AssetModel asset_;
    GoBridge* bridge_;
    QFrame* statusCard_ = nullptr;
    QLabel* statusDot_ = nullptr;
    QLabel* stateLabel_ = nullptr;
    QLabel* errorLabel_ = nullptr;
    QLabel* requestCount_ = nullptr;
    QLabel* messageCount_ = nullptr;
    QLabel* riskCount_ = nullptr;
    QLabel* blockedCount_ = nullptr;
    QLabel* tokenCount_ = nullptr;
    QLabel* promptTokenCount_ = nullptr;
    QLabel* completionTokenCount_ = nullptr;
    QLabel* toolCallCount_ = nullptr;
    QLabel* auditTokenCount_ = nullptr;
    QLabel* auditPromptTokenCount_ = nullptr;
    QLabel* auditCompletionTokenCount_ = nullptr;
    QLabel* eventCount_ = nullptr;
    QPushButton* clearEventsButton_ = nullptr;
    QCheckBox* auditOnly_ = nullptr;
    SecurityEventListWidget* eventsList_ = nullptr;
    QStackedWidget* eventStack_ = nullptr;
    AnalysisLogView* groupedLog_ = nullptr;
    RawLogView* rawLog_ = nullptr;
    TrendChartWidget* tokenTrend_ = nullptr;
    TrendChartWidget* toolTrend_ = nullptr;
    QTimer* refreshTimer_ = nullptr;
    QString sessionId_;
    QString statusError_;
    QString metricsError_;
    QString logsError_;
    QString eventsError_;
    bool refreshInFlight_ = false;
    bool logsInFlight_ = false;
    bool eventsInFlight_ = false;
    bool eventsRefreshPending_ = false;
    bool clearEventsInFlight_ = false;
    quint64 eventsGeneration_ = 0;
};
