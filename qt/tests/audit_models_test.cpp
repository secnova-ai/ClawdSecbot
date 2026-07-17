#include "domain/Models.h"
#include "service/AuditService.h"

#include <QJsonArray>
#include <QJsonObject>
#include <QtTest>

class AuditModelsTest final : public QObject {
    Q_OBJECT

private slots:
    void parsesSecurityEvents() {
        const QJsonObject response{
            {QStringLiteral("success"), true},
            {QStringLiteral("data"), QJsonArray{QJsonObject{
                {QStringLiteral("id"), QStringLiteral("event-1")},
                {QStringLiteral("timestamp"), QStringLiteral("2026-07-17T09:00:01Z")},
                {QStringLiteral("event_type"), QStringLiteral("tool_policy")},
                {QStringLiteral("action_desc"), QStringLiteral("Blocked shell command")},
                {QStringLiteral("risk_type"), QStringLiteral("command")},
                {QStringLiteral("detail"), QStringLiteral("rm command")},
                {QStringLiteral("source"), QStringLiteral("shepherd")},
                {QStringLiteral("request_id"), QStringLiteral("request-1")},
                {QStringLiteral("asset_id"), QStringLiteral("asset-1")},
                {QStringLiteral("instruction_chain_id"), QStringLiteral("chain-1")},
            }}}};

        const QList<SecurityEventModel> events = SecurityEventModel::fromResponse(response);
        QCOMPARE(events.size(), 1);
        QCOMPARE(events.first().id, QStringLiteral("event-1"));
        QCOMPARE(events.first().requestId, QStringLiteral("request-1"));
        QCOMPARE(events.first().assetId, QStringLiteral("asset-1"));
        QCOMPARE(events.first().instructionChainId, QStringLiteral("chain-1"));
    }

    void aggregatesAuditActions() {
        QList<AuditLogModel> logs(4);
        logs[0].action = QStringLiteral("ALLOW");
        logs[1].action = QStringLiteral("WARN");
        logs[2].action = QStringLiteral("BLOCK");
        logs[3].action = QStringLiteral("HARD_BLOCK");

        const AuditStatisticsModel statistics = AuditService::aggregate(logs);
        QCOMPARE(statistics.total, 4);
        QCOMPARE(statistics.allowedCount, 1);
        QCOMPARE(statistics.riskCount, 1);
        QCOMPARE(statistics.blockedCount, 2);
    }
};

QTEST_MAIN(AuditModelsTest)
#include "audit_models_test.moc"
