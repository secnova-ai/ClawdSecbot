#include <flutter/dart_project.h>
#include <flutter/flutter_view_controller.h>
#include <windows.h>

#include "flutter_window.h"
#include "single_instance_manager.h"
#include "utils.h"

namespace {
bool IsRunningAsAdministrator() {
  BOOL is_admin = FALSE;
  PSID admin_group = nullptr;
  SID_IDENTIFIER_AUTHORITY nt_authority = SECURITY_NT_AUTHORITY;

  if (!AllocateAndInitializeSid(&nt_authority, 2, SECURITY_BUILTIN_DOMAIN_RID,
                                DOMAIN_ALIAS_RID_ADMINS, 0, 0, 0, 0, 0, 0,
                                &admin_group)) {
    return false;
  }

  if (!CheckTokenMembership(nullptr, admin_group, &is_admin)) {
    is_admin = FALSE;
  }

  FreeSid(admin_group);
  return is_admin == TRUE;
}
}  // namespace

int APIENTRY wWinMain(_In_ HINSTANCE instance, _In_opt_ HINSTANCE prev,
                      _In_ wchar_t *command_line, _In_ int show_command) {
  // Attach to console when present (e.g., 'flutter run') or create a
  // new console when running with a debugger.
  if (!::AttachConsole(ATTACH_PARENT_PROCESS) && ::IsDebuggerPresent()) {
    CreateAndAttachConsole();
  }

  if (!IsRunningAsAdministrator()) {
    MessageBoxW(
        nullptr,
        L"ClawdSecbot requires administrator privileges. Please restart and "
        L"approve the UAC prompt.",
        L"Administrator Privileges Required",
        MB_OK | MB_ICONERROR | MB_SETFOREGROUND | MB_TOPMOST);
    return EXIT_FAILURE;
  }

  // Initialize COM, so that it is available for use in the library and/or
  // plugins.
  ::CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);

  flutter::DartProject project(L"data");

  std::vector<std::string> command_line_arguments =
      GetCommandLineArguments();
  const bool is_multi_window =
      !command_line_arguments.empty() &&
      command_line_arguments.front() == "multi_window";

  SingleInstanceManager single_instance;
  if (!is_multi_window) {
    if (!single_instance.Initialize()) {
      MessageBoxW(nullptr, L"Failed to initialize single-instance manager.",
                  L"Startup Error",
                  MB_OK | MB_ICONERROR | MB_SETFOREGROUND | MB_TOPMOST);
      return EXIT_FAILURE;
    }
    if (!single_instance.IsPrimaryInstance()) {
      single_instance.NotifyPrimaryInstance();
      return EXIT_SUCCESS;
    }
  }

  project.set_dart_entrypoint_arguments(std::move(command_line_arguments));

  FlutterWindow window(project);
  Win32Window::Point origin(10, 10);
  Win32Window::Size size(600, 780);
  if (!window.Create(L"bot_sec_manager", origin, size)) {
    return EXIT_FAILURE;
  }
  if (!is_multi_window) {
    single_instance.AttachMainWindow(window.GetHandle());
  }
  window.SetQuitOnClose(true);

  ::MSG msg;
  while (::GetMessage(&msg, nullptr, 0, 0)) {
    ::TranslateMessage(&msg);
    ::DispatchMessage(&msg);
  }

  ::CoUninitialize();
  if (!is_multi_window) {
    single_instance.Shutdown();
  }
  return EXIT_SUCCESS;
}
