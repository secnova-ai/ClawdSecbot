#include "ui/widgets/TrendChartWidget.h"

#include <QPainter>
#include <QPainterPath>

#include <algorithm>

TrendChartWidget::TrendChartWidget(const QString& title, const QColor& color, const Mode mode, QWidget* parent)
    : QWidget(parent), title_(title), color_(color), mode_(mode) {
    setMinimumHeight(150);
}

void TrendChartWidget::setValues(const QList<int>& values) {
    values_ = values;
    update();
}

void TrendChartWidget::paintEvent(QPaintEvent*) {
    QPainter painter(this);
    painter.setRenderHint(QPainter::Antialiasing);
    const QRectF panel = rect().adjusted(1, 1, -1, -1);
    painter.setPen(QColor(255, 255, 255, 25));
    painter.setBrush(QColor(255, 255, 255, 12));
    painter.drawRoundedRect(panel, 12, 12);
    painter.setPen(QColor(255, 255, 255, 220));
    QFont titleFont = painter.font();
    titleFont.setBold(true);
    painter.setFont(titleFont);
    painter.drawText(QRectF(14, 10, width() - 28, 24), Qt::AlignLeft | Qt::AlignVCenter, title_);

    const QRectF plot = panel.adjusted(14, 42, -14, -14);
    painter.setPen(QColor(255, 255, 255, 16));
    for (int row = 0; row < 4; ++row) {
        const qreal y = plot.top() + plot.height() * row / 3.0;
        painter.drawLine(QPointF(plot.left(), y), QPointF(plot.right(), y));
    }
    if (values_.isEmpty()) {
        painter.setPen(QColor(255, 255, 255, 90));
        painter.drawText(plot, Qt::AlignCenter, QStringLiteral("暂无趋势数据"));
        return;
    }
    const int maximum = qMax(1, *std::max_element(values_.cbegin(), values_.cend()));
    painter.setPen(QPen(color_, 2.2, Qt::SolidLine, Qt::RoundCap, Qt::RoundJoin));
    painter.setBrush(color_);
    if (mode_ == Mode::Bars) {
        const qreal step = plot.width() / values_.size();
        const qreal barWidth = qMax<qreal>(3, step * 0.55);
        for (qsizetype index = 0; index < values_.size(); ++index) {
            const qreal height = plot.height() * values_.at(index) / maximum;
            painter.drawRoundedRect(QRectF(plot.left() + index * step + (step - barWidth) / 2,
                                           plot.bottom() - height, barWidth, height), 2, 2);
        }
        return;
    }
    QPainterPath path;
    for (qsizetype index = 0; index < values_.size(); ++index) {
        const qreal x = values_.size() == 1 ? plot.center().x() : plot.left() + plot.width() * index / (values_.size() - 1);
        const qreal y = plot.bottom() - plot.height() * values_.at(index) / maximum;
        if (index == 0) path.moveTo(x, y); else path.lineTo(x, y);
    }
    painter.drawPath(path);
}
