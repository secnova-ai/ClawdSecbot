#include "service/ProtectionService.h"

#include "bridge/GoBridge.h"

#include <QJsonArray>
#include <QJsonDocument>

namespace {
QJsonObject setProtectionDisabled(GoBridge& bridge, const QString& assetId) {
    const QJsonObject payload{{QStringLiteral("asset_id"), assetId},
                              {QStringLiteral("enabled"), false}};
    return bridge.call("SetProtectionEnabledFFI",
                       QString::fromUtf8(QJsonDocument(payload).toJson(QJsonDocument::Compact)));
}
}

QJsonObject ProtectionService::activeProtectionSummary(GoBridge& bridge) {
    const QJsonObject configsResponse = bridge.call("GetEnabledProtectionConfigsFFI");
    if (!configsResponse.value(QStringLiteral("success")).toBool()) return configsResponse;

    QJsonArray runningAssetIds;
    const QJsonArray configs = configsResponse.value(QStringLiteral("data")).toArray();
    for (const QJsonValue& value : configs) {
        const QString assetId = value.toObject().value(QStringLiteral("asset_id")).toString().trimmed();
        if (assetId.isEmpty()) continue;
        const QJsonObject status = bridge.call("GetProtectionProxyStatusByAsset", assetId);
        if (status.contains(QStringLiteral("success")) && !status.value(QStringLiteral("success")).toBool()) {
            return status;
        }
        if (status.value(QStringLiteral("running")).toBool(status.value(QStringLiteral("is_running")).toBool())) {
            runningAssetIds.append(assetId);
        }
    }

    return {{QStringLiteral("success"), true},
            {QStringLiteral("data"), QJsonObject{{QStringLiteral("count"), runningAssetIds.size()},
                                                  {QStringLiteral("enabled_count"), configs.size()},
                                                  {QStringLiteral("running_asset_ids"), runningAssetIds}}}};
}

QJsonObject ProtectionService::stopAndRestore(GoBridge& bridge, const AssetModel& asset) {
    QJsonObject result = bridge.call("StopProtectionProxyByAsset", asset.id);
    if (!result.value(QStringLiteral("success")).toBool()) return result;
    result = bridge.call("RestoreToInitialConfigByAssetFFI", asset.id);
    if (!result.value(QStringLiteral("success")).toBool()) return result;
    return setProtectionDisabled(bridge, asset.id);
}

QStringList ProtectionService::restoreAll(GoBridge& bridge) {
    QStringList failures;
    const QJsonObject response = bridge.call("GetEnabledProtectionConfigsFFI");
    if (!response.value(QStringLiteral("success")).toBool()) {
        failures.append(response.value(QStringLiteral("error")).toString(QStringLiteral("Failed to load protection configurations.")));
        return failures;
    }

    for (const QJsonValue& value : response.value(QStringLiteral("data")).toArray()) {
        const QJsonObject config = value.toObject();
        const AssetModel asset{config.value(QStringLiteral("asset_id")).toString(),
                               {},
                               config.value(QStringLiteral("asset_name")).toString(),
                               {},
                               {},
                               {},
                               config};
        const QJsonObject result = stopAndRestore(bridge, asset);
        if (!result.value(QStringLiteral("success")).toBool()) {
            failures.append(QStringLiteral("%1: %2")
                                .arg(asset.name.isEmpty() ? asset.id : asset.name,
                                     result.value(QStringLiteral("error")).toString()));
        }
    }
    return failures;
}
