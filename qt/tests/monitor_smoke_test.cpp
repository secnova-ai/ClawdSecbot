#include "ui/ProtectionMonitorWindow.h"

#include <QCheckBox>
#include <QLabel>
#include <QListWidget>
#include <QPlainTextEdit>
#include <QTabWidget>
#include <QtTest>

class MonitorSmokeTest final : public QObject {
    Q_OBJECT

private slots:
    void rendersAllInteractiveSections() {
        const AssetModel asset{QStringLiteral("openclaw:test"), QStringLiteral("Openclaw"), QStringLiteral("Openclaw"),
                               QStringLiteral("service"), QStringLiteral("1.0"), QString(), QJsonObject{}};
        ProtectionMonitorWindow window(asset, nullptr);
        window.show();
        QTRY_VERIFY(window.isVisible());

        QStringList labels;
        for (const QLabel* label : window.findChildren<QLabel*>()) labels.append(label->text());
        for (const QString& expected : {QStringLiteral("分析请求"), QStringLiteral("风险次数"), QStringLiteral("拦截次数"),
                                       QStringLiteral("Token 用量"), QStringLiteral("输入 Token"), QStringLiteral("输出 Token"),
                                       QStringLiteral("工具调用"), QStringLiteral("安全分析 Token")}) {
            QVERIFY2(labels.contains(expected), qPrintable(QStringLiteral("missing metric: %1").arg(expected)));
        }

        auto* auditOnly = window.findChild<QCheckBox*>();
        QVERIFY(auditOnly != nullptr);
        auditOnly->setChecked(true);
        QVERIFY(auditOnly->isChecked());
        QCOMPARE(window.findChildren<QPlainTextEdit*>().size(), 2);
        QVERIFY(window.findChild<QTabWidget*>() != nullptr);
        auto* events = window.findChild<QListWidget*>(QStringLiteral("securityEventList"));
        QVERIFY(events != nullptr);
        QCOMPARE(events->count(), 1);
    }
};

QTEST_MAIN(MonitorSmokeTest)
#include "monitor_smoke_test.moc"
