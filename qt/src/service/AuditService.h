#pragma once

#include "domain/Models.h"

#include <QString>

class GoBridge;

struct AuditQuery {
    int page = 0;
    int pageSize = 50;
    bool riskOnly = false;
    QString searchQuery;
    QString assetId;
};

struct AuditStatisticsModel {
    int total = 0;
    int riskCount = 0;
    int blockedCount = 0;
    int allowedCount = 0;
};

struct AuditPageResult {
    QList<AuditLogModel> logs;
    AuditStatisticsModel statistics;
    QString error;
    bool success = false;
};

class AuditService final {
public:
    static AuditPageResult fetchPage(GoBridge& bridge, const AuditQuery& query);
    static QList<SecurityEventModel> fetchRelatedEvents(GoBridge& bridge, const AuditLogModel& log,
                                                        QString* error = nullptr);

    static AuditStatisticsModel aggregate(const QList<AuditLogModel>& logs, int total = -1);
};
