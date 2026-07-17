#include "service/ScanService.h"

#include "bridge/GoBridge.h"

#include <QDateTime>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>

namespace {
QJsonArray responseArray(const QJsonObject& response) {
    const QJsonValue data = response.value(QStringLiteral("data"));
    return data.isArray() ? data.toArray() : QJsonArray{};
}
}

ScanResultModel ScanService::loadLatest(GoBridge& bridge) {
    return ScanResultModel::fromResponse(bridge.call("GetLatestScanResult"));
}

ScanResultModel ScanService::runScan(GoBridge& bridge) {
    const QJsonObject assetResponse = bridge.call("ScanAssetsFFI");
    if (!assetResponse.value(QStringLiteral("success")).toBool()) return {};

    const QJsonObject hashesResponse = bridge.call("GetScannedSkillHashes");
    if (!hashesResponse.value(QStringLiteral("success")).toBool()) return {};
    const QString hashes = QString::fromUtf8(QJsonDocument(responseArray(hashesResponse)).toJson(QJsonDocument::Compact));

    const QJsonObject riskResponse = bridge.call("AssessRisksFFI", hashes);
    if (!riskResponse.value(QStringLiteral("success")).toBool()) return {};

    const QJsonArray assets = responseArray(assetResponse);
    const QJsonArray risks = responseArray(riskResponse);
    const QString createdAt = QDateTime::currentDateTimeUtc().toString(Qt::ISODateWithMs);
    const QJsonObject payload{{QStringLiteral("config_found"), !assets.isEmpty()},
                              {QStringLiteral("assets"), assets},
                              {QStringLiteral("risks"), risks},
                              {QStringLiteral("created_at"), createdAt}};
    const QJsonObject saveResponse = bridge.call(
        "SaveScanResult", QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact)));
    if (!saveResponse.value(QStringLiteral("success")).toBool()) return {};

    const QJsonObject combined{{QStringLiteral("success"), true},
                               {QStringLiteral("data"), payload}};
    return ScanResultModel::fromResponse(combined);
}
