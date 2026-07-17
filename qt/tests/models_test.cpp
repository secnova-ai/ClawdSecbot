#include "domain/Models.h"

#include <QJsonArray>
#include <QJsonObject>
#include <QTest>

class ModelsTest final : public QObject {
    Q_OBJECT

private slots:
    void parsesScanResponse() {
        const QJsonObject asset{{QStringLiteral("id"), QStringLiteral("openclaw:123")},
                                {QStringLiteral("name"), QStringLiteral("Openclaw")},
                                {QStringLiteral("version"), QStringLiteral("1.0")}};
        const QJsonObject risk{{QStringLiteral("id"), QStringLiteral("unsafe")},
                               {QStringLiteral("asset_id"), QStringLiteral("openclaw:123")},
                               {QStringLiteral("title"), QStringLiteral("Unsafe setting")},
                               {QStringLiteral("level"), QStringLiteral("high")}};
        const QJsonObject response{{QStringLiteral("success"), true},
                                   {QStringLiteral("data"), QJsonObject{{QStringLiteral("assets"), QJsonArray{asset}},
                                                                         {QStringLiteral("risks"), QJsonArray{risk}}}}};
        const ScanResultModel result = ScanResultModel::fromResponse(response);
        QVERIFY(result.valid);
        QCOMPARE(result.assets.size(), 1);
        QCOMPARE(result.risks.size(), 1);
        QCOMPARE(result.risks.first().assetId, QStringLiteral("openclaw:123"));
    }

    void rejectsFailedScanResponse() {
        const ScanResultModel result = ScanResultModel::fromResponse({{QStringLiteral("success"), false}});
        QVERIFY(!result.valid);
    }

    void parsesAuditTimelinePayloads() {
        const QJsonObject log{{QStringLiteral("id"), QStringLiteral("audit-1")},
                              {QStringLiteral("timestamp"), QStringLiteral("2026-07-17T09:00:00Z")},
                              {QStringLiteral("request_id"), QStringLiteral("request-1")},
                              {QStringLiteral("request_content"), QStringLiteral("执行检查")},
                              {QStringLiteral("output_content"), QStringLiteral("检查完成")},
                              {QStringLiteral("duration_ms"), 1200},
                              {QStringLiteral("total_tokens"), 42},
                              {QStringLiteral("messages"), QStringLiteral(R"([{"index":2,"role":"assistant","content":"done"},{"index":0,"role":"system","content":"hidden"},{"index":1,"role":"user","content":"check"}])")},
                              {QStringLiteral("tool_calls"), QJsonArray{QJsonObject{{QStringLiteral("name"), QStringLiteral("shell" )},
                                                                                   {QStringLiteral("arguments"), QStringLiteral(R"({"cmd":"pwd"})")},
                                                                                   {QStringLiteral("result"), QStringLiteral("/tmp")},
                                                                                   {QStringLiteral("is_sensitive"), true}}}}};
        const QJsonObject response{{QStringLiteral("data"), QJsonObject{{QStringLiteral("logs"), QJsonArray{log}},
                                                                         {QStringLiteral("total"), 1}}}};
        int total = 0;
        const QList<AuditLogModel> logs = AuditLogModel::fromResponse(response, &total);
        QCOMPARE(total, 1);
        QCOMPARE(logs.size(), 1);
        QCOMPARE(logs.first().requestId, QStringLiteral("request-1"));
        QCOMPARE(logs.first().messages.size(), 3);
        QCOMPARE(logs.first().toolCalls.size(), 1);
        QCOMPARE(logs.first().toolCalls.first().name, QStringLiteral("shell"));
        QVERIFY(logs.first().toolCalls.first().sensitive);
        QCOMPARE(logs.first().durationMs, 1200);
        QCOMPARE(logs.first().totalTokens, 42);
    }
};

QTEST_MAIN(ModelsTest)
#include "models_test.moc"
