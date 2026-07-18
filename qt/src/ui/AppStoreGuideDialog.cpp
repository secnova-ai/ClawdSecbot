#include "ui/dialogs/AppStoreGuideDialog.h"

#include "ui/DialogChrome.h"

#include <QDialogButtonBox>
#include <QFrame>
#include <QHBoxLayout>
#include <QLabel>
#include <QPushButton>
#include <QVBoxLayout>

AppStoreGuideDialog::AppStoreGuideDialog(QWidget* parent) : QDialog(parent) {
    setWindowTitle(QStringLiteral("配置引导"));
    setProperty("tone", DialogChrome::toneName(DialogChrome::Tone::Info));
    DialogChrome::prepare(this, QSize(720, 570), QSize(650, 500));
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(26, 24, 26, 22);
    root->setSpacing(18);
    root->addWidget(DialogChrome::createHeader(this, QStringLiteral("▤"), QStringLiteral("配置引导"),
                                                QStringLiteral("将 Bot 模型端点安全切换到本地防护代理"), DialogChrome::Tone::Info));
    auto* steps = new QWidget(this);
    auto* stepsLayout = new QVBoxLayout(steps);
    stepsLayout->setContentsMargins(0, 0, 0, 0);
    stepsLayout->setSpacing(10);
    const QList<QPair<QString, QString>> guideSteps{
        {QStringLiteral("打开 Dashboard"), QStringLiteral("执行 openclaw dashboard --no-open，进入 Settings > Config")},
        {QStringLiteral("添加代理供应商"), QStringLiteral("Base URL 设置为 http://127.0.0.1:13436")},
        {QStringLiteral("更新默认模型"), QStringLiteral("将 Primary Model 指向 ClawdSecbot provider")},
        {QStringLiteral("保存并验证"), QStringLiteral("点击 Save 和 Reload，然后发送一条测试消息")},
    };
    for (int index = 0; index < guideSteps.size(); ++index) {
        auto* card = new QFrame(steps);
        card->setObjectName(QStringLiteral("guideStepCard"));
        auto* layout = new QHBoxLayout(card);
        layout->setContentsMargins(14, 12, 14, 12);
        layout->setSpacing(12);
        auto* number = new QLabel(QString::number(index + 1), card);
        number->setObjectName(QStringLiteral("guideStepNumber"));
        number->setAlignment(Qt::AlignCenter);
        number->setFixedSize(30, 30);
        layout->addWidget(number, 0, Qt::AlignTop);
        auto* copy = new QWidget(card);
        auto* copyLayout = new QVBoxLayout(copy);
        copyLayout->setContentsMargins(0, 0, 0, 0);
        copyLayout->setSpacing(3);
        auto* title = new QLabel(guideSteps.at(index).first, copy);
        title->setObjectName(QStringLiteral("sectionTitle"));
        copyLayout->addWidget(title);
        auto* detail = new QLabel(guideSteps.at(index).second, copy);
        detail->setObjectName(QStringLiteral("muted"));
        detail->setWordWrap(true);
        detail->setTextInteractionFlags(Qt::TextSelectableByMouse);
        copyLayout->addWidget(detail);
        layout->addWidget(copy, 1);
        stepsLayout->addWidget(card);
    }
    root->addWidget(steps, 1);
    auto* status = new QLabel(QStringLiteral("完成后返回 ClawdSecbot，防护状态会自动刷新。"), this);
    status->setObjectName(QStringLiteral("dialogHint"));
    status->setWordWrap(true);
    root->addWidget(status);
    auto* buttons = new QDialogButtonBox(QDialogButtonBox::Close, this);
    DialogChrome::styleButtonBox(buttons);
    buttons->button(QDialogButtonBox::Close)->setText(QStringLiteral("完成"));
    buttons->button(QDialogButtonBox::Close)->setObjectName(QStringLiteral("primaryButton"));
    connect(buttons, &QDialogButtonBox::rejected, this, &QDialog::accept);
    connect(buttons, &QDialogButtonBox::accepted, this, &QDialog::accept);
    root->addWidget(buttons);
}
