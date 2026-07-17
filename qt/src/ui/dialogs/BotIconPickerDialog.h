#pragma once

#include <QDialog>

class QLabel;

class BotIconPickerDialog final : public QDialog {
    Q_OBJECT
public:
    explicit BotIconPickerDialog(const QString& currentIcon = QStringLiteral("package"),
                                 quint32 currentColor = 0xFF6366F1,
                                 QWidget* parent = nullptr);
    QString selectedIcon() const;
    quint32 selectedColor() const;
    static QString glyphForName(const QString& iconName);

private:
    QString selectedIcon_;
    quint32 selectedColor_ = 0xFF6366F1;
    QLabel* preview_ = nullptr;
};
