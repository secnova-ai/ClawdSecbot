#pragma once

#include "domain/Models.h"

#include <QDialog>

class GoBridge;

class MitigationDialog final : public QDialog {
    Q_OBJECT
public:
    MitigationDialog(const RiskModel& risk, GoBridge* bridge, QWidget* parent = nullptr);
};
