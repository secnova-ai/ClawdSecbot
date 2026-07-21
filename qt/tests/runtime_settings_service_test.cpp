#include "service/ApiServerState.h"
#include "service/ScheduledScanService.h"
#include "service/StartupService.h"

#include <QFile>
#include <QSignalSpy>
#include <QTemporaryDir>
#include <QtTest>

class RuntimeSettingsServiceTest final : public QObject {
    Q_OBJECT

private slots:
    void apiServerAlreadyInDesiredStateIsIdempotent() {
        const QJsonObject running{{QStringLiteral("success"), false},
                                  {QStringLiteral("error"), QStringLiteral("API server is already running")}};
        const QJsonObject stopped{{QStringLiteral("success"), false},
                                  {QStringLiteral("error"), QStringLiteral("API server is not running")}};
        QVERIFY(ApiServerState::isAlreadyInDesiredState(running, true));
        QVERIFY(ApiServerState::isAlreadyInDesiredState(stopped, false));
        QVERIFY(!ApiServerState::isAlreadyInDesiredState(running, false));
        QVERIFY(!ApiServerState::isAlreadyInDesiredState(stopped, true));
    }

    void scheduledScanCanBeEnabledAndDisabled() {
        ScheduledScanService service;
        QSignalSpy spy(&service, &ScheduledScanService::scanRequested);

        service.configure(1);
        QCOMPARE(service.intervalSeconds(), 1);
        QTRY_COMPARE_WITH_TIMEOUT(spy.count(), 1, 1600);

        service.configure(0);
        QCOMPARE(service.intervalSeconds(), 0);
        const int countAfterDisable = spy.count();
        QTest::qWait(1100);
        QCOMPARE(spy.count(), countAfterDisable);
    }

    void startupRegistrationRoundTripsInIsolatedDirectory() {
#if defined(Q_OS_WIN)
        QSKIP("Windows startup registration uses the user registry and is covered by platform CI.");
#else
        QTemporaryDir temporary;
        QVERIFY(temporary.isValid());
        const StartupServicePaths paths{
            QStringLiteral("/Applications/Clawd & Secbot.app/Contents/MacOS/ClawdSecbot"),
            temporary.filePath(QStringLiteral("LaunchAgents")),
            temporary.filePath(QStringLiteral("autostart")),
        };
        QString error;
        QVERIFY2(!StartupService::isEnabledForPaths(paths, &error), qPrintable(error));
        QVERIFY2(StartupService::setEnabledForPaths(true, paths, &error), qPrintable(error));
        QVERIFY(StartupService::isEnabledForPaths(paths, &error));

#if defined(Q_OS_MACOS)
        QFile registration(temporary.filePath(QStringLiteral("LaunchAgents/com.bot.secnova.clawdsecbot.plist")));
        QVERIFY(registration.open(QIODevice::ReadOnly));
        const QByteArray content = registration.readAll();
        QVERIFY(content.contains("Clawd &amp; Secbot.app"));
#else
        QFile registration(temporary.filePath(QStringLiteral("autostart/clawdsecbot.desktop")));
        QVERIFY(registration.open(QIODevice::ReadOnly));
        QVERIFY(registration.readAll().contains("X-GNOME-Autostart-enabled=true"));
#endif

        QVERIFY2(StartupService::setEnabledForPaths(false, paths, &error), qPrintable(error));
        QVERIFY(!StartupService::isEnabledForPaths(paths, &error));
#endif
    }
};

QTEST_MAIN(RuntimeSettingsServiceTest)
#include "runtime_settings_service_test.moc"
