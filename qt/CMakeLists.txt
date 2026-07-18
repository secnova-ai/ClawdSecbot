cmake_minimum_required(VERSION 3.21)

get_filename_component(REPOSITORY_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/.." ABSOLUTE)
file(STRINGS "${REPOSITORY_ROOT}/pubspec.yaml" APP_VERSION_LINE REGEX "^version:[ \t]*[0-9]+\\.[0-9]+\\.[0-9]+")
string(REGEX MATCH "[0-9]+\\.[0-9]+\\.[0-9]+" APP_VERSION "${APP_VERSION_LINE}")
if(NOT APP_VERSION)
    message(FATAL_ERROR "Unable to read the application version from pubspec.yaml")
endif()

project(clawdsecbot_qt VERSION "${APP_VERSION}" LANGUAGES CXX)

set(APP_DISPLAY_NAME "ClawdSecbot")

if(APPLE)
    set(APP_BUNDLE_IDENTIFIER "com.bot.secnova.clawdsecbot.qt" CACHE STRING "macOS bundle identifier")
    set(APP_MACOS_MINIMUM_VERSION "14.0" CACHE STRING "Minimum supported macOS version")
    set(CMAKE_OSX_DEPLOYMENT_TARGET "${APP_MACOS_MINIMUM_VERSION}" CACHE STRING "Minimum macOS deployment version" FORCE)
endif()

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTOUIC ON)
set(CMAKE_AUTORCC ON)

find_package(Qt6 6.4 REQUIRED COMPONENTS Core Gui Widgets Concurrent Test)

if(APPLE)
    set(GO_LIBRARY_NAME "botsec.dylib")
elseif(WIN32)
    set(GO_LIBRARY_NAME "botsec.dll")
else()
    set(GO_LIBRARY_NAME "botsec.so")
endif()

set(GO_LIBRARY_SOURCE "${REPOSITORY_ROOT}/plugins/${GO_LIBRARY_NAME}")
find_program(GO_EXECUTABLE go REQUIRED)
add_custom_target(botsec_go_library
    COMMAND "${CMAKE_COMMAND}" -E make_directory "${REPOSITORY_ROOT}/plugins"
    COMMAND "${GO_EXECUTABLE}" build -buildvcs=false -buildmode=c-shared -o "${GO_LIBRARY_SOURCE}" .
    BYPRODUCTS "${GO_LIBRARY_SOURCE}"
    WORKING_DIRECTORY "${REPOSITORY_ROOT}/go_lib"
    COMMENT "Building the shared Go business library"
    VERBATIM
)

set(CORE_SOURCES
    src/common/AppLogger.h
    src/common/AppLogger.cpp
    src/core/AppConfig.h
    src/core/AppConfig.cpp
    src/domain/Models.h
    src/domain/Models.cpp
    src/bridge/GoBridge.h
    src/bridge/GoBridge.cpp
    src/service/AuditService.h
    src/service/AuditService.cpp
    src/service/ProtectionService.h
    src/service/ProtectionService.cpp
    src/service/ScanService.h
    src/service/ScanService.cpp
)

set(UI_SOURCES
    src/app/AppController.h
    src/app/AppController.cpp
    src/ui/MainWindow.h
    src/ui/MainWindow.cpp
    src/ui/AuditLogWindow.h
    src/ui/AuditLogWindow.cpp
    src/ui/ProtectionMonitorWindow.h
    src/ui/ProtectionMonitorWindow.cpp
    src/ui/Dialogs.h
    src/ui/Dialogs.cpp
    src/ui/dialogs/SettingsDialog.h
    src/ui/dialogs/ProtectionConfigDialog.h
    src/ui/dialogs/SkillScanDialog.h
    src/ui/dialogs/SkillScanResultsDialog.h
    src/ui/dialogs/OnboardingDialog.h
    src/ui/dialogs/MitigationDialog.h
    src/ui/dialogs/BotIconPickerDialog.h
    src/ui/dialogs/BotIconPickerDialog.cpp
    src/ui/dialogs/AppStoreGuideDialog.h
    src/ui/AppStoreGuideDialog.cpp
    src/ui/SkillScanDialog.cpp
    src/ui/widgets/GradientWidget.h
    src/ui/widgets/GradientWidget.cpp
    src/ui/widgets/AssetCardWidget.h
    src/ui/widgets/AssetCardWidget.cpp
    src/ui/widgets/AuditTimelineWidget.h
    src/ui/widgets/AuditTimelineWidget.cpp
    src/ui/widgets/TrendChartWidget.h
    src/ui/widgets/TrendChartWidget.cpp
    src/ui/widgets/AnalysisLogView.h
    src/ui/widgets/AnalysisLogView.cpp
    src/ui/widgets/SecurityEventListWidget.h
    src/ui/widgets/SecurityEventListWidget.cpp
    src/ui/UiTheme.h
    src/ui/UiTheme.cpp
    src/ui/UiDialogs.h
    src/ui/UiDialogs.cpp
    src/ui/DialogChrome.h
    src/ui/DialogChrome.cpp
    src/resources.qrc
)

