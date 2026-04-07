#ifndef RUNNER_SINGLE_INSTANCE_MANAGER_H_
#define RUNNER_SINGLE_INSTANCE_MANAGER_H_

#include <atomic>
#include <functional>
#include <string>
#include <thread>

class SingleInstanceManager {
 public:
  SingleInstanceManager();
  ~SingleInstanceManager();

  bool Initialize();
  bool IsPrimaryInstance() const;
  bool NotifyPrimaryInstance() const;
  void StartListening(std::function<void()> on_activate);
  void Shutdown();

 private:
  void RunServer();

  std::string lock_file_path_;
  std::string socket_path_;
  int lock_fd_ = -1;
  int server_fd_ = -1;
  std::thread server_thread_;
  std::function<void()> on_activate_;
  std::atomic<bool> is_primary_{false};
  std::atomic<bool> stop_requested_{false};
};

#endif  // RUNNER_SINGLE_INSTANCE_MANAGER_H_
