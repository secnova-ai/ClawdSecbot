#include <cstdlib>
#include <cstring>

namespace {
char* copyResult(const char* value) {
    const std::size_t size = std::strlen(value) + 1;
    auto* result = static_cast<char*>(std::malloc(size));
    if (result != nullptr) std::memcpy(result, value, size);
    return result;
}
}

extern "C" char* InitPathsWithConfigFFI(const char*, const char*, const char*) {
    return copyResult("{\"success\":true}");
}

extern "C" char* InitLoggingFFI(const char*) {
    return copyResult("{\"success\":true}");
}

extern "C" char* InitDatabase(const char*) {
    return copyResult("{\"success\":true}");
}

extern "C" char* CloseDatabase() {
    return copyResult("{\"success\":true}");
}

extern "C" char* GetProtectionProxyStatusByAsset(const char*) {
    return copyResult("{\"success\":false,\"error\":\"status unavailable\"}");
}

extern "C" char* GetApiStatisticsFFI(const char*) {
    return copyResult("{\"success\":false,\"error\":\"metrics unavailable\"}");
}

extern "C" char* GetProtectionProxyLogs(const char*) {
    return copyResult("{\"error\":\"logs unavailable\"}");
}

extern "C" char* GetSecurityEventsFFI(const char*) {
    return copyResult("{\"success\":false,\"error\":\"events unavailable\"}");
}

extern "C" char* GetAllSkillScansFFI() {
    return copyResult(
        "{\"success\":true,\"data\":[{\"skill_name\":\"filesystem-audit\",\"skill_path\":\"/tmp/skills/filesystem-audit\","
        "\"scanned_at\":\"2026-07-18T10:20:30Z\",\"trusted\":true,\"safe\":false,\"issues\":[]}]}");
}

extern "C" void FreeString(char* value) {
    std::free(value);
}
