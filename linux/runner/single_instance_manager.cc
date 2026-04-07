#include "single_instance_manager.h"

#include <fcntl.h>
#include <sys/file.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

#include <cstring>

#include <glib.h>

namespace {
constexpr char kShowCommand[] = "SHOW";
}

SingleInstanceManager::SingleInstanceManager() {
  const gchar* runtime_dir = g_get_user_runtime_dir();
  if (runtime_dir == nullptr || *runtime_dir == '\0') {
    runtime_dir = g_get_tmp_dir();
  }

  lock_file_path_ = std::string(runtime_dir) + "/clawdsecbot.lock";
  socket_path_ = std::string(runtime_dir) + "/clawdsecbot.sock";
}

SingleInstanceManager::~SingleInstanceManager() {
  Shutdown();
}

bool SingleInstanceManager::Initialize() {
  lock_fd_ = open(lock_file_path_.c_str(), O_CREAT | O_RDWR, 0600);
  if (lock_fd_ < 0) {
    return false;
  }

  if (flock(lock_fd_, LOCK_EX | LOCK_NB) != 0) {
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
  const int fd = socket(AF_UNIX, SOCK_STREAM, 0);
  if (fd < 0) {
    return false;
  }

  sockaddr_un addr{};
  addr.sun_family = AF_UNIX;
  std::strncpy(addr.sun_path, socket_path_.c_str(), sizeof(addr.sun_path) - 1);

  const bool connected =
      connect(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) == 0;
  if (!connected) {
    close(fd);
    return false;
  }

  const ssize_t sent = send(fd, kShowCommand, sizeof(kShowCommand) - 1, 0);
  close(fd);
  return sent >= 0;
}

void SingleInstanceManager::StartListening(std::function<void()> on_activate) {
  on_activate_ = std::move(on_activate);
  if (!is_primary_.load() || server_thread_.joinable()) {
    return;
  }

  server_thread_ = std::thread(&SingleInstanceManager::RunServer, this);
}

void SingleInstanceManager::Shutdown() {
  stop_requested_ = true;

  if (is_primary_.load()) {
    NotifyPrimaryInstance();
  }

  if (server_thread_.joinable()) {
    server_thread_.join();
  }

  if (server_fd_ >= 0) {
    close(server_fd_);
    server_fd_ = -1;
  }

  unlink(socket_path_.c_str());

  if (lock_fd_ >= 0) {
    flock(lock_fd_, LOCK_UN);
    close(lock_fd_);
    lock_fd_ = -1;
  }
}

void SingleInstanceManager::RunServer() {
  unlink(socket_path_.c_str());

  server_fd_ = socket(AF_UNIX, SOCK_STREAM, 0);
  if (server_fd_ < 0) {
    return;
  }

  sockaddr_un addr{};
  addr.sun_family = AF_UNIX;
  std::strncpy(addr.sun_path, socket_path_.c_str(), sizeof(addr.sun_path) - 1);

  if (bind(server_fd_, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) != 0 ||
      listen(server_fd_, 4) != 0) {
    close(server_fd_);
    server_fd_ = -1;
    unlink(socket_path_.c_str());
    return;
  }

  while (!stop_requested_.load()) {
    const int client_fd = accept(server_fd_, nullptr, nullptr);
    if (client_fd < 0) {
      continue;
    }

    char buffer[16] = {};
    const ssize_t received = recv(client_fd, buffer, sizeof(buffer), 0);
    close(client_fd);

    if (received <= 0 || stop_requested_.load()) {
      continue;
    }

    if (std::string(buffer, buffer + received).find(kShowCommand) ==
        std::string::npos) {
      continue;
    }

    if (on_activate_) {
      g_idle_add_full(
          G_PRIORITY_DEFAULT,
          [](gpointer data) -> gboolean {
            auto* callback = static_cast<std::function<void()>*>(data);
            (*callback)();
            return G_SOURCE_REMOVE;
          },
          &on_activate_, nullptr);
    }
  }
}
