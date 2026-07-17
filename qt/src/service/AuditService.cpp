#include "service/AuditService.h"

#include "bridge/GoBridge.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>

#include <algorithm>

namespace {
QString queryPayload(const AuditQuery& query, int limit, int offset) {
    const QJsonObject object{{QStringLiteral("limit"), limit},
                             {QStringLiteral("offset"), offset},
                             {QStringLiteral("risk_only"), query.riskOnly},
                             {QStringLiteral("search_query"), query.searchQuery},
                             {QStringLiteral("asset_id"), query.assetId}};
    return QString::fromUtf8(QJsonDocument(object).toJson(QJsonDocument::Compact));
}

QString countPayload(const AuditQuery& query) {
    const QJsonObject object{{QStringLiteral("risk_only"), query.riskOnly},
                             {QStringLiteral("search_query"), query.searchQuery},
                             {QStringLiteral("asset_id"), query.assetId}};
    return QString::fromUtf8(QJsonDocument(object).toJson(QJsonDocument::Compact));
}

AuditStatisticsModel statisticsFromResponse(const QJsonObject& response) {
    const QJsonObject data = response.value(QStringLiteral("data")).toObject();
    return {data.value(QStringLiteral("total")).toInt(),
            data.value(QStringLiteral("risk_count")).toInt(),
            data.value(QStringLiteral("blocked_count")).toInt(),
            data.value(QStringLiteral("allowed_count")).toInt()};
}
}

AuditPageResult AuditService::fetchPage(GoBridge& bridge, const AuditQuery& query) {
    AuditPageResult result;
    const int pageSize = qBound(1, query.pageSize, 500);
    const int offset = qMax(0, query.page) * pageSize;

    const QJsonObject countResponse = bridge.call("GetAuditLogCountFFI", countPayload(query));
    if (!countResponse.value(QStringLiteral("success")).toBool()) {
        result.error = countResponse.value(QStringLiteral("error")).toString();
        return result;
    }
    const int total = countResponse.value(QStringLiteral("data")).toInt();

    const QJsonObject pageResponse = bridge.call("GetAuditLogsFFI", queryPayload(query, pageSize, offset));
    if (!pageResponse.value(QStringLiteral("success")).toBool()) {
        result.error = pageResponse.value(QStringLiteral("error")).toString();
        return result;
    }
    result.logs = AuditLogModel::fromResponse(pageResponse);

    if (!query.riskOnly && query.searchQuery.trimmed().isEmpty()) {
        const QJsonObject statsFilter{{QStringLiteral("asset_id"), query.assetId}};
        const QJsonObject statsResponse = bridge.call(
            "GetAuditLogStatisticsByFilterFFI",
            QString::fromUtf8(QJsonDocument(statsFilter).toJson(QJsonDocument::Compact)));
        if (!statsResponse.value(QStringLiteral("success")).toBool()) {
            result.error = statsResponse.value(QStringLiteral("error")).toString();
            return result;
        }
        result.statistics = statisticsFromResponse(statsResponse);
    } else {
        QList<AuditLogModel> matchingLogs;
        constexpr int aggregatePageSize = 500;
        for (int aggregateOffset = 0; aggregateOffset < total; aggregateOffset += aggregatePageSize) {
            const QJsonObject response = bridge.call(
                "GetAuditLogsFFI", queryPayload(query, aggregatePageSize, aggregateOffset));
            if (!response.value(QStringLiteral("success")).toBool()) {
                result.error = response.value(QStringLiteral("error")).toString();
                return result;
            }
            const QList<AuditLogModel> page = AuditLogModel::fromResponse(response);
            matchingLogs.append(page);
            if (page.size() < aggregatePageSize) break;
        }
        result.statistics = aggregate(matchingLogs, total);
    }

    result.statistics.total = total;
    result.success = true;
    return result;
}

QList<SecurityEventModel> AuditService::fetchRelatedEvents(GoBridge& bridge, const AuditLogModel& log,
                                                            QString* error) {
    if (!log.requestId.trimmed().isEmpty()) {
        const QJsonObject response = bridge.call("GetSecurityEventsByRequestIDFFI", log.requestId);
        if (!response.value(QStringLiteral("success")).toBool()) {
            if (error != nullptr) *error = response.value(QStringLiteral("error")).toString();
            return {};
        }
        const QList<SecurityEventModel> exactMatches = SecurityEventModel::fromResponse(response);
        if (!exactMatches.isEmpty()) return exactMatches;
    }

    if (log.instructionChainId.trimmed().isEmpty()) return {};
    QList<SecurityEventModel> chainMatches;
    constexpr int pageSize = 500;
    for (int offset = 0;; offset += pageSize) {
        const QJsonObject query{{QStringLiteral("limit"), pageSize},
                                {QStringLiteral("offset"), offset},
                                {QStringLiteral("asset_id"), log.assetId}};
        const QJsonObject response = bridge.call(
            "GetSecurityEventsFFI", QString::fromUtf8(QJsonDocument(query).toJson(QJsonDocument::Compact)));
        if (!response.value(QStringLiteral("success")).toBool()) {
            if (error != nullptr) *error = response.value(QStringLiteral("error")).toString();
            return {};
        }
        const QList<SecurityEventModel> page = SecurityEventModel::fromResponse(response);
        for (const SecurityEventModel& event : page) {
            if (event.instructionChainId.trimmed() == log.instructionChainId.trimmed()) chainMatches.append(event);
        }
        if (page.size() < pageSize) break;
    }
    std::sort(chainMatches.begin(), chainMatches.end(), [](const SecurityEventModel& left, const SecurityEventModel& right) {
        return left.timestamp < right.timestamp;
    });
    return chainMatches;
}

AuditStatisticsModel AuditService::aggregate(const QList<AuditLogModel>& logs, int total) {
    AuditStatisticsModel result;
    result.total = total >= 0 ? total : logs.size();
    for (const AuditLogModel& log : logs) {
        const QString action = log.action.trimmed().toUpper();
        if (action == QStringLiteral("BLOCK") || action == QStringLiteral("HARD_BLOCK")) ++result.blockedCount;
        if (action == QStringLiteral("WARN")) ++result.riskCount;
        if (action == QStringLiteral("ALLOW")) ++result.allowedCount;
    }
    return result;
}
