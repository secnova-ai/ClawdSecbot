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
    return copyResult("{\"success\":false,\"error\":\"fixture path failure\"}");
}

extern "C" char* InitLoggingFFI(const char*) {
    return copyResult("{\"success\":true}");
}

extern "C" char* InitDatabase(const char*) {
    return copyResult("{\"success\":true}");
}

extern "C" void FreeString(char* value) {
    std::free(value);
}
