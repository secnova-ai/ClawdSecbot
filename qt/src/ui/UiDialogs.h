#pragma once

#include "ui/DialogChrome.h"

#include <functional>

#include <QList>
#include <QString>

class QDialog;
class QWidget;

namespace UiDialogs {
enum class ButtonStyle {
    Secondary,
    Primary,
    Danger,
};

struct Choice {
    QString id;
    QString text;
    ButtonStyle style = ButtonStyle::Secondary;
    bool isDefault = false;
};

void showWarning(QWidget* parent, const QString& title, const QString& text);
void showInformation(QWidget* parent, const QString& title, const QString& text);
void showAbout(QWidget* parent, const QString& title, const QString& text);
void confirm(QWidget* parent, const QString& title, const QString& text, std::function<void()> onAccepted,
             const QString& acceptText = QStringLiteral("确定"));
QDialog* choose(QWidget* parent, const QString& title, const QString& text, DialogChrome::Tone tone,
                const QList<Choice>& choices, std::function<void(const QString&)> onFinished);
QDialog* showProgress(QWidget* parent, const QString& title, const QString& text);
}