add_library(clawdsecbot_qt_core STATIC ${CORE_SOURCES})
target_include_directories(clawdsecbot_qt_core PUBLIC src)
target_link_libraries(clawdsecbot_qt_core PUBLIC Qt6::Core Qt6::Gui Qt6::Concurrent)
target_compile_definitions(clawdsecbot_qt_core PUBLIC CLAWDSECBOT_VERSION="${PROJECT_VERSION}")

qt_add_executable(clawdsecbot_qt
    src/main.cpp
    ${UI_SOURCES}
)
target_include_directories(clawdsecbot_qt PRIVATE src)
target_link_libraries(clawdsecbot_qt PRIVATE clawdsecbot_qt_core Qt6::Widgets)
add_dependencies(clawdsecbot_qt botsec_go_library)

if(MSVC)
    target_compile_options(clawdsecbot_qt_core PRIVATE /W4)
    target_compile_options(clawdsecbot_qt PRIVATE /W4)
else()
    target_compile_options(clawdsecbot_qt_core PRIVATE -Wall -Wextra -Wpedantic)
    target_compile_options(clawdsecbot_qt PRIVATE -Wall -Wextra -Wpedantic)
endif()

if(APPLE)
    add_custom_command(TARGET clawdsecbot_qt POST_BUILD
        COMMAND "${CMAKE_COMMAND}" -E make_directory "$<TARGET_BUNDLE_DIR:clawdsecbot_qt>/Contents/Resources/plugins"
        COMMAND "${CMAKE_COMMAND}" -E copy_if_different "${GO_LIBRARY_SOURCE}" "$<TARGET_BUNDLE_DIR:clawdsecbot_qt>/Contents/Resources/plugins/${GO_LIBRARY_NAME}"
        VERBATIM
    )
else()
    add_custom_command(TARGET clawdsecbot_qt POST_BUILD
        COMMAND "${CMAKE_COMMAND}" -E make_directory "$<TARGET_FILE_DIR:clawdsecbot_qt>/plugins"
        COMMAND "${CMAKE_COMMAND}" -E copy_if_different "${GO_LIBRARY_SOURCE}" "$<TARGET_FILE_DIR:clawdsecbot_qt>/plugins/${GO_LIBRARY_NAME}"
        VERBATIM
    )
endif()

if(APPLE)
    set_target_properties(clawdsecbot_qt PROPERTIES
        MACOSX_BUNDLE TRUE
        OUTPUT_NAME "${APP_DISPLAY_NAME}"
        MACOSX_BUNDLE_BUNDLE_NAME "${APP_DISPLAY_NAME}"
        MACOSX_BUNDLE_GUI_IDENTIFIER "${APP_BUNDLE_IDENTIFIER}"
    )
endif()

enable_testing()

add_library(go_bridge_init_failure_fixture SHARED tests/go_bridge_init_failure_fixture.cpp)
add_library(go_bridge_monitor_error_fixture SHARED tests/go_bridge_monitor_error_fixture.cpp)
if(WIN32)
    set_target_properties(go_bridge_init_failure_fixture go_bridge_monitor_error_fixture
        PROPERTIES WINDOWS_EXPORT_ALL_SYMBOLS ON)
endif()

qt_add_executable(models_test
    tests/models_test.cpp
)
target_include_directories(models_test PRIVATE src)
target_link_libraries(models_test PRIVATE clawdsecbot_qt_core Qt6::Test)
add_test(NAME models_test COMMAND models_test)

qt_add_executable(monitor_smoke_test
    tests/monitor_smoke_test.cpp
    src/ui/ProtectionMonitorWindow.cpp
    src/ui/widgets/GradientWidget.cpp
    src/ui/widgets/TrendChartWidget.cpp
    src/ui/widgets/AnalysisLogView.cpp
    src/ui/widgets/SecurityEventListWidget.cpp
    src/ui/UiDialogs.cpp
    src/ui/DialogChrome.cpp
    src/resources.qrc
)
target_include_directories(monitor_smoke_test PRIVATE src)
target_link_libraries(monitor_smoke_test PRIVATE clawdsecbot_qt_core Qt6::Widgets Qt6::Test)
target_compile_definitions(monitor_smoke_test PRIVATE
    MONITOR_ERROR_FIXTURE_PATH="$<TARGET_FILE:go_bridge_monitor_error_fixture>")
