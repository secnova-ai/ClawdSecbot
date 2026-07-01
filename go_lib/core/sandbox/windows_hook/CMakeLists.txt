cmake_minimum_required(VERSION 3.16)
project(sandbox_hook C)

set(CMAKE_C_STANDARD 11)

# Force static runtime library to avoid VCRUNTIME dependency
if(MSVC)
    set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
endif()

include(FetchContent)

# MinHook: lightweight inline hooking library
FetchContent_Declare(
    minhook
    GIT_REPOSITORY https://github.com/TsudaKageyu/minhook.git
    GIT_TAG        v1.3.3
)
FetchContent_MakeAvailable(minhook)

# cJSON: single-file JSON parser
FetchContent_Declare(
    cjson
    GIT_REPOSITORY https://github.com/DaveGamble/cJSON.git
    GIT_TAG        v1.7.18
)
# cJSON's released CMakeLists still declares an old minimum CMake version.
# Populate sources only and compile cJSON.c directly so newer CMake runners do
# not configure the cJSON subproject.
FetchContent_GetProperties(cjson)
if(NOT cjson_POPULATED)
    FetchContent_Populate(cjson)
endif()

add_library(sandbox_hook SHARED
    sandbox_hook.c
    hooks.c
    policy.c
    ${cjson_SOURCE_DIR}/cJSON.c
    ${minhook_SOURCE_DIR}/src/buffer.c
    ${minhook_SOURCE_DIR}/src/hook.c
    ${minhook_SOURCE_DIR}/src/trampoline.c
    ${minhook_SOURCE_DIR}/src/hde/hde32.c
    ${minhook_SOURCE_DIR}/src/hde/hde64.c
)

target_include_directories(sandbox_hook PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}
    ${minhook_SOURCE_DIR}/include
    ${minhook_SOURCE_DIR}/src
    ${minhook_SOURCE_DIR}/src/hde
    ${cjson_SOURCE_DIR}
)

target_link_libraries(sandbox_hook PRIVATE
    ws2_32
    shlwapi
    iphlpapi
)

set_target_properties(sandbox_hook PROPERTIES
    OUTPUT_NAME "sandbox_hook"
    PREFIX ""
)

install(TARGETS sandbox_hook RUNTIME DESTINATION bin)
