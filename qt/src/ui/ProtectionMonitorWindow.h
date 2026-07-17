#pragma once

#include "domain/Models.h"

#include <QMainWindow>

class GoBridge;
class QLabel;
class QCheckBox;
class QListWidget;
class QPlainTextEdit;
class QTabWidget;
class QTimer;
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

    AssetModel asset_;
    GoBridge* bridge_;
    QLabel* stateLabel_ = nullptr;
    QLabel* requestCount_ = nullptr;
    QLabel* riskCount_ = nullptr;
    QLabel* blockedCount_ = nullptr;
    QLabel* tokenCount_ = nullptr;
    QLabel* promptTokenCount_ = nullptr;
    QLabel* completionTokenCount_ = nullptr;
    QLabel* toolCallCount_ = nullptr;
    QLabel* auditTokenCount_ = nullptr;
    QLabel* decision_ = nullptr;
    QCheckBox* auditOnly_ = nullptr;
    QListWidget* eventsList_ = nullptr;
    QPlainTextEdit* groupedLog_ = nullptr;
    QPlainTextEdit* rawLog_ = nullptr;
    TrendChartWidget* tokenTrend_ = nullptr;
    TrendChartWidget* toolTrend_ = nullptr;
    QTimer* refreshTimer_ = nullptr;
    QString sessionId_;
    bool refreshInFlight_ = false;
};
