#include "bridge/GoBridge.h"

#include <QDir>
#include <QElapsedTimer>
#include <QTemporaryDir>
#include <QThread>
#include <QtConcurrent>
#include <QtTest>

#include <atomic>

class BridgeLifecycleTest final : public QObject {
    Q_OBJECT

private slots:
    void initializationStopsOnPathFailure() {
        QTemporaryDir temporary;
        QVERIFY(temporary.isValid());
        AppConfig config;
        config.workspaceDir = temporary.path();
        config.homeDir = temporary.path();
        config.sandboxDir = temporary.path();
        config.logDir = temporary.path();
        config.libraryPath = QString::fromUtf8(INIT_FAILURE_FIXTURE_PATH);
        config.appVersion = QStringLiteral("1.0.0");

        GoBridge bridge;
        QString error;
        QVERIFY(!bridge.initialize(config, &error));
        QCOMPARE(error, QStringLiteral("fixture path failure"));
        QVERIFY(!bridge.isReady());
    }

    void shutdownDoesNotWaitForUnrelatedGlobalPoolWork() {
        std::atomic_bool completed = false;
        std::atomic_bool bridgeWorkCompleted = false;
        QFuture<void> future = QtConcurrent::run([&completed]() {
            QThread::msleep(250);
            completed.store(true);
        });

        GoBridge bridge;
        QFuture<void> bridgeFuture = QtConcurrent::run(bridge.workerPool(), [&bridgeWorkCompleted]() {
            QThread::msleep(30);
            bridgeWorkCompleted.store(true);
        });
        QElapsedTimer timer;
        timer.start();
        bridge.shutdown();

        QVERIFY2(timer.elapsed() < 150, "Bridge shutdown waited for unrelated global thread-pool work");
        QVERIFY(bridgeWorkCompleted.load());
        QVERIFY(!completed.load());
        QVERIFY(!bridge.isReady());
        bridgeFuture.waitForFinished();
        future.waitForFinished();
    }
};

QTEST_MAIN(BridgeLifecycleTest)
#include "bridge_lifecycle_test.moc"
