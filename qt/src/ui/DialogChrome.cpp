#include "ui/DialogChrome.h"

#include <QDialog>
#include <QAbstractButton>
#include <QDialogButtonBox>
#include <QEvent>
#include <QHBoxLayout>
#include <QLabel>
#include <QMouseEvent>
#include <QPropertyAnimation>
#include <QPushButton>
#include <QStyle>
#include <QTimer>
#include <QVBoxLayout>
#include <QWindow>

namespace {
class DialogAnimator final : public QObject {
public:
    explicit DialogAnimator(QDialog* dialog) : QObject(dialog), dialog_(dialog), animation_(dialog, "windowOpacity", this) {
        animation_.setDuration(140);
        animation_.setStartValue(0.0);
        animation_.setEndValue(1.0);
    }

protected:
    bool eventFilter(QObject* watched, QEvent* event) override {
        if (watched == dialog_ && event->type() == QEvent::Show) {
            dialog_->setWindowOpacity(0.0);
            animation_.start();
        }
        return QObject::eventFilter(watched, event);
    }

private:
    QDialog* dialog_;
    QPropertyAnimation animation_;
};

class DialogHeader final : public QWidget {
public:
    explicit DialogHeader(QDialog* dialog) : QWidget(dialog), dialog_(dialog) {
        setObjectName(QStringLiteral("dialogHeader"));
        setCursor(Qt::SizeAllCursor);
    }

protected:
    void mousePressEvent(QMouseEvent* event) override {
        if (event->button() == Qt::LeftButton && dialog_ != nullptr && dialog_->windowHandle() != nullptr) {
            dialog_->windowHandle()->startSystemMove();
            event->accept();
            return;
        }
        QWidget::mousePressEvent(event);
    }

private:
    QDialog* dialog_;
};
}

QString DialogChrome::toneName(Tone tone) {
    switch (tone) {
        case Tone::Info: return QStringLiteral("info");
        case Tone::Success: return QStringLiteral("success");
        case Tone::Warning: return QStringLiteral("warning");
        case Tone::Danger: return QStringLiteral("danger");
        case Tone::Accent: return QStringLiteral("accent");
    }
    return QStringLiteral("accent");
}

void DialogChrome::prepare(QDialog* dialog, const QSize& preferredSize, const QSize& minimumSize) {
    if (dialog == nullptr) return;
    dialog->setWindowFlags(Qt::Dialog | Qt::FramelessWindowHint);
    dialog->setAttribute(Qt::WA_TranslucentBackground);
    dialog->setObjectName(QStringLiteral("styledDialog"));
    dialog->setModal(true);
    if (minimumSize.isValid()) dialog->setMinimumSize(minimumSize);
    if (preferredSize.isValid()) dialog->resize(preferredSize);
    auto* animator = new DialogAnimator(dialog);
    dialog->installEventFilter(animator);
}

QWidget* DialogChrome::createHeader(QDialog* dialog, const QString& glyph, const QString& title,
                                    const QString& subtitle, Tone tone, bool closable) {
    auto* header = new DialogHeader(dialog);
    auto* layout = new QHBoxLayout(header);
    layout->setContentsMargins(0, 0, 0, 0);
    layout->setSpacing(12);
    auto* icon = new QLabel(glyph, header);
    icon->setObjectName(QStringLiteral("dialogHeaderIcon"));
    icon->setProperty("tone", toneName(tone));
    icon->setAlignment(Qt::AlignCenter);
    icon->setFixedSize(42, 42);
    icon->setAttribute(Qt::WA_TransparentForMouseEvents);
    layout->addWidget(icon, 0, Qt::AlignTop);
    auto* copy = new QWidget(header);
    copy->setAttribute(Qt::WA_TransparentForMouseEvents);
    auto* copyLayout = new QVBoxLayout(copy);
    copyLayout->setContentsMargins(0, subtitle.isEmpty() ? 8 : 2, 0, 0);
    copyLayout->setSpacing(2);
    auto* titleLabel = new QLabel(title, copy);
    titleLabel->setObjectName(QStringLiteral("dialogTitle"));
    copyLayout->addWidget(titleLabel);
    if (!subtitle.isEmpty()) {
        auto* subtitleLabel = new QLabel(subtitle, copy);
        subtitleLabel->setObjectName(QStringLiteral("dialogSubtitle"));
        subtitleLabel->setWordWrap(true);
        copyLayout->addWidget(subtitleLabel);
    }
    layout->addWidget(copy, 1, Qt::AlignTop);
    if (closable) {
        auto* close = new QPushButton(QStringLiteral("×"), header);
        close->setObjectName(QStringLiteral("dialogCloseButton"));
        close->setToolTip(QStringLiteral("关闭"));
        close->setAccessibleName(QStringLiteral("关闭弹窗"));
        close->setCursor(Qt::PointingHandCursor);
        QObject::connect(close, &QPushButton::clicked, dialog, &QDialog::reject);
        layout->addWidget(close, 0, Qt::AlignTop);
    }
    return header;
}

void DialogChrome::styleButtonBox(QDialogButtonBox* buttons) {
    if (buttons == nullptr) return;
    buttons->setObjectName(QStringLiteral("dialogButtonBox"));
    buttons->setCenterButtons(false);
    QTimer::singleShot(0, buttons, [buttons]() {
        for (QAbstractButton* button : buttons->buttons()) {
            if (button->objectName() == QStringLiteral("primaryButton")) {
                button->setStyleSheet(QStringLiteral(
                    "QPushButton{background:#6366F1;color:#FFFFFF;font-weight:600;padding:11px 22px;border-radius:8px;}"
                    "QPushButton:hover{background:#7779F5;}"
                    "QPushButton:disabled{background:rgba(99,102,241,28);color:rgba(255,255,255,65);}"));
            }
            button->style()->unpolish(button);
            button->style()->polish(button);
        }
    });
}
