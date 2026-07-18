#pragma once

#include <QHash>
#include <QJsonArray>
#include <QJsonObject>
#include <QPlainTextEdit>
#include <QScrollArea>
#include <QStringList>

class QVBoxLayout;

class RawLogView final : public QPlainTextEdit {
    Q_OBJECT

public:
    explicit RawLogView(QWidget* parent = nullptr);
    void appendLogLines(const QStringList& lines);
};

class AnalysisLogView final : public QScrollArea {
    Q_OBJECT

public:
    explicit AnalysisLogView(QWidget* parent = nullptr);

    void applySnapshots(const QJsonArray& snapshots);
    void clearRecords();
    int recordCount() const;

private:
    void rebuild();
    QWidget* createRecordCard(const QJsonObject& record);

    QWidget* content_ = nullptr;
    QVBoxLayout* layout_ = nullptr;
    QHash<QString, QJsonObject> records_;
    QStringList order_;
};
