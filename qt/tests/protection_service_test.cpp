#include "bridge/GoBridge.h"
#include "service/ProtectionService.h"

#include <QDir>
#include <QJsonArray>
#include <QJsonDocument>
#include <QTemporaryDir>
#include <QtTest>

class ProtectionServiceTest final : public QObject {
    Q_OBJECT

private slots:
    void enabledConfigurationWithoutProxyIsNotReportedAsActive() {
        QTemporaryDir temporary;
        QVERIFY(temporary.isValid());
        const QString home = temporary.filePath(QStringLiteral("home"));
        const QString sandbox = temporary.filePath(QStringLiteral("sandbox"));
        const QString logs = temporary.filePath(QStringLiteral("logs"));
        QVERIFY(QDir{}.mkpath(home));
        QVERIFY(QDir{}.mkpath(sandbox));
        QVERIFY(QDir{}.mkpath(logs));

        AppConfig config;
        config.workspaceDir = temporary.path();
        config.homeDir = home;
        config.sandboxDir = sandbox;
        config.logDir = logs;
        config.libraryPath = QString::fromUtf8(GO_LIBRARY_PATH);
        config.appVersion = QStringLiteral(CLAWDSECBOT_VERSION);

        GoBridge bridge;
        QString error;
        QVERIFY2(bridge.initialize(config, &error), qPrintable(error));
        const QJsonObject payload{{QStringLiteral("asset_id"), QStringLiteral("openclaw:runtime-state-test")},
                                  {QStringLiteral("asset_name"), QStringLiteral("openclaw")},
                                  {QStringLiteral("enabled"), true}};
        const QJsonObject saved = bridge.call(
            "SaveProtectionConfigFFI", QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact)));
        QVERIFY(saved.value(QStringLiteral("success")).toBool());

        const QJsonObject summary = ProtectionService::activeProtectionSummary(bridge);
        QVERIFY(summary.value(QStringLiteral("success")).toBool());
        const QJsonObject data = summary.value(QStringLiteral("data")).toObject();
        QCOMPARE(data.value(QStringLiteral("enabled_count")).toInt(), 1);
        QCOMPARE(data.value(QStringLiteral("count")).toInt(), 0);
        QVERIFY(data.value(QStringLiteral("running_asset_ids")).toArray().isEmpty());
        bridge.shutdown();
    }
};

QTEST_MAIN(ProtectionServiceTest)
#include "protection_service_test.moc"
