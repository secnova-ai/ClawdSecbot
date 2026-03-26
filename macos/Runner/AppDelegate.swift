import Cocoa
import FlutterMacOS
import desktop_multi_window

@main
class AppDelegate: FlutterAppDelegate {
  private var bookmarkHandler: SecurityScopedBookmarkHandler?
  private var appExitChannel: FlutterMethodChannel?
  
  override func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
    return false  // 窗口关闭后不退出应用,保持托盘运行
  }

  override func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
    guard let channel = appExitChannel else {
      return .terminateNow
    }

    channel.invokeMethod("requestAppExit", arguments: nil)
    return .terminateCancel
  }

  override func applicationSupportsSecureRestorableState(_ app: NSApplication) -> Bool {
    return true
  }

  override func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
    if !flag, let window = mainFlutterWindow {
      window.makeKeyAndOrderFront(nil)
      sender.activate(ignoringOtherApps: true)
    }
    return true
  }
  
  override func applicationDidFinishLaunching(_ notification: Notification) {
    // Register security-scoped bookmark handler
    if let controller = mainFlutterWindow?.contentViewController as? FlutterViewController {
      bookmarkHandler = SecurityScopedBookmarkHandler(messenger: controller.engine.binaryMessenger)
      appExitChannel = FlutterMethodChannel(
        name: "com.clawdbot.guard/app_exit",
        binaryMessenger: controller.engine.binaryMessenger
      )
    }
    
    // 在应用完全启动后设置多窗口插件回调，确保引擎完全就绪
    // 此时主窗口的 Flutter 引擎已经完全初始化,不会出现 "Invalid engine handle" 错误
    FlutterMultiWindowPlugin.setOnWindowCreatedCallback { controller in
      // 为新创建的子窗口注册所有插件
      RegisterGeneratedPlugins(registry: controller)
      // 子窗口保留原生红黄绿按钮，不再隐藏
    }
  }
}

// MARK: - Security-Scoped Bookmark Handler

/// Handles Security-Scoped Bookmarks for sandbox file access
class SecurityScopedBookmarkHandler: NSObject {
    private let channel: FlutterMethodChannel
    private let bookmarkKey = "com.clawdbot.guard.configDirBookmark"
    private var accessingURLs: [URL] = []
    
    init(messenger: FlutterBinaryMessenger) {
        channel = FlutterMethodChannel(
            name: "com.clawdbot.guard/security_scoped_bookmark",
            binaryMessenger: messenger
        )
        super.init()
        
        channel.setMethodCallHandler { [weak self] call, result in
            self?.handleMethodCall(call, result: result)
        }
    }
    
    private func handleMethodCall(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
        switch call.method {
        case "hasStoredBookmark":
            result(hasStoredBookmark())
            
        case "getBookmarkedPath":
            result(getBookmarkedPath())
            
        case "selectAndStoreDirectory":
            selectAndStoreDirectory(result: result)
            
        case "startAccessingDirectory":
            if let args = call.arguments as? [String: Any],
               let path = args["path"] as? String {
                result(startAccessingDirectory(path: path))
            } else {
                result(startAccessingStoredDirectory())
            }
            
        case "stopAccessingDirectory":
            stopAccessingDirectory()
            result(true)
            
        case "clearBookmark":
            clearBookmark()
            result(true)
            
        case "findConfigDirectory":
            result(findExistingConfigDirectory())
            
        default:
            result(FlutterMethodNotImplemented)
        }
    }
    
    /// Check if we have a stored bookmark
    private func hasStoredBookmark() -> Bool {
        return UserDefaults.standard.data(forKey: bookmarkKey) != nil
    }
    
    /// Get the path from stored bookmark (if valid)
    private func getBookmarkedPath() -> String? {
        guard let bookmarkData = UserDefaults.standard.data(forKey: bookmarkKey) else {
            return nil
        }
        
        do {
            var isStale = false
            let url = try URL(
                resolvingBookmarkData: bookmarkData,
                options: .withSecurityScope,
                relativeTo: nil,
                bookmarkDataIsStale: &isStale
            )
            
            if isStale {
                // Bookmark is stale, need to re-authorize
                UserDefaults.standard.removeObject(forKey: bookmarkKey)
                return nil
            }
            
            return url.path
        } catch {
            print("Failed to resolve bookmark: \(error)")
            return nil
        }
    }
    