add_dependencies(monitor_smoke_test go_bridge_monitor_error_fixture)
add_test(NAME monitor_smoke_test COMMAND monitor_smoke_test)
set_tests_properties(monitor_smoke_test PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen")

qt_add_executable(audit_timeline_test
    tests/audit_timeline_test.cpp
    src/ui/widgets/AuditTimelineWidget.cpp
)
target_include_directories(audit_timeline_test PRIVATE src)
target_link_libraries(audit_timeline_test PRIVATE clawdsecbot_qt_core Qt6::Widgets Qt6::Test)
add_test(NAME audit_timeline_test COMMAND audit_timeline_test)
set_tests_properties(audit_timeline_test PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen")

qt_add_executable(audit_models_test
    tests/audit_models_test.cpp
)
target_include_directories(audit_models_test PRIVATE src)
target_link_libraries(audit_models_test PRIVATE clawdsecbot_qt_core Qt6::Test)
add_test(NAME audit_models_test COMMAND audit_models_test)

qt_add_executable(bridge_lifecycle_test
    tests/bridge_lifecycle_test.cpp
)
target_include_directories(bridge_lifecycle_test PRIVATE src)
target_link_libraries(bridge_lifecycle_test PRIVATE clawdsecbot_qt_core Qt6::Concurrent Qt6::Test)
target_compile_definitions(bridge_lifecycle_test PRIVATE
    INIT_FAILURE_FIXTURE_PATH="$<TARGET_FILE:go_bridge_init_failure_fixture>")
add_dependencies(bridge_lifecycle_test go_bridge_init_failure_fixture)
add_test(NAME bridge_lifecycle_test COMMAND bridge_lifecycle_test)

qt_add_executable(protection_service_test
    tests/protection_service_test.cpp
)
target_include_directories(protection_service_test PRIVATE src)
target_link_libraries(protection_service_test PRIVATE clawdsecbot_qt_core Qt6::Test)
target_compile_definitions(protection_service_test PRIVATE GO_LIBRARY_PATH="${GO_LIBRARY_SOURCE}")
add_dependencies(protection_service_test botsec_go_library)
add_test(NAME protection_service_test COMMAND protection_service_test)

add_test(NAME ui_non_blocking_source_test
    COMMAND "${CMAKE_COMMAND}" -DSOURCE_DIR=${CMAKE_CURRENT_SOURCE_DIR}
            -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/ui_non_blocking_source_test.cmake"
)

qt_add_executable(dialog_visual_test
    tests/dialog_visual_test.cpp
    src/ui/dialogs/SettingsDialog.h
    src/ui/dialogs/ProtectionConfigDialog.h
    src/ui/dialogs/SkillScanDialog.h
    src/ui/dialogs/SkillScanResultsDialog.h
    src/ui/dialogs/OnboardingDialog.h
    src/ui/dialogs/MitigationDialog.h
    src/ui/dialogs/BotIconPickerDialog.h
    src/ui/dialogs/AppStoreGuideDialog.h
    src/ui/widgets/SecurityEventListWidget.h
    src/ui/Dialogs.cpp
    src/ui/SkillScanDialog.cpp
    src/ui/dialogs/BotIconPickerDialog.cpp
    src/ui/AppStoreGuideDialog.cpp
    src/ui/widgets/SecurityEventListWidget.cpp
    src/ui/UiDialogs.cpp
    src/ui/DialogChrome.cpp
    src/resources.qrc
)
target_include_directories(dialog_visual_test PRIVATE src)
target_link_libraries(dialog_visual_test PRIVATE clawdsecbot_qt_core Qt6::Widgets Qt6::Test)
target_compile_definitions(dialog_visual_test PRIVATE
    DIALOG_FIXTURE_PATH="$<TARGET_FILE:go_bridge_monitor_error_fixture>")
add_dependencies(dialog_visual_test go_bridge_monitor_error_fixture)
add_test(NAME dialog_visual_test COMMAND dialog_visual_test)
set_tests_properties(dialog_visual_test PROPERTIES ENVIRONMENT "QT_QPA_PLATFORM=offscreen")

include(GNUInstallDirs)
install(TARGETS clawdsecbot_qt
    BUNDLE DESTINATION .
    RUNTIME DESTINATION "${CMAKE_INSTALL_BINDIR}"
)
if(NOT APPLE)
    install(FILES "${GO_LIBRARY_SOURCE}" DESTINATION "${CMAKE_INSTALL_BINDIR}/plugins")
endif()

if(APPLE)
    set(CPACK_GENERATOR "DragNDrop")
else()
    set(CPACK_GENERATOR "TGZ")
endif()
set(CPACK_PACKAGE_NAME "${APP_DISPLAY_NAME}")
set(CPACK_PACKAGE_VERSION "${PROJECT_VERSION}")
include(CPack)
