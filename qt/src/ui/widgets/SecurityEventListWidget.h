#pragma once

#include <QJsonArray>
#include <QJsonObject>
#include <QListWidget>

class QDialog;

class SecurityEventListWidget final : public QListWidget {
    Q_OBJECT

public:
    explicit SecurityEventListWidget(QWidget* parent = nullptr);

    void setEvents(const QJsonArray& events);
    int eventCount() const;

signals:
    void eventActivated(const QJsonObject& event);
    void eventCountChanged(int count);
};

QDialog* showSecurityEventDetail(const QJsonObject& event, QWidget* parent);
