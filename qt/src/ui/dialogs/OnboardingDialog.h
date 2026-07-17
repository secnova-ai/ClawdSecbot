#pragma once

#include <QDialog>

class GoBridge;
class QLabel;
class QComboBox;
class QLineEdit;
class QPushButton;
class QStackedWidget;

class OnboardingDialog final : public QDialog {
    Q_OBJECT
public:
    explicit OnboardingDialog(GoBridge* bridge, QWidget* parent = nullptr);

private:
    void moveStep(int delta);
    void saveBotAndContinue();

    GoBridge* bridge_;
    QStackedWidget* pages_ = nullptr;
    QLabel* stepLabel_ = nullptr;
    QPushButton* nextButton_ = nullptr;
    QComboBox* onboardingBotProvider_ = nullptr;
    QLineEdit* onboardingBotBaseUrl_ = nullptr;
    QLineEdit* onboardingBotApiKey_ = nullptr;
    QLineEdit* onboardingBotModel_ = nullptr;
    QString onboardingAssetId_;
};
