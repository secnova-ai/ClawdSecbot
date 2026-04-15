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
# Avoid MSVC /Za from cJSON custom flags; it conflicts with /std:c11.
set(ENABLE_CUSTOM_COMPILER_FLAGS OFF CACHE BOOL "" FORCE)
set(ENABLE_CJSON_TEST OFF CACHE BOOL "" FORCE)
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(BUILD_SHARED_AND_STATIC_LIBS OFF CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(cjson)

add_library(sandbox_hook SHARED
    sandbox_hook.c
    hooks.c
    policy.c
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
    cjson
    ws2_32
    shlwapi
    iphlpapi
)

set_target_properties(sandbox_hook PROPERTIES
    OUTPUT_NAME "sandbox_hook"
    PREFIX ""
)

install(TARGETS sandbox_hook RUNTIME DESTINATION bin)
