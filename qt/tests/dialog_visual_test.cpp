#include "ui/DialogChrome.h"
#include "ui/UiDialogs.h"
#include "ui/dialogs/AppStoreGuideDialog.h"
#include "ui/dialogs/BotIconPickerDialog.h"
#include "ui/dialogs/MitigationDialog.h"
#include "ui/dialogs/OnboardingDialog.h"
#include "ui/dialogs/ProtectionConfigDialog.h"
#include "ui/dialogs/SettingsDialog.h"
#include "ui/dialogs/SkillScanDialog.h"
#include "ui/dialogs/SkillScanResultsDialog.h"
#include "ui/widgets/SecurityEventListWidget.h"

#include <QApplication>
#include <QDialog>
#include <QDir>
#include <QFile>
#include <QJsonArray>
#include <QLabel>
#include <QPainter>
#include <QPushButton>
#include <QtTest>

class DialogVisualTest final : public QObject {
    Q_OBJECT

private slots:
    void initTestCase() {
        QFile theme(QStringLiteral(":/themes/light.qss"));
        QVERIFY(theme.open(QIODevice::ReadOnly));
        qApp->setStyleSheet(QString::fromUtf8(theme.readAll()));
    }

    void allDialogsUseSharedChrome() {
        const AssetModel asset{QStringLiteral("openclaw:test"), QStringLiteral("Openclaw"), QStringLiteral("Openclaw"),
                               QStringLiteral("service"), QStringLiteral("1.0"), QString(), QJsonObject{}};
        const RiskModel risk{QStringLiteral("demo-risk"), asset.id, asset.sourcePlugin, QStringLiteral("高风险配置"),
                             QStringLiteral("当前配置允许不受限制的高风险工具调用。"), QStringLiteral("high"),
                             QJsonObject{{QStringLiteral("type"), QStringLiteral("suggestion")},
                                         {QStringLiteral("suggestions"), QJsonArray{}}}, QJsonObject{}};
        QList<QDialog*> dialogs{
            new SettingsDialog(nullptr),
            new ProtectionConfigDialog(asset, nullptr),
            new SkillScanResultsDialog(nullptr),
            new OnboardingDialog(nullptr),
            new MitigationDialog(risk, nullptr),
            new SkillScanDialog(nullptr, QStringLiteral("Openclaw")),
            new BotIconPickerDialog(QStringLiteral("shield"), 0xFF6366F1),
            new AppStoreGuideDialog,
            showSecurityEventDetail(QJsonObject{{QStringLiteral("id"), QStringLiteral("event-demo")},
                                                {QStringLiteral("timestamp"), QStringLiteral("2026-07-18T10:20:31.000Z")},
                                                {QStringLiteral("event_type"), QStringLiteral("blocked")},
                                                {QStringLiteral("action_desc"), QStringLiteral("Prompt injection blocked")},
                                                {QStringLiteral("risk_type"), QStringLiteral("PROMPT_INJECTION_DIRECT")},
                                                {QStringLiteral("detail"), QStringLiteral("Matched semantic protection rule")},
                                                {QStringLiteral("source"), QStringLiteral("react_agent")}}, nullptr),
            UiDialogs::choose(nullptr, QStringLiteral("停止防护"),
                              QStringLiteral("停止 Openclaw 的防护并恢复 Bot 原始配置？"), DialogChrome::Tone::Warning,
                              {{QStringLiteral("cancel"), QStringLiteral("取消"), UiDialogs::ButtonStyle::Secondary, false},
                               {QStringLiteral("accept"), QStringLiteral("停止并恢复"), UiDialogs::ButtonStyle::Danger, true}},
                              [](const QString&) {}),
            UiDialogs::showProgress(nullptr, QStringLiteral("正在处理"), QStringLiteral("正在安全地完成后台操作，请稍候…")),
        };

        const QString screenshotDir = qEnvironmentVariable("DIALOG_SCREENSHOT_DIR");
        if (!screenshotDir.isEmpty()) QVERIFY(QDir{}.mkpath(screenshotDir));
        int index = 0;
        for (QDialog* dialog : dialogs) {
            QVERIFY(dialog != nullptr);
            dialog->setAttribute(Qt::WA_DeleteOnClose, false);
            dialog->show();
            QTRY_VERIFY(dialog->isVisible());
            QTest::qWait(170);
            QCOMPARE(dialog->objectName(), QStringLiteral("styledDialog"));
            QVERIFY(dialog->findChild<QWidget*>(QStringLiteral("dialogHeader")) != nullptr);
            QVERIFY(dialog->findChild<QLabel*>(QStringLiteral("dialogTitle")) != nullptr);
            QVERIFY(dialog->width() >= 420);
            QVERIFY(dialog->height() >= 180);
            if (dialog->windowTitle() == QStringLiteral("开启防护") || dialog->windowTitle() == QStringLiteral("选择图标")
                || dialog->windowTitle() == QStringLiteral("配置引导")) {
                QVERIFY(dialog->findChild<QPushButton*>(QStringLiteral("primaryButton")) != nullptr);
            }
            if (!screenshotDir.isEmpty()) {
                const QString name = QStringLiteral("dialog_%1_%2.png").arg(index++, 2, 10, QLatin1Char('0')).arg(dialog->windowTitle());
                const QPixmap popup = dialog->grab();
                QImage composed(popup.size(), QImage::Format_ARGB32_Premultiplied);
                composed.fill(QColor(QStringLiteral("#0F0F23")));
                QPainter painter(&composed);
                QPixmap flattened = popup;
                flattened.setDevicePixelRatio(1.0);
                painter.drawPixmap(0, 0, flattened);
                painter.end();
                QVERIFY(composed.save(QDir(screenshotDir).filePath(name)));
            }
            dialog->hide();
            delete dialog;
        }
    }
};

QTEST_MAIN(DialogVisualTest)
#include "dialog_visual_test.moc"
