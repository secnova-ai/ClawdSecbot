file(GLOB_RECURSE UI_SOURCE_FILES
    "${SOURCE_DIR}/src/app/*.cpp"
    "${SOURCE_DIR}/src/ui/*.cpp"
)

foreach(SOURCE_FILE IN LISTS UI_SOURCE_FILES)
    file(READ "${SOURCE_FILE}" SOURCE_CONTENT)
    if(SOURCE_CONTENT MATCHES "QMessageBox::(warning|information|question|about)[(]")
        message(FATAL_ERROR "Blocking static QMessageBox call found in ${SOURCE_FILE}")
    endif()
    if(SOURCE_CONTENT MATCHES "[.]exec[(][)]")
        message(FATAL_ERROR "Nested modal event loop found in ${SOURCE_FILE}")
    endif()
    if(SOURCE_CONTENT MATCHES "QFileDialog::getSaveFileName")
        message(FATAL_ERROR "Blocking static file dialog found in ${SOURCE_FILE}")
    endif()
    if(SOURCE_CONTENT MATCHES "\n[ \t]*setEnabled[(]false[)]")
        message(FATAL_ERROR "Whole-window disabling found in ${SOURCE_FILE}")
    endif()
    if(SOURCE_CONTENT MATCHES "QtConcurrent::run[(][ \t\r\n]*\\[bridge")
        message(FATAL_ERROR "Go work must use the bridge-owned thread pool in ${SOURCE_FILE}")
    endif()
endforeach()

file(READ "${SOURCE_DIR}/src/bridge/GoBridge.cpp" BRIDGE_SOURCE)
if(BRIDGE_SOURCE MATCHES "globalInstance[(][)][^\n]*waitForDone")
    message(FATAL_ERROR "GoBridge shutdown must not wait for unrelated global thread-pool work")
endif()
