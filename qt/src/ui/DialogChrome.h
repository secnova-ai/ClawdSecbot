#pragma once

#include <QSize>
#include <QString>

class QDialog;
class QDialogButtonBox;
class QWidget;

namespace DialogChrome {
enum class Tone {
    Accent,
    Info,
    Success,
    Warning,
    Danger,
};

void prepare(QDialog* dialog, const QSize& preferredSize, const QSize& minimumSize = {});
QWidget* createHeader(QDialog* dialog, const QString& glyph, const QString& title,
                      const QString& subtitle = {}, Tone tone = Tone::Accent, bool closable = true);
void styleButtonBox(QDialogButtonBox* buttons);
QString toneName(Tone tone);
}