    /// Show directory picker and store the bookmark
    private func selectAndStoreDirectory(result: @escaping FlutterResult) {
        DispatchQueue.main.async {
            let panel = NSOpenPanel()
            panel.canChooseFiles = false
            panel.canChooseDirectories = true
            panel.allowsMultipleSelection = false
            panel.canCreateDirectories = false
            panel.showsHiddenFiles = true
            panel.message = NSLocalizedString(
                "Select the configuration directory (e.g., .openclaw, .moltbot, or .clawdbot)",
                comment: "Directory picker message"
            )
            panel.prompt = NSLocalizedString("Select", comment: "Select button")
            
            // Try to start in home directory
            if let homeURL = FileManager.default.homeDirectoryForCurrentUser as URL? {
                panel.directoryURL = homeURL
            }
            
            panel.begin { [weak self] response in
                if response == .OK, let url = panel.url {
                    self?.storeBookmark(for: url, result: result)
                } else {
                    result(nil)
                }
            }
        }
    }
    
    /// Store security-scoped bookmark for the selected directory
    private func storeBookmark(for url: URL, result: @escaping FlutterResult) {
        do {
            let bookmarkData = try url.bookmarkData(
                options: .withSecurityScope,
                includingResourceValuesForKeys: nil,
                relativeTo: nil
            )
            
            UserDefaults.standard.set(bookmarkData, forKey: bookmarkKey)
            result(url.path)
        } catch {
            print("Failed to create bookmark: \(error)")
            result(FlutterError(
                code: "BOOKMARK_ERROR",
                message: "Failed to create security-scoped bookmark",
                details: error.localizedDescription
            ))
        }
    }
    
    /// Start accessing the stored bookmarked directory
    private func startAccessingStoredDirectory() -> Bool {
        guard let bookmarkData = UserDefaults.standard.data(forKey: bookmarkKey) else {
            return false
        }
        
        do {
            var isStale = false
            let url = try URL(
                resolvingBookmarkData: bookmarkData,
                options: .withSecurityScope,
                relativeTo: nil,
                bookmarkDataIsStale: &isStale
            )
            
            if isStale {
                print("Bookmark is stale")
                return false
            }
            
            if url.startAccessingSecurityScopedResource() {
                accessingURLs.append(url)
                return true
            }
            return false
        } catch {
            print("Failed to resolve bookmark: \(error)")
            return false
        }
    }
    
    /// Start accessing a specific directory path
    private func startAccessingDirectory(path: String) -> Bool {
        let url = URL(fileURLWithPath: path)
        if url.startAccessingSecurityScopedResource() {
            accessingURLs.append(url)
            return true
        }
        return false
    }
    
    /// Stop accessing all security-scoped resources
    private func stopAccessingDirectory() {
        for url in accessingURLs {
            url.stopAccessingSecurityScopedResource()
        }
        accessingURLs.removeAll()
    }
    
    /// Clear the stored bookmark
    private func clearBookmark() {
        stopAccessingDirectory()
        UserDefaults.standard.removeObject(forKey: bookmarkKey)
    }
    
    /// Find existing config directory in common locations
    private func findExistingConfigDirectory() -> String? {
        let homeDir = FileManager.default.homeDirectoryForCurrentUser
        let possibleDirs = [
            homeDir.appendingPathComponent(".openclaw"),
            homeDir.appendingPathComponent(".moltbot"),
            homeDir.appendingPathComponent(".clawdbot")
        ]
        
        for dir in possibleDirs {
            var isDirectory: ObjCBool = false
            if FileManager.default.fileExists(atPath: dir.path, isDirectory: &isDirectory),
               isDirectory.boolValue {
                return dir.path
            }
        }
        
        return nil
    }
    
    deinit {
        stopAccessingDirectory()
    }
}
