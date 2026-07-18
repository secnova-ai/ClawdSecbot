#pragma once

#include "domain/Models.h"

#include <QFrame>
#include <QColor>

class QLabel;
class QPushButton;
class QVBoxLayout;
class QWidget;

class AssetCardWidget final : public QFrame {
    Q_OBJECT

public:
    explicit AssetCardWidget(const AssetModel& asset, bool initiallyExpanded, QWidget* parent = nullptr);

    const AssetModel& asset() const;
    void setProtected(bool protectedState);
    void setOperationInProgress(bool inProgress);
    void setIconAppearance(const QString& iconName, const QString& glyph, const QColor& color);
    QString iconName() const;
    QColor iconColor() const;

signals:
    void configureRequested();
    void monitorRequested();
    void stopRequested();
    void iconPickerRequested();

protected:
    void mousePressEvent(QMouseEvent* event) override;

private:
    void buildUi();
    void toggleExpanded();
    void updateExpandedState();
    void updateProtectionState();
    QWidget* buildDetails();
    QString bindPortSummary() const;
    QString assetTypeText() const;
    bool isAssetRunning() const;

    AssetModel asset_;
    bool expanded_ = false;
    bool protected_ = false;
    bool operationInProgress_ = false;
    QWidget* header_ = nullptr;
    QWidget* details_ = nullptr;
    QLabel* statusBadge_ = nullptr;
    QLabel* chevron_ = nullptr;
    QPushButton* iconButton_ = nullptr;
    QString iconName_ = QStringLiteral("package");
    QColor iconColor_ = QColor(QStringLiteral("#6366F1"));
    QVBoxLayout* actionsLayout_ = nullptr;
};
