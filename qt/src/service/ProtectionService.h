#pragma once

#include "domain/Models.h"

#include <QJsonObject>
#include <QStringList>

class GoBridge;

class ProtectionService final {
public:
    static QJsonObject stopAndRestore(GoBridge& bridge, const AssetModel& asset);
    static QStringList restoreAll(GoBridge& bridge);
};
