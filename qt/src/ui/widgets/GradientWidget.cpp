#include "ui/widgets/GradientWidget.h"

#include <QLinearGradient>
#include <QPainter>

GradientWidget::GradientWidget(QWidget* parent) : QWidget(parent) {
    setObjectName(QStringLiteral("gradientBackground"));
    setAutoFillBackground(false);
}

void GradientWidget::paintEvent(QPaintEvent* event) {
    Q_UNUSED(event)
    QPainter painter(this);
    QLinearGradient gradient(rect().topLeft(), rect().bottomRight());
    gradient.setColorAt(0.0, QColor(QStringLiteral("#0F0F23")));
    gradient.setColorAt(0.52, QColor(QStringLiteral("#1A1A2E")));
    gradient.setColorAt(1.0, QColor(QStringLiteral("#16213E")));
    painter.fillRect(rect(), gradient);
}
