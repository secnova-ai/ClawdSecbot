#pragma once

#include <QColor>
#include <QList>
#include <QString>
#include <QWidget>

class TrendChartWidget final : public QWidget {
    Q_OBJECT
public:
    enum class Mode { Line, Bars };

    TrendChartWidget(const QString& title, const QColor& color, Mode mode, QWidget* parent = nullptr);
    void setValues(const QList<int>& values);

protected:
    void paintEvent(QPaintEvent* event) override;

private:
    QString title_;
    QColor color_;
    Mode mode_;
    QList<int> values_;
};
