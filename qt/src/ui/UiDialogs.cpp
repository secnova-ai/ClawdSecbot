#include "ui/UiDialogs.h"

#include <QDialog>
#include <QFrame>
#include <QHBoxLayout>
#include <QLabel>
#include <QProgressBar>
#include <QPushButton>
#include <QVBoxLayout>

#include <utility>

namespace {
QString glyphForTone(DialogChrome::Tone tone) {
    switch (tone) {
        case DialogChrome::Tone::Info: return QStringLiteral("i");
        case DialogChrome::Tone::Success: return QStringLiteral("✓");
        case DialogChrome::Tone::Warning: return QStringLiteral("!");
        case DialogChrome::Tone::Danger: return QStringLiteral("×");
        case DialogChrome::Tone::Accent: return QStringLiteral("◇");
    }
    return QStringLiteral("◇");
}

QString buttonObjectName(UiDialogs::ButtonStyle style) {
    switch (style) {
        case UiDialogs::ButtonStyle::Primary: return QStringLiteral("primaryButton");
        case UiDialogs::ButtonStyle::Danger: return QStringLiteral("dangerActionButton");
        case UiDialogs::ButtonStyle::Secondary: return QStringLiteral("secondaryButton");
    }
    return QStringLiteral("secondaryButton");
}

QFrame* messageBody(QDialog* dialog, const QString& text) {
    auto* body = new QFrame(dialog);
    body->setObjectName(QStringLiteral("dialogMessageBody"));
    auto* layout = new QVBoxLayout(body);
    layout->setContentsMargins(16, 15, 16, 15);
    auto* label = new QLabel(text, body);
    label->setObjectName(QStringLiteral("dialogMessageText"));
    label->setWordWrap(true);
    label->setTextInteractionFlags(Qt::TextSelectableByMouse);
    layout->addWidget(label);
    return body;
}

void showMessage(QWidget* parent, DialogChrome::Tone tone, const QString& title, const QString& text) {
    UiDialogs::choose(parent, title, text, tone,
                      {{QStringLiteral("ok"), QStringLiteral("知道了"), UiDialogs::ButtonStyle::Primary, true}},
                      [](const QString&) {});
}
}

void UiDialogs::showWarning(QWidget* parent, const QString& title, const QString& text) {
    showMessage(parent, DialogChrome::Tone::Warning, title, text);
}

void UiDialogs::showInformation(QWidget* parent, const QString& title, const QString& text) {
    showMessage(parent, DialogChrome::Tone::Success, title, text);
}

void UiDialogs::showAbout(QWidget* parent, const QString& title, const QString& text) {
    showMessage(parent, DialogChrome::Tone::Accent, title, text);
}

void UiDialogs::confirm(QWidget* parent, const QString& title, const QString& text, std::function<void()> onAccepted,
                        const QString& acceptText) {
    const bool destructive = acceptText.contains(QStringLiteral("删除")) || acceptText.contains(QStringLiteral("清空"))
        || acceptText.contains(QStringLiteral("停止")) || acceptText.contains(QStringLiteral("恢复"));
    choose(parent, title, text, destructive ? DialogChrome::Tone::Warning : DialogChrome::Tone::Accent,
           {{QStringLiteral("cancel"), QStringLiteral("取消"), ButtonStyle::Secondary, false},
            {QStringLiteral("accept"), acceptText, destructive ? ButtonStyle::Danger : ButtonStyle::Primary, true}},
           [onAccepted = std::move(onAccepted)](const QString& id) mutable {
        if (id == QStringLiteral("accept") && onAccepted) onAccepted();
    });
}

QDialog* UiDialogs::choose(QWidget* parent, const QString& title, const QString& text, DialogChrome::Tone tone,
                           const QList<Choice>& choices, std::function<void(const QString&)> onFinished) {
    auto* dialog = new QDialog(parent);
    dialog->setAttribute(Qt::WA_DeleteOnClose);
    dialog->setWindowTitle(title);
    dialog->setProperty("tone", DialogChrome::toneName(tone));
    DialogChrome::prepare(dialog, QSize(440, 0), QSize(420, 0));
    auto* root = new QVBoxLayout(dialog);
    root->setContentsMargins(24, 22, 24, 20);
    root->setSpacing(18);
    root->addWidget(DialogChrome::createHeader(dialog, glyphForTone(tone), title, {}, tone));
    root->addWidget(messageBody(dialog, text));
    auto* footer = new QWidget(dialog);
    footer->setObjectName(QStringLiteral("dialogFooter"));
    auto* footerLayout = new QHBoxLayout(footer);
    footerLayout->setContentsMargins(0, 0, 0, 0);
    footerLayout->setSpacing(10);
    footerLayout->addStretch();
    for (const Choice& choice : choices) {
        auto* button = new QPushButton(choice.text, footer);
        button->setObjectName(buttonObjectName(choice.style));
        button->setMinimumWidth(choice.style == ButtonStyle::Secondary ? 88 : 104);
        if (choice.isDefault) button->setDefault(true);
        QObject::connect(button, &QPushButton::clicked, dialog, [dialog, id = choice.id]() {
            dialog->setProperty("selectedChoice", id);
            dialog->accept();
        });
        footerLayout->addWidget(button);
    }
    root->addWidget(footer);
    QObject::connect(dialog, &QDialog::finished, dialog, [dialog, onFinished = std::move(onFinished)](int) mutable {
        if (onFinished) onFinished(dialog->property("selectedChoice").toString());
    });
    dialog->adjustSize();
    dialog->setMinimumWidth(420);
    dialog->setMaximumWidth(560);
    dialog->open();
    return dialog;
}

QDialog* UiDialogs::showProgress(QWidget* parent, const QString& title, const QString& text) {
    auto* dialog = new QDialog(parent);
    dialog->setAttribute(Qt::WA_DeleteOnClose);
    dialog->setWindowTitle(title);
    dialog->setProperty("tone", DialogChrome::toneName(DialogChrome::Tone::Info));
    DialogChrome::prepare(dialog, QSize(440, 0), QSize(420, 0));
    auto* root = new QVBoxLayout(dialog);
    root->setContentsMargins(24, 22, 24, 22);
    root->setSpacing(18);
    root->addWidget(DialogChrome::createHeader(dialog, QStringLiteral("◌"), title, {}, DialogChrome::Tone::Info, false));
    root->addWidget(messageBody(dialog, text));
    auto* progress = new QProgressBar(dialog);
    progress->setObjectName(QStringLiteral("dialogProgress"));
    progress->setRange(0, 0);
    progress->setTextVisible(false);
    root->addWidget(progress);
    dialog->adjustSize();
    dialog->setMinimumWidth(420);
    dialog->setMaximumWidth(560);
    dialog->open();
    return dialog;
}
