#pragma once

#include <QWidget>

class GradientWidget : public QWidget {
    Q_OBJECT
public:
    explicit GradientWidget(QWidget* parent = nullptr);
protected:
    void paintEvent(QPaintEvent* event) override;
};
