#pragma once

#include <QDialog>

class GoBridge;
class QComboBox;
class QCheckBox;
class QLineEdit;
class QPushButton;
class QTabWidget;

class SettingsDialog final : public QDialog {
    Q_OBJECT
public:
    explicit SettingsDialog(GoBridge* bridge, QWidget* parent = nullptr);

signals:
    void scheduledScanIntervalChanged(int intervalSeconds);

private:
    void loadModelConfig();
    void saveCurrentTab();

    GoBridge* bridge_;
    QTabWidget* tabs_ = nullptr;
    QComboBox* provider_ = nullptr;
    QComboBox* scheduleCombo_ = nullptr;
    QCheckBox* startupCheck_ = nullptr;
    QLineEdit* baseUrl_ = nullptr;
    QLineEdit* apiKey_ = nullptr;
    QLineEdit* modelName_ = nullptr;
    QPushButton* saveButton_ = nullptr;
};
