#include "bridge/GoBridge.h"

#include <QThread>
#include <QtConcurrent>
#include <QtTest>

#include <atomic>

class BridgeLifecycleTest final : public QObject {
    Q_OBJECT

private slots:
    void shutdownWaitsForQueuedBridgeWork() {
        std::atomic_bool completed = false;
        [[maybe_unused]] const QFuture<void> future = QtConcurrent::run([&completed]() {
            QThread::msleep(30);
            completed.store(true);
        });

        GoBridge bridge;
        bridge.shutdown();

        QVERIFY(completed.load());
        QVERIFY(!bridge.isReady());
    }
};

QTEST_MAIN(BridgeLifecycleTest)
#include "bridge_lifecycle_test.moc"
