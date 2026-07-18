#pragma once

#include <QDialog>

class GoBridge;
class QJsonObject;
class QLabel;
class QPlainTextEdit;
class QProgressBar;
class QPushButton;
class QScrollArea;
class QStackedWidget;
class QTimer;
class QVBoxLayout;

class SkillScanDialog final : public QDialog {
    Q_OBJECT
public:
    SkillScanDialog(GoBridge* bridge, const QString& assetName = {}, QWidget* parent = nullptr);
    void reject() override;

private:
    void startScan();
    void pollBatch();
    void loadResults();
    void renderResults(const QJsonObject& response);
    void finishWithError(const QString& error);

    GoBridge* bridge_;
    QString assetName_;
    QString batchId_;
    QLabel* status_ = nullptr;
    QStackedWidget* pages_ = nullptr;
    QProgressBar* progress_ = nullptr;
    QPlainTextEdit* logView_ = nullptr;
    QScrollArea* resultsScroll_ = nullptr;
    QWidget* resultsContent_ = nullptr;
    QVBoxLayout* resultsLayout_ = nullptr;
    QPushButton* closeButton_ = nullptr;
    QTimer* pollTimer_ = nullptr;
    bool polling_ = false;
    bool startInFlight_ = false;
    bool closeRequested_ = false;
};
