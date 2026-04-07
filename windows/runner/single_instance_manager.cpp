#include "single_instance_manager.h"
#include "flutter_window.h"

#include <array>
#include <string_view>

namespace {
constexpr wchar_t kMutexName[] = L"Global\\ClawdSecbot.SingleInstance";
constexpr wchar_t kPipeName[] = L"\\\\.\\pipe\\ClawdSecbot.SingleInstance";
constexpr std::string_view kShowCommand = "SHOW";
}

SingleInstanceManager::SingleInstanceManager()
    : mutex_name_(kMutexName), pipe_name_(kPipeName) {}

SingleInstanceManager::~SingleInstanceManager() {
  Shutdown();
}

bool SingleInstanceManager::Initialize() {
  mutex_handle_ = CreateMutexW(nullptr, TRUE, mutex_name_.c_str());
  if (mutex_handle_ == nullptr) {
    return false;
  }

  if (GetLastError() == ERROR_ALREADY_EXISTS) {
    is_primary_ = false;
    return true;
  }

  is_primary_ = true;
  return true;
}

bool SingleInstanceManager::IsPrimaryInstance() const {
  return is_primary_.load();
}

bool SingleInstanceManager::NotifyPrimaryInstance() const {
  if (pipe_name_.empty()) {
    return false;
  }

  if (!WaitNamedPipeW(pipe_name_.c_str(), 1500)) {
    return false;
  }

  HANDLE pipe = CreateFileW(pipe_name_.c_str(), GENERIC_WRITE, 0, nullptr,
                            OPEN_EXISTING, 0, nullptr);
  if (pipe == INVALID_HANDLE_VALUE) {
    return false;
  }

  DWORD bytes_written = 0;
  const auto* buffer = reinterpret_cast<const void*>(kShowCommand.data());
  const DWORD buffer_size = static_cast<DWORD>(kShowCommand.size());
  const BOOL ok = WriteFile(pipe, buffer, buffer_size, &bytes_written, nullptr);
  CloseHandle(pipe);
  return ok == TRUE;
}

void SingleInstanceManager::AttachMainWindow(HWND hwnd) {
  {
    std::lock_guard<std::mutex> lock(window_mutex_);
    main_window_ = hwnd;
  }

  if (!is_primary_.load() || pipe_thread_.joinable()) {
    return;
  }

  pipe_thread_ = std::thread(&SingleInstanceManager::RunPipeServer, this);
}

void SingleInstanceManager::Shutdown() {
  stop_requested_ = true;

  if (is_primary_.load()) {
    NotifyPrimaryInstance();
  }

  if (pipe_thread_.joinable()) {
    pipe_thread_.join();
  }

  if (mutex_handle_ != nullptr) {
    ReleaseMutex(mutex_handle_);
    CloseHandle(mutex_handle_);
    mutex_handle_ = nullptr;
  }
}

void SingleInstanceManager::RunPipeServer() {
  while (!stop_requested_.load()) {
    HANDLE pipe = CreateNamedPipeW(
        pipe_name_.c_str(), PIPE_ACCESS_INBOUND,
        PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT, 1, 256, 256, 0,
        nullptr);
    if (pipe == INVALID_HANDLE_VALUE) {
      return;
    }

    const BOOL connected =
        ConnectNamedPipe(pipe, nullptr)
            ? TRUE
            : (GetLastError() == ERROR_PIPE_CONNECTED ? TRUE : FALSE);
    if (!connected) {
      CloseHandle(pipe);
      continue;
    }

    std::array<char, 16> buffer{};
    DWORD bytes_read = 0;
    if (ReadFile(pipe, buffer.data(), static_cast<DWORD>(buffer.size()),
                 &bytes_read, nullptr) &&
        bytes_read > 0) {
      HWND hwnd = nullptr;
      {
        std::lock_guard<std::mutex> lock(window_mutex_);
        hwnd = main_window_;
      }
      if (hwnd != nullptr && !stop_requested_.load()) {
        PostMessage(hwnd, WM_SHOW_EXISTING_INSTANCE, 0, 0);
      }
    }

    FlushFileBuffers(pipe);
    DisconnectNamedPipe(pipe);
    CloseHandle(pipe);
  }
}
