#pragma once

#include "domain/Models.h"

class GoBridge;

class ScanService final {
public:
    static ScanResultModel loadLatest(GoBridge& bridge);
    static ScanResultModel runScan(GoBridge& bridge);
};
