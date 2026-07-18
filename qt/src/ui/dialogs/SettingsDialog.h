#pragma once

#include <QDialog>

class GoBridge;
class QComboBox;
class QLineEdit;
class QPushButton;
class QTabWidget;

class SettingsDialog final : public QDialog {
    Q_OBJECT
public:
    explicit SettingsDialog(GoBridge* bridge, QWidget* parent = nullptr);

private:
    void loadModelConfig();
    void saveCurrentTab();

    GoBridge* bridge_;
    QTabWidget* tabs_ = nullptr;
    QComboBox* provider_ = nullptr;
    QLineEdit* baseUrl_ = nullptr;
    QLineEdit* apiKey_ = nullptr;
    QLineEdit* modelName_ = nullptr;
    QPushButton* saveButton_ = nullptr;
};
