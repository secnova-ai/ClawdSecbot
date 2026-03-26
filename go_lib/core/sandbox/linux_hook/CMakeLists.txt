cmake_minimum_required(VERSION 3.10)
project(botsec_sandbox_preload C)

set(CMAKE_C_STANDARD 11)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

add_library(sandbox_preload SHARED preload.c)

set_target_properties(sandbox_preload PROPERTIES
    OUTPUT_NAME "sandbox_preload"
)

target_link_libraries(sandbox_preload dl)
