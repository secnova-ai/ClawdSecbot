#include "ui/dialogs/AppStoreGuideDialog.h"

#include <QDialogButtonBox>
#include <QLabel>
#include <QPlainTextEdit>
#include <QPushButton>
#include <QVBoxLayout>

AppStoreGuideDialog::AppStoreGuideDialog(QWidget* parent) : QDialog(parent) {
    setWindowFlags(Qt::Dialog | Qt::FramelessWindowHint);
    setWindowTitle(QStringLiteral("配置引导"));
    resize(680, 500);
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(24, 24, 24, 20);
    root->setSpacing(14);
    auto* title = new QLabel(QStringLiteral("配置引导"), this);
    title->setObjectName(QStringLiteral("dialogTitle"));
    root->addWidget(title);
    auto* detail = new QLabel(QStringLiteral("启动防护后，请将 Bot 模型端点切换到本地代理。按以下步骤完成配置。"), this);
    detail->setWordWrap(true);
    detail->setObjectName(QStringLiteral("muted"));
    root->addWidget(detail);
    const QString promptText = QStringLiteral(
        "1. 执行 openclaw dashboard --no-open 并进入 Settings > Config\n\n"
        "2. 添加 ClawdSecbot 代理供应商，Base URL 使用 http://127.0.0.1:13436\n\n"
        "3. 将 Primary Model 指向 ClawdSecbot provider\n\n"
        "4. 点击 Save，再点击 Reload，然后发送一条验证消息");
    auto* prompt = new QPlainTextEdit(promptText, this);
    prompt->setReadOnly(true);
    root->addWidget(prompt, 1);
    auto* status = new QLabel(QStringLiteral("请完成以上配置后返回"), this);
    status->setObjectName(QStringLiteral("muted"));
    root->addWidget(status);
    auto* buttons = new QDialogButtonBox(QDialogButtonBox::Close, this);
    buttons->button(QDialogButtonBox::Close)->setText(QStringLiteral("完成"));
    buttons->button(QDialogButtonBox::Close)->setObjectName(QStringLiteral("primaryButton"));
    connect(buttons, &QDialogButtonBox::rejected, this, &QDialog::accept);
    connect(buttons, &QDialogButtonBox::accepted, this, &QDialog::accept);
    root->addWidget(buttons);
}
