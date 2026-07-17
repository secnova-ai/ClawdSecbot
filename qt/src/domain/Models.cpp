#include "domain/Models.h"

#include <QJsonArray>
#include <QJsonDocument>

namespace {
QJsonObject responseData(const QJsonObject& response) {
    const QJsonValue data = response.value(QStringLiteral("data"));
    return data.isObject() ? data.toObject() : response;
}

QJsonArray arrayFromValue(const QJsonValue& value) {
    if (value.isArray()) return value.toArray();
    if (!value.isString()) return {};
    const QJsonDocument document = QJsonDocument::fromJson(value.toString().toUtf8());
    return document.isArray() ? document.array() : QJsonArray{};
}
}

ScanResultModel ScanResultModel::fromResponse(const QJsonObject& response) {
    ScanResultModel result;
    if (!response.value(QStringLiteral("success")).toBool()) return result;
    const QJsonObject data = responseData(response);
    for (const QJsonValue& value : data.value(QStringLiteral("assets")).toArray()) {
        const QJsonObject object = value.toObject();
        result.assets.append({object.value(QStringLiteral("id")).toString(),
                              object.value(QStringLiteral("source_plugin")).toString(object.value(QStringLiteral("plugin")).toString()),
                              object.value(QStringLiteral("name")).toString(),
                              object.value(QStringLiteral("type")).toString(),
                              object.value(QStringLiteral("version")).toString(),
                              object.value(QStringLiteral("service_name")).toString(), object});
    }
    QJsonArray risks = data.value(QStringLiteral("risks")).toArray();
    for (const QJsonValue& value : risks) {
        const QJsonObject object = value.toObject();
        const QJsonObject args = object.value(QStringLiteral("args")).toObject();
        result.risks.append({object.value(QStringLiteral("id")).toString(),
                             object.value(QStringLiteral("asset_id")).toString(args.value(QStringLiteral("asset_id")).toString()),
                             object.value(QStringLiteral("source_plugin")).toString(args.value(QStringLiteral("source_plugin")).toString()),
                             object.value(QStringLiteral("title")).toString(),
                             object.value(QStringLiteral("description")).toString(),
                             object.value(QStringLiteral("level")).toVariant().toString(),
                             object.value(QStringLiteral("mitigation")).toObject(), object});
    }
    result.scannedAt = data.value(QStringLiteral("created_at")).toString();
    result.valid = true;
    return result;
}

QList<AuditLogModel> AuditLogModel::fromResponse(const QJsonObject& response, int* total) {
    QList<AuditLogModel> result;
    if (response.contains(QStringLiteral("success")) && !response.value(QStringLiteral("success")).toBool()) {
        if (total != nullptr) *total = 0;
        return result;
    }
    const QJsonValue rawData = response.value(QStringLiteral("data"));
    const QJsonObject data = rawData.isObject() ? rawData.toObject() : response;
    QJsonArray logs = rawData.isArray() ? rawData.toArray() : data.value(QStringLiteral("logs")).toArray();
    if (logs.isEmpty() && response.value(QStringLiteral("logs")).isArray()) logs = response.value(QStringLiteral("logs")).toArray();
    if (total != nullptr) *total = data.value(QStringLiteral("total")).toInt(logs.size());
    for (const QJsonValue& value : logs) {
        const QJsonObject object = value.toObject();
        AuditLogModel log;
        log.id = object.value(QStringLiteral("id")).toString();
        log.timestamp = object.value(QStringLiteral("timestamp")).toString();
        log.requestId = object.value(QStringLiteral("request_id")).toString();
        log.instructionChainId = object.value(QStringLiteral("instruction_chain_id")).toString();
        log.assetName = object.value(QStringLiteral("asset_name")).toString();
        log.assetId = object.value(QStringLiteral("asset_id")).toString();
        log.model = object.value(QStringLiteral("model")).toString();
        log.requestContent = object.value(QStringLiteral("request_content")).toString();
        log.outputContent = object.value(QStringLiteral("output_content")).toString();
        log.action = object.value(QStringLiteral("action")).toString(QStringLiteral("ALLOW"));
        log.riskLevel = object.value(QStringLiteral("risk_level")).toString();
        log.riskReason = object.value(QStringLiteral("risk_reason")).toString();
        log.hasRisk = object.value(QStringLiteral("has_risk")).toBool();
        log.durationMs = object.value(QStringLiteral("duration_ms")).toInt();
        log.totalTokens = object.value(QStringLiteral("total_tokens")).toInt(object.value(QStringLiteral("token_count")).toInt());

        for (const QJsonValue& messageValue : arrayFromValue(object.value(QStringLiteral("messages")))) {
            const QJsonObject message = messageValue.toObject();
            log.messages.append({message.value(QStringLiteral("index")).toInt(),
                                 message.value(QStringLiteral("role")).toString(),
                                 message.value(QStringLiteral("content")).toString()});
        }
        for (const QJsonValue& toolValue : arrayFromValue(object.value(QStringLiteral("tool_calls")))) {
            const QJsonObject tool = toolValue.toObject();
            log.toolCalls.append({tool.value(QStringLiteral("name")).toString(),
                                  tool.value(QStringLiteral("arguments")).toVariant().toString(),
                                  tool.value(QStringLiteral("result")).toVariant().toString(),
                                  tool.value(QStringLiteral("is_sensitive")).toBool()});
        }
        log.raw = object;
        result.append(log);
    }
    return result;
}

QList<SecurityEventModel> SecurityEventModel::fromResponse(const QJsonObject& response) {
    QList<SecurityEventModel> result;
    if (response.contains(QStringLiteral("success")) && !response.value(QStringLiteral("success")).toBool()) return result;
    const QJsonValue data = response.value(QStringLiteral("data"));
    const QJsonArray events = data.isArray() ? data.toArray() : response.value(QStringLiteral("events")).toArray();
    for (const QJsonValue& value : events) {
        const QJsonObject object = value.toObject();
        result.append({object.value(QStringLiteral("id")).toString(),
                       object.value(QStringLiteral("timestamp")).toString(),
                       object.value(QStringLiteral("event_type")).toString(),
                       object.value(QStringLiteral("action_desc")).toString(),
                       object.value(QStringLiteral("risk_type")).toString(),
                       object.value(QStringLiteral("detail")).toString(),
                       object.value(QStringLiteral("source")).toString(),
                       object.value(QStringLiteral("request_id")).toString(),
                       object.value(QStringLiteral("asset_id")).toString(),
                       object.value(QStringLiteral("instruction_chain_id")).toString()});
    }
    return result;
}
