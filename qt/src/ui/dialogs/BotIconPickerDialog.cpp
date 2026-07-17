#include "ui/dialogs/BotIconPickerDialog.h"

#include <QAbstractButton>
#include <QButtonGroup>
#include <QColor>
#include <QDialogButtonBox>
#include <QGridLayout>
#include <QHash>
#include <QHBoxLayout>
#include <QLabel>
#include <QPushButton>
#include <QVariant>
#include <QVBoxLayout>

namespace {
QLabel* titleLabel(const QString& text, QWidget* parent) {
    auto* label = new QLabel(text, parent);
    label->setObjectName(QStringLiteral("dialogTitle"));
    return label;
}
}

QString BotIconPickerDialog::glyphForName(const QString& iconName) {
    static const QHash<QString, QString> glyphs{
        {QStringLiteral("scissors"), QStringLiteral("✂")}, {QStringLiteral("bug"), QStringLiteral("♨")}, {QStringLiteral("bot"), QStringLiteral("♙")}, {QStringLiteral("shield"), QStringLiteral("◇")},
        {QStringLiteral("globe"), QStringLiteral("◎")}, {QStringLiteral("server"), QStringLiteral("▤")}, {QStringLiteral("zap"), QStringLiteral("ϟ")}, {QStringLiteral("star"), QStringLiteral("☆")},
        {QStringLiteral("flame"), QStringLiteral("♨")}, {QStringLiteral("cloud"), QStringLiteral("☁")}, {QStringLiteral("box"), QStringLiteral("□")}, {QStringLiteral("heart"), QStringLiteral("♡")},
        {QStringLiteral("target"), QStringLiteral("◉")}, {QStringLiteral("anchor"), QStringLiteral("⚓")}, {QStringLiteral("compass"), QStringLiteral("✥")}, {QStringLiteral("crown"), QStringLiteral("♛")},
        {QStringLiteral("diamond"), QStringLiteral("◇")}, {QStringLiteral("gem"), QStringLiteral("◈")}, {QStringLiteral("rocket"), QStringLiteral("↟")}, {QStringLiteral("swords"), QStringLiteral("⚔")},
        {QStringLiteral("eye"), QStringLiteral("◉")}, {QStringLiteral("lock"), QStringLiteral("▣")}, {QStringLiteral("key"), QStringLiteral("⚿")}, {QStringLiteral("cpu"), QStringLiteral("▦")},
        {QStringLiteral("file-json"), QStringLiteral("▱")}, {QStringLiteral("package"), QStringLiteral("▣")}, {QStringLiteral("terminal"), QStringLiteral(">_ ")}, {QStringLiteral("wifi"), QStringLiteral("⌁")},
    };
    return glyphs.value(iconName, QStringLiteral("▣"));
}

