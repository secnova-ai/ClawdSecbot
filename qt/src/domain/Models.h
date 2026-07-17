#pragma once

#include <QJsonObject>
#include <QList>
#include <QString>

struct AssetModel {
    QString id;
    QString sourcePlugin;
    QString name;
    QString type;
    QString version;
    QString serviceName;
    QJsonObject raw;
};

struct RiskModel {
    QString id;
    QString assetId;
    QString sourcePlugin;
    QString title;
    QString description;
    QString level;
    QJsonObject mitigation;
    QJsonObject raw;
};

struct ScanResultModel {
    QList<AssetModel> assets;
    QList<RiskModel> risks;
    QString scannedAt;
    bool valid = false;

    static ScanResultModel fromResponse(const QJsonObject& response);
};

struct AuditMessageModel {
    int index = 0;
    QString role;
    QString content;
};

struct AuditToolCallModel {
    QString name;
    QString arguments;
    QString result;
    bool sensitive = false;
};

struct SecurityEventModel {
    QString id;
    QString timestamp;
    QString eventType;
    QString actionDescription;
    QString riskType;
    QString detail;
    QString source;
    QString requestId;
    QString assetId;
    QString instructionChainId;

    static QList<SecurityEventModel> fromResponse(const QJsonObject& response);
};

struct AuditLogModel {
    QString id;
    QString timestamp;
    QString requestId;
    QString instructionChainId;
    QString assetName;
    QString assetId;
    QString model;
    QString requestContent;
    QString outputContent;
    QString action;
    QString riskLevel;
    QString riskReason;
    bool hasRisk = false;
    int durationMs = 0;
    int totalTokens = 0;
    QList<AuditMessageModel> messages;
    QList<AuditToolCallModel> toolCalls;
    QJsonObject raw;

    static QList<AuditLogModel> fromResponse(const QJsonObject& response, int* total = nullptr);
};
