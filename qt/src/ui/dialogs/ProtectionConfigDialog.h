#pragma once

#include "domain/Models.h"

#include <QDialog>
#include <QJsonArray>

class GoBridge;
class QCheckBox;
class QComboBox;
class QLineEdit;
class QTabWidget;
class QVBoxLayout;

class ProtectionConfigDialog final : public QDialog {
    Q_OBJECT
public:
    ProtectionConfigDialog(const AssetModel& asset, GoBridge* bridge, QWidget* parent = nullptr);
    QString sessionId() const;

private:
    void loadConfig();
    void saveConfig();
    void appendRuleCard(const QJsonObject& rule);

    AssetModel asset_;
    GoBridge* bridge_;
    QTabWidget* tabs_ = nullptr;
    QLineEdit* newRule_ = nullptr;
    QWidget* rulesContainer_ = nullptr;
    QVBoxLayout* rulesLayout_ = nullptr;
    QList<QCheckBox*> ruleChecks_;
    QJsonArray shepherdRules_;
    QString sessionId_;
    QLineEdit* tokenLimit_ = nullptr;
    QLineEdit* dailyTokenLimit_ = nullptr;
    QComboBox* pathMode_ = nullptr;
    QLineEdit* paths_ = nullptr;
    QComboBox* inboundMode_ = nullptr;
    QLineEdit* inboundAddresses_ = nullptr;
    QComboBox* outboundMode_ = nullptr;
    QLineEdit* outboundAddresses_ = nullptr;
    QComboBox* shellMode_ = nullptr;
    QLineEdit* shellCommands_ = nullptr;
    QComboBox* botProvider_ = nullptr;
    QLineEdit* botBaseUrl_ = nullptr;
    QLineEdit* botApiKey_ = nullptr;
    QLineEdit* botModel_ = nullptr;
    QLineEdit* botSecretKey_ = nullptr;
    QCheckBox* auditOnly_ = nullptr;
    QCheckBox* userInputDetection_ = nullptr;
    QCheckBox* sandboxEnabled_ = nullptr;
};
