#include "ui/widgets/AuditTimelineWidget.h"

#include <QFrame>
#include <QLabel>
#include <QtTest>

#include <algorithm>

class AuditTimelineTest final : public QObject {
    Q_OBJECT

private slots:
    void mergesMessagesAndToolsIntoReplayOrder() {
        AuditLogModel log;
        log.id = QStringLiteral("audit-1");
        log.requestId = QStringLiteral("request-1");
        log.durationMs = 1200;
        log.messages = {{0, QStringLiteral("system"), QStringLiteral("hidden")},
                        {1, QStringLiteral("user"), QStringLiteral("check")},
                        {2, QStringLiteral("assistant"), QStringLiteral("done")}};
        log.toolCalls = {{QStringLiteral("shell"), QStringLiteral(R"({"cmd":"pwd"})"), QStringLiteral("/tmp"), false}};

        AuditTimelineWidget widget;
        widget.setLog(log);
        widget.show();
        QTRY_VERIFY(widget.isVisible());

        QList<QFrame*> cards;
        for (QFrame* frame : widget.findChildren<QFrame*>()) {
            if (frame->property("timelineIndex").isValid()) cards.append(frame);
        }
        std::sort(cards.begin(), cards.end(), [](const QFrame* left, const QFrame* right) {
            return left->property("timelineIndex").toInt() < right->property("timelineIndex").toInt();
        });
        QCOMPARE(cards.size(), 4);
        QCOMPARE(cards.at(0)->property("timelineRole").toString(), QStringLiteral("User"));
        QCOMPARE(cards.at(1)->property("timelineRole").toString(), QStringLiteral("ToolCall"));
        QCOMPARE(cards.at(2)->property("timelineRole").toString(), QStringLiteral("ToolResult"));
        QCOMPARE(cards.at(3)->property("timelineRole").toString(), QStringLiteral("Assistant"));

        QStringList ticks;
        for (QLabel* label : widget.findChildren<QLabel*>(QStringLiteral("auditTimelineTick"))) ticks.append(label->text());
        QCOMPARE(ticks, QStringList({QStringLiteral("开始"), QStringLiteral("步骤 2"), QStringLiteral("步骤 3"), QStringLiteral("结束")}));
    }

    void showsPersistedSecurityEventTimestamps() {
        AuditLogModel log;
        log.id = QStringLiteral("audit-2");
        log.requestId = QStringLiteral("request-2");
        log.requestContent = QStringLiteral("check");
        const QList<SecurityEventModel> events{{QStringLiteral("event-1"),
                                                QStringLiteral("2026-07-17T09:10:11.123Z"),
                                                QStringLiteral("tool_policy"),
                                                QStringLiteral("Blocked command"),
                                                QStringLiteral("command"),
                                                QStringLiteral("dangerous command"),
                                                QStringLiteral("shepherd"),
                                                QStringLiteral("request-2")}};

        AuditTimelineWidget widget;
        widget.setLog(log, events);

        const QList<QLabel*> timestamps = widget.findChildren<QLabel*>(QStringLiteral("auditSecurityEventTime"));
        QCOMPARE(timestamps.size(), 1);
        QVERIFY(timestamps.first()->property("exactTimestamp").toBool());
        QVERIFY(timestamps.first()->text().endsWith(QStringLiteral(".123")));
    }
};

QTEST_MAIN(AuditTimelineTest)
#include "audit_timeline_test.moc"
