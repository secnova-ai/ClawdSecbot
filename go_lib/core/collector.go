package core

import (
	"os"
	"path/filepath"
	"strings"

	"go_lib/core/logging"
)

// Collector 系统信息采集器接口
// 定义了系统快照采集的标准方法，所有采集器（包括测试用的模拟采集器）都应实现此接口
type Collector interface {
	// Collect 采集并返回当前系统快照
	Collect() (SystemSnapshot, error)
}

// platformCollector 平台相关的系统信息采集器
// 各平台在 collector_darwin.go / collector_linux.go 中实现具体采集逻辑
type platformCollector struct {
	// ConfigPath 是从 Flutter 传入的授权路径
	ConfigPath string
}

// 确保 platformCollector 实现了 Collector 接口
var _ Collector = (*platformCollector)(nil)

// NewCollector 创建适用于当前平台的系统信息采集器
func NewCollector(configPath string) Collector {
	return &platformCollector{ConfigPath: configPath}
}

// Collect 采集当前系统快照
func (c *platformCollector) Collect() (SystemSnapshot, error) {
	logging.Info("开始采集系统快照...")

	ports, err := c.getOpenPorts()
	if err != nil {
		// 记录错误但不中断,继续采集其他信息
		logging.Warning("采集端口信息失败: %v", err)
	} else {
		logging.Info("成功采集到 %d 个开放端口", len(ports))
	}

	procs, err := c.getRunningProcesses()
	if err != nil {
		logging.Warning("采集进程信息失败: %v", err)
	} else {
		logging.Info("成功采集到 %d 个运行进程", len(procs))
	}

	services, err := c.getServices()
	if err != nil {
		logging.Warning("采集服务信息失败: %v", err)
	} else {
		logging.Info("成功采集到 %d 个系统服务", len(services))
	}

	return SystemSnapshot{
		OpenPorts:        ports,
		RunningProcesses: procs,
		Services:         services,
		FileExists: func(path string) bool {
			originalPath := path
			// 处理波浪号 ~ 扩展
			if strings.HasPrefix(path, "~/") {
				var dirname string

				// 优先使用外部传入的 ConfigPath（来自 Flutter 的授权路径）
				if c.ConfigPath != "" {
					dirname = c.ConfigPath
					logging.Info("使用授权路径进行扩展: %s", dirname)
				} else {
					if userHome, err := os.UserHomeDir(); err == nil && userHome != "" {
						dirname = userHome
						logging.Info("Resolved tilde HOME from system user home: %s", dirname)
					} else {
						pm := GetPathManager()
						if pm.IsInitialized() {
							dirname = pm.GetHomeDir()
							logging.Warning("Failed to resolve system user home, using PathManager HOME: %s", dirname)
						} else {
							logging.Warning("Failed to resolve HOME for tilde expansion")
						}
					}
				}

				if dirname == "" {
					logging.Warning("Failed to expand path because HOME is empty: %s", originalPath)
					return false
				}
				path = filepath.Join(dirname, strings.TrimPrefix(path, "~/"))
				logging.Debug("路径扩展: %s -> %s", originalPath, path)
			}

			_, err := os.Stat(path)
			exists := err == nil || !os.IsNotExist(err)

			if exists {
				logging.Info("文件检查成功: %s (存在)", path)
			} else {
				logging.Warning("文件检查失败: %s (不存在或无权限), 错误: %v", path, err)
			}

			return exists
		},
	}, nil
}
