#include "bridge/GoBridge.h"

#include <QDir>
#include <QJsonDocument>
#include <QTemporaryDir>
#include <QtTest>

class AppSettingsBridgeTest final : public QObject {
    Q_OBJECT

private slots:
    void stringSettingsRoundTripThroughGoFfi() {
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

        const QJsonObject intervalPayload{{QStringLiteral("key"), QStringLiteral("scheduled_scan_interval_seconds")},
                                          {QStringLiteral("value"), QStringLiteral("1800")}};
        const QJsonObject intervalSaved = bridge.call(
            "SaveAppSettingFFI", QString::fromUtf8(QJsonDocument(intervalPayload).toJson(QJsonDocument::Compact)));
        QVERIFY2(intervalSaved.value(QStringLiteral("success")).toBool(), qPrintable(intervalSaved.value(QStringLiteral("error")).toString()));
        QCOMPARE(bridge.call("GetAppSettingFFI", QStringLiteral("scheduled_scan_interval_seconds")).value(QStringLiteral("data")).toString(),
                 QStringLiteral("1800"));

        const QJsonObject boolPayload{{QStringLiteral("key"), QStringLiteral("api_server_enabled")},
                                      {QStringLiteral("value"), QStringLiteral("true")}};
        const QJsonObject boolSaved = bridge.call(
            "SaveAppSettingFFI", QString::fromUtf8(QJsonDocument(boolPayload).toJson(QJsonDocument::Compact)));
        QVERIFY2(boolSaved.value(QStringLiteral("success")).toBool(), qPrintable(boolSaved.value(QStringLiteral("error")).toString()));
        QCOMPARE(bridge.call("GetAppSettingFFI", QStringLiteral("api_server_enabled")).value(QStringLiteral("data")).toString(),
                 QStringLiteral("true"));
        bridge.shutdown();
    }
};

QTEST_MAIN(AppSettingsBridgeTest)
#include "app_settings_bridge_test.moc"
