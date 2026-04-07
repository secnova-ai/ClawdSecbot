#ifndef RUNNER_SINGLE_INSTANCE_MANAGER_H_
#define RUNNER_SINGLE_INSTANCE_MANAGER_H_

#include <windows.h>

#include <atomic>
#include <mutex>
#include <string>
#include <thread>

class SingleInstanceManager {
 public:
  SingleInstanceManager();
  ~SingleInstanceManager();

  bool Initialize();
  bool IsPrimaryInstance() const;
  bool NotifyPrimaryInstance() const;
  void AttachMainWindow(HWND hwnd);
  void Shutdown();

 private:
  void RunPipeServer();

  std::wstring mutex_name_;
  std::wstring pipe_name_;
  HANDLE mutex_handle_ = nullptr;
  mutable std::mutex window_mutex_;
  HWND main_window_ = nullptr;
  std::thread pipe_thread_;
  std::atomic<bool> is_primary_{false};
  std::atomic<bool> stop_requested_{false};
};

#endif  // RUNNER_SINGLE_INSTANCE_MANAGER_H_
