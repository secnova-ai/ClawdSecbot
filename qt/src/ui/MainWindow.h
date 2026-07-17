#pragma once

#include "domain/Models.h"

#include <QMainWindow>

class GoBridge;
class QLabel;
class QProgressBar;
class QPushButton;
class QStackedWidget;
class QVBoxLayout;
class AssetCardWidget;

class MainWindow final : public QMainWindow {
    Q_OBJECT
public:
    explicit MainWindow(GoBridge* bridge, QWidget* parent = nullptr);
    void initializeData();

protected:
    bool eventFilter(QObject* watched, QEvent* event) override;

private:
    void buildUi();
    QWidget* buildTitleBar();
    QWidget* buildIdlePage();
    QWidget* buildScanningPage();
    QWidget* buildResultsPage();
    void loadLatestResult();
    void showOnboardingIfNeeded();
    void startScan();
    void renderResult(const ScanResultModel& result);
    void refreshProtectionStates();
    void stopProtection(const AssetModel& asset);
    void openSettings();
    void openAuditLog();
    void openProtectionConfig(const AssetModel& asset);
    void openProtectionMonitor(const AssetModel& asset, const QString& sessionId = {});

    GoBridge* bridge_;
    QStackedWidget* pages_ = nullptr;
    QProgressBar* scanProgress_ = nullptr;
    QLabel* scanStep_ = nullptr;
    QLabel* resultTitle_ = nullptr;
    QLabel* resultTime_ = nullptr;
    QWidget* resultContent_ = nullptr;
    QVBoxLayout* resultLayout_ = nullptr;
    QPushButton* scanButton_ = nullptr;
    ScanResultModel result_;
    QList<AssetCardWidget*> assetCards_;
};
