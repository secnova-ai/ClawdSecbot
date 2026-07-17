#pragma once

#include <QDialog>

class GoBridge;

class SkillScanResultsDialog final : public QDialog {
    Q_OBJECT
public:
    explicit SkillScanResultsDialog(GoBridge* bridge, QWidget* parent = nullptr);
};
