#pragma once

#include "domain/Models.h"

#include <QWidget>

class QVBoxLayout;

class AuditTimelineWidget final : public QWidget {
public:
    explicit AuditTimelineWidget(QWidget* parent = nullptr);

    void setLog(const AuditLogModel& log, const QList<SecurityEventModel>& securityEvents = {});

private:
    void clearContent();

    QVBoxLayout* contentLayout_ = nullptr;
};
