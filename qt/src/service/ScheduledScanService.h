#pragma once

#include <QObject>

class QTimer;

class ScheduledScanService final : public QObject {
    Q_OBJECT
public:
    explicit ScheduledScanService(QObject* parent = nullptr);
    void configure(int intervalSeconds);
    int intervalSeconds() const;

signals:
    void scanRequested();

private:
    QTimer* timer_ = nullptr;
    int intervalSeconds_ = 0;
};
