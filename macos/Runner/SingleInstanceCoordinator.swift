import Cocoa
import Darwin

final class SingleInstanceCoordinator {
  private let lockFilePath: String
  private let socketPath: String
  private let queue = DispatchQueue(label: "com.bot.secnova.clawdsecbot.single-instance")

  private var lockFD: Int32 = -1
  private var serverFD: Int32 = -1
  private var readSource: DispatchSourceRead?

  var onActivateExistingInstance: (() -> Void)?

  init() {
    let tempDir = NSTemporaryDirectory()
    lockFilePath = (tempDir as NSString).appendingPathComponent("clawdsecbot.lock")
    socketPath = (tempDir as NSString).appendingPathComponent("clawdsecbot.sock")
  }

  func acquireOrNotifyExisting() -> Bool {
    lockFD = open(lockFilePath, O_CREAT | O_RDWR, S_IRUSR | S_IWUSR)
    guard lockFD >= 0 else {
      return true
    }

    if flock(lockFD, LOCK_EX | LOCK_NB) != 0 {
      _ = notifyExistingInstance()
      close(lockFD)
      lockFD = -1
      return false
    }

    startSocketServer()
    return true
  }

  func shutdown() {
    readSource?.cancel()
    readSource = nil

    if serverFD >= 0 {
      close(serverFD)
      serverFD = -1
    }

    unlink(socketPath)

    if lockFD >= 0 {
      flock(lockFD, LOCK_UN)
      close(lockFD)
      lockFD = -1
    }
  }

  private func notifyExistingInstance() -> Bool {
    let fd = socket(AF_UNIX, SOCK_STREAM, 0)
    guard fd >= 0 else {
      return false
    }
    defer { close(fd) }

    var addr = sockaddr_un()
    addr.sun_len = UInt8(MemoryLayout<sockaddr_un>.size)
    addr.sun_family = sa_family_t(AF_UNIX)

    let maxPathLength = MemoryLayout.size(ofValue: addr.sun_path)
    socketPath.withCString { pathPtr in
      withUnsafeMutablePointer(to: &addr.sun_path) { sunPathPtr in
        sunPathPtr.withMemoryRebound(to: CChar.self, capacity: maxPathLength) { destPtr in
          strncpy(destPtr, pathPtr, maxPathLength - 1)
          destPtr[maxPathLength - 1] = 0
        }
      }
    }

    let connected = withUnsafePointer(to: &addr) { ptr -> Int32 in
      ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) {
        connect(fd, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
      }
    }
    guard connected == 0 else {
      return false
    }

    let command = Array("SHOW".utf8)
    let sent = command.withUnsafeBytes { bytes in
      send(fd, bytes.baseAddress, bytes.count, 0)
    }
    return sent >= 0
  }

  private func startSocketServer() {
    unlink(socketPath)

    serverFD = socket(AF_UNIX, SOCK_STREAM, 0)
    guard serverFD >= 0 else {
      return
    }

    var addr = sockaddr_un()
    addr.sun_len = UInt8(MemoryLayout<sockaddr_un>.size)
    addr.sun_family = sa_family_t(AF_UNIX)

    let maxPathLength = MemoryLayout.size(ofValue: addr.sun_path)
    socketPath.withCString { pathPtr in
      withUnsafeMutablePointer(to: &addr.sun_path) { sunPathPtr in
        sunPathPtr.withMemoryRebound(to: CChar.self, capacity: maxPathLength) { destPtr in
          strncpy(destPtr, pathPtr, maxPathLength - 1)
          destPtr[maxPathLength - 1] = 0
        }
      }
    }

    let bound = withUnsafePointer(to: &addr) { ptr -> Int32 in
      ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) {
        bind(serverFD, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
      }
    }
    guard bound == 0, listen(serverFD, 4) == 0 else {
      close(serverFD)
      serverFD = -1
      unlink(socketPath)
      return
    }

    readSource = DispatchSource.makeReadSource(fileDescriptor: serverFD, queue: queue)
    readSource?.setEventHandler { [weak self] in
      self?.acceptClient()
    }
    readSource?.setCancelHandler { [weak self] in
      guard let self else { return }
      if self.serverFD >= 0 {
        close(self.serverFD)
        self.serverFD = -1
      }
    }
    readSource?.resume()
  }

  private func acceptClient() {
    var addr = sockaddr()
    var len: socklen_t = socklen_t(MemoryLayout<sockaddr>.size)
    let clientFD = accept(serverFD, &addr, &len)
    guard clientFD >= 0 else {
      return
    }
    defer { close(clientFD) }

    var buffer = [UInt8](repeating: 0, count: 16)
    let count = recv(clientFD, &buffer, buffer.count, 0)
    guard count > 0 else {
      return
    }

    let message = String(decoding: buffer.prefix(Int(count)), as: UTF8.self)
    guard message.contains("SHOW") else {
      return
    }

    DispatchQueue.main.async { [weak self] in
      self?.onActivateExistingInstance?()
    }
  }

  deinit {
    shutdown()
  }
}