BotIconPickerDialog::BotIconPickerDialog(const QString& currentIcon, quint32 currentColor, QWidget* parent) : QDialog(parent) {
    setWindowFlags(Qt::Dialog | Qt::FramelessWindowHint);
    setWindowTitle(QStringLiteral("选择图标"));
    resize(400, 430);
    selectedIcon_ = currentIcon.isEmpty() ? QStringLiteral("package") : currentIcon;
    selectedColor_ = currentColor;
    auto* root = new QVBoxLayout(this);
    root->setContentsMargins(24, 24, 24, 24);
    root->setSpacing(12);
    auto* header = new QHBoxLayout;
    header->addWidget(titleLabel(QStringLiteral("选择图标"), this), 1);
    preview_ = new QLabel(glyphForName(selectedIcon_), this);
    preview_->setObjectName(QStringLiteral("botIconPreview"));
    preview_->setAlignment(Qt::AlignCenter);
    preview_->setFixedSize(44, 44);
    header->addWidget(preview_);
    root->addLayout(header);
    auto* iconLabel = new QLabel(QStringLiteral("图标"), this);
    iconLabel->setObjectName(QStringLiteral("muted"));
    root->addWidget(iconLabel);
    auto* grid = new QGridLayout;
    grid->setSpacing(6);
    auto* group = new QButtonGroup(this);
    group->setExclusive(true);
    const QStringList icons{QStringLiteral("scissors"), QStringLiteral("bug"), QStringLiteral("bot"), QStringLiteral("shield"), QStringLiteral("globe"), QStringLiteral("server"), QStringLiteral("zap"),
                            QStringLiteral("star"), QStringLiteral("flame"), QStringLiteral("cloud"), QStringLiteral("box"), QStringLiteral("heart"), QStringLiteral("target"), QStringLiteral("anchor"),
                            QStringLiteral("compass"), QStringLiteral("crown"), QStringLiteral("diamond"), QStringLiteral("gem"), QStringLiteral("rocket"), QStringLiteral("swords"), QStringLiteral("eye"),
                            QStringLiteral("lock"), QStringLiteral("key"), QStringLiteral("cpu"), QStringLiteral("file-json"), QStringLiteral("package"), QStringLiteral("terminal"), QStringLiteral("wifi")};
    for (int i = 0; i < icons.size(); ++i) {
        auto* button = new QPushButton(glyphForName(icons.at(i)), this);
        button->setCheckable(true);
        button->setObjectName(QStringLiteral("botIconChoice"));
        button->setProperty("iconName", icons.at(i));
        button->setFixedSize(40, 40);
        button->setToolTip(icons.at(i));
        group->addButton(button, i);
        grid->addWidget(button, i / 7, i % 7);
        if (icons.at(i) == selectedIcon_) button->setChecked(true);
    }
    connect(group, &QButtonGroup::idClicked, this, [this, icons](int id) {
        selectedIcon_ = icons.value(id, QStringLiteral("package"));
        preview_->setText(glyphForName(selectedIcon_));
    });
    root->addLayout(grid);
    auto* colorLabel = new QLabel(QStringLiteral("颜色"), this);
    colorLabel->setObjectName(QStringLiteral("muted"));
    root->addWidget(colorLabel);
    auto* colors = new QHBoxLayout;
    colors->setSpacing(8);
    auto* colorGroup = new QButtonGroup(this);
    colorGroup->setExclusive(true);
    const QList<quint32> colorValues{0xFFEF4444, 0xFFF97316, 0xFFF59E0B, 0xFF22C55E,
                                     0xFF3B82F6, 0xFF6366F1, 0xFFEC4899, 0xFFFFFFFF};
    auto updatePreview = [this, group]() {
        const QColor color = QColor::fromRgba(selectedColor_);
        preview_->setStyleSheet(QStringLiteral("color:%1;background:rgba(%2,%3,%4,40);border:1px solid rgba(%2,%3,%4,90);border-radius:10px;font-size:22px;")
                                    .arg(color.name()).arg(color.red()).arg(color.green()).arg(color.blue()));
        for (QAbstractButton* button : group->buttons()) {
            const bool selected = button->isChecked();
            button->setStyleSheet(QStringLiteral("font-size:18px;color:%1;background:%2;border:%3;border-radius:8px;")
                                      .arg(selected ? color.name() : QStringLiteral("rgba(255,255,255,105)"),
                                           selected ? QStringLiteral("rgba(%1,%2,%3,38)").arg(color.red()).arg(color.green()).arg(color.blue()) : QStringLiteral("rgba(255,255,255,12)"),
                                           selected ? QStringLiteral("1px solid %1").arg(color.name()) : QStringLiteral("1px solid rgba(255,255,255,25)")));
        }
    };
    for (int i = 0; i < colorValues.size(); ++i) {
        const quint32 value = colorValues.at(i);
        const QColor color = QColor::fromRgba(value);
        auto* button = new QPushButton(this);
        button->setCheckable(true);
        button->setFixedSize(32, 32);
        button->setProperty("colorValue", QVariant::fromValue(value));
        button->setStyleSheet(QStringLiteral("background:%1;border-radius:16px;border:%2;")
                                  .arg(color.name(), value == selectedColor_ ? QStringLiteral("3px solid white") : QStringLiteral("0")));
        colorGroup->addButton(button, i);
        colors->addWidget(button);
        if (value == selectedColor_) button->setChecked(true);
    }
    colors->addStretch();
    connect(colorGroup, &QButtonGroup::idClicked, this, [this, colorValues, colorGroup, updatePreview](int id) {
        selectedColor_ = colorValues.value(id, 0xFF6366F1);
        for (QAbstractButton* button : colorGroup->buttons()) {
            const QColor color = QColor::fromRgba(button->property("colorValue").toUInt());
            button->setStyleSheet(QStringLiteral("background:%1;border-radius:16px;border:%2;")
                                      .arg(color.name(), button->isChecked() ? QStringLiteral("3px solid white") : QStringLiteral("0")));
        }
        updatePreview();
    });
    connect(group, &QButtonGroup::idClicked, this, [updatePreview](int) { updatePreview(); });
    root->addLayout(colors);
    updatePreview();
    root->addSpacing(6);
    auto* buttons = new QDialogButtonBox(QDialogButtonBox::Cancel | QDialogButtonBox::Ok, this);
    buttons->button(QDialogButtonBox::Cancel)->setText(QStringLiteral("取消"));
    buttons->button(QDialogButtonBox::Ok)->setText(QStringLiteral("确认"));
    buttons->button(QDialogButtonBox::Ok)->setObjectName(QStringLiteral("primaryButton"));
    connect(buttons, &QDialogButtonBox::rejected, this, &QDialog::reject);
    connect(buttons, &QDialogButtonBox::accepted, this, &QDialog::accept);
    root->addWidget(buttons);
}

QString BotIconPickerDialog::selectedIcon() const { return selectedIcon_; }
quint32 BotIconPickerDialog::selectedColor() const { return selectedColor_; }
