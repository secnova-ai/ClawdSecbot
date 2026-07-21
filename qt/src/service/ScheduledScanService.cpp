#include "service/ScheduledScanService.h"

#include "common/AppLogger.h"

#include <QTimer>

ScheduledScanService::ScheduledScanService(QObject* parent) : QObject(parent), timer_(new QTimer(this)) {
    timer_->setTimerType(Qt::CoarseTimer);
    connect(timer_, &QTimer::timeout, this, &ScheduledScanService::scanRequested);
}

void ScheduledScanService::configure(int intervalSeconds) {
    timer_->stop();
    intervalSeconds_ = qMax(0, intervalSeconds);
    if (intervalSeconds_ == 0) {
        AppLogger::info(QStringLiteral("Scheduled scan disabled."));
        return;
    }
    timer_->start(std::chrono::seconds(intervalSeconds_));
    AppLogger::info(QStringLiteral("Scheduled scan enabled every %1 seconds.").arg(intervalSeconds_));
}

int ScheduledScanService::intervalSeconds() const { return intervalSeconds_; }
