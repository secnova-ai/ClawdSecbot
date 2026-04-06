package dintalclaw

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go_lib/core"
)

// checkConfigPermissions 检查 mykey.py 配置文件权限是否为 0600
func checkConfigPermissions(configPath string, risks *[]core.Risk) {
	info, err := os.Stat(configPath)
	if err != nil {
		return
	}

	if runtime.GOOS == "windows" {
		configACL, aclErr := checkWindowsACL(configPath)
		if aclErr != nil {
			*risks = append(*risks, core.Risk{
				ID:          "config_perm_unsafe",
				Title:       "配置文件权限不安全",
				Description: fmt.Sprintf("配置文件 ACL 校验失败: %v", aclErr),
				Level:       core.RiskLevelCritical,
				Args:        map[string]interface{}{"path": configPath, "acl_summary": "acl check failed"},
			})
		} else if !configACL.Safe {
			*risks = append(*risks, core.Risk{
				ID:          "config_perm_unsafe",
				Title:       "配置文件权限不安全",
				Description: fmt.Sprintf("配置文件 ACL 不安全: %s", configACL.Summary),
				Level:       core.RiskLevelCritical,
				Args: map[string]interface{}{
					"path":           configPath,
					"acl_summary":    configACL.Summary,
					"acl_violations": configACL.Violations,
				},
			})
		}
		return
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		*risks = append(*risks, core.Risk{
			ID:          "config_perm_unsafe",
			Title:       "配置文件权限不安全",
			Description: fmt.Sprintf("mykey.py 当前权限为 %o，期望为 600", perm),
			Level:       core.RiskLevelCritical,
			Args:        map[string]interface{}{"path": configPath, "current": fmt.Sprintf("%o", perm)},
		})
	}
}

// checkMemoryDirPermissions 检查安装根目录下 memory 目录权限是否为 0700（技能目录，非 temp）
func checkMemoryDirPermissions(installRoot string, risks *[]core.Risk) {
	memoryDir := filepath.Join(installRoot, "memory")
	info, err := os.Stat(memoryDir)
	if err != nil {
		return
	}

	if runtime.GOOS == "windows" {
		dirACL, aclErr := checkWindowsACL(memoryDir)
		if aclErr != nil {
			*risks = append(*risks, core.Risk{
				ID:          "memory_dir_perm_unsafe",
				Title:       "记忆目录权限不安全",
				Description: fmt.Sprintf("memory 目录 ACL 校验失败: %v", aclErr),
				Level:       core.RiskLevelCritical,
				Args:        map[string]interface{}{"path": memoryDir, "acl_summary": "acl check failed"},
			})
		} else if !dirACL.Safe {
			*risks = append(*risks, core.Risk{
				ID:          "memory_dir_perm_unsafe",
				Title:       "记忆目录权限不安全",
				Description: fmt.Sprintf("memory 目录 ACL 不安全: %s", dirACL.Summary),
				Level:       core.RiskLevelCritical,
				Args: map[string]interface{}{
					"path":           memoryDir,
					"acl_summary":    dirACL.Summary,
					"acl_violations": dirACL.Violations,
				},
			})
		}
		return
	}

	dirPerm := info.Mode().Perm()
	if dirPerm != 0700 {
		*risks = append(*risks, core.Risk{
			ID:          "memory_dir_perm_unsafe",
			Title:       "记忆目录权限不安全",
			Description: fmt.Sprintf("memory 目录当前权限为 %o，期望为 700", dirPerm),
			Level:       core.RiskLevelCritical,
			Args:        map[string]interface{}{"path": memoryDir, "current": fmt.Sprintf("%o", dirPerm)},
		})
	}
}

// checkProcessNotRoot 检查 DinTalClaw 相关进程是否以 root 运行（检测目标进程 UID，非当前 Go 进程）
func checkProcessNotRoot(risks *[]core.Risk) {
	if runtime.GOOS == "windows" {
		return
	}

	if dintalclawAnyPIDRunsAsRoot() {
		*risks = append(*risks, core.Risk{
			ID:          "process_running_as_root",
			Title:       "进程以 root 身份运行",
			Description: "检测到 DinTalClaw 相关进程以 root 用户运行，权限过高存在安全风险",
			Level:       core.RiskLevelHigh,
		})
	}
}

// checkLogDirPermissions 检查日志目录权限
func checkLogDirPermissions(installRoot string, risks *[]core.Risk) {
	logDir := filepath.Join(installRoot, "temp")
	info, err := os.Stat(logDir)
	if err != nil {
		return
	}

	if runtime.GOOS == "windows" {
		logACL, aclErr := checkWindowsACL(logDir)
		if aclErr != nil {
			*risks = append(*risks, core.Risk{
				ID:          "log_dir_perm_unsafe",
				Title:       "日志目录权限不安全",
				Description: fmt.Sprintf("日志目录 ACL 校验失败: %v", aclErr),
				Level:       core.RiskLevelCritical,
				Args:        map[string]interface{}{"path": logDir, "acl_summary": "acl check failed"},
			})
		} else if !logACL.Safe {
			*risks = append(*risks, core.Risk{
				ID:          "log_dir_perm_unsafe",
				Title:       "日志目录权限不安全",
				Description: fmt.Sprintf("日志目录 ACL 不安全: %s", logACL.Summary),
				Level:       core.RiskLevelCritical,
				Args: map[string]interface{}{
					"path":           logDir,
					"acl_summary":    logACL.Summary,
					"acl_violations": logACL.Violations,
				},
			})
		}
		return
	}

	dirPerm := info.Mode().Perm()
	if dirPerm != 0700 {
		*risks = append(*risks, core.Risk{
			ID:          "log_dir_perm_unsafe",
			Title:       "日志目录权限不安全",
			Description: fmt.Sprintf("日志目录当前权限为 %o，期望为 700", dirPerm),
			Level:       core.RiskLevelMedium,
			Args:        map[string]interface{}{"path": logDir},
		})
	}
}

// checkCredentialsInConfig 检测 mykey.py 中的明文敏感信息
func checkCredentialsInConfig(configPath string, risks *[]core.Risk) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	patterns := []string{"sk-", "ghp_", "ghu_", "Bearer ", "AWS_ACCESS_KEY_ID"}
	for _, p := range patterns {
		if strings.Contains(string(content), p) {
			*risks = append(*risks, core.Risk{
				ID:          "plaintext_secrets",
				Title:       "Plaintext Secrets Detected in Config",
				Description: fmt.Sprintf("Found potential secret matching pattern '%s' in mykey.py", p),
				Level:       core.RiskLevelCritical,
				Args:        map[string]interface{}{"pattern": p},
			})
			break
		}
	}
}
