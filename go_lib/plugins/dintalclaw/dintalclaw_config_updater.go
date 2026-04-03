package dintalclaw

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go_lib/core/logging"
)

const InitialBackupFileName = "mykey.py.initial"

// backupDintalclawConfigOnce 初始备份（只在第一次时执行）
func backupDintalclawConfigOnce(configPath string, backupDir string) (string, bool, error) {
	if strings.TrimSpace(backupDir) == "" {
		return "", false, fmt.Errorf("backup directory is empty")
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", false, fmt.Errorf("create backup directory failed: %w", err)
	}

	initialBackupPath := filepath.Join(backupDir, InitialBackupFileName)
	if _, err := os.Stat(initialBackupPath); err == nil {
		return initialBackupPath, false, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", false, fmt.Errorf("read config failed: %w", err)
	}
	if err := os.WriteFile(initialBackupPath, data, 0600); err != nil {
		return "", false, fmt.Errorf("write initial backup failed: %w", err)
	}

	return initialBackupPath, true, nil
}

func hasInitialBackup(backupDir string) bool {
	if strings.TrimSpace(backupDir) == "" {
		return false
	}
	initialBackupPath := filepath.Join(backupDir, InitialBackupFileName)
	_, err := os.Stat(initialBackupPath)
	return err == nil
}

func getInitialBackupPath(backupDir string) string {
	return filepath.Join(backupDir, InitialBackupFileName)
}

// backupDintalclawConfig 备份配置文件（兼容旧接口）
func backupDintalclawConfig(configPath string, backupDir string) (string, error) {
	backupPath, _, err := backupDintalclawConfigOnce(configPath, backupDir)
	return backupPath, err
}

// restoreDintalclawConfig 从备份恢复配置
func restoreDintalclawConfig(configPath string, backupPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup failed: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("restore config failed: %w", err)
	}
	return nil
}

// RestoreToInitialConfigResult 恢复初始配置的结果
type RestoreToInitialConfigResult struct {
	Success        bool   `json:"success"`
	Message        string `json:"message,omitempty"`
	Error          string `json:"error,omitempty"`
	ConfigPath     string `json:"config_path,omitempty"`
	BackupPath     string `json:"backup_path,omitempty"`
	ProcessRestart bool   `json:"process_restart,omitempty"`
}

// RestoreToInitialConfigByAsset 恢复 mykey.py 到初始状态并重启进程
func RestoreToInitialConfigByAsset(backupDir string, assetID string) RestoreToInitialConfigResult {
	logging.Info("[RestoreToInitialConfig] Starting restore to initial config...")

	if !hasInitialBackup(backupDir) {
		return RestoreToInitialConfigResult{Success: false, Error: "initial backup not found"}
	}

	configPath, err := findConfigPathForDintalclaw()
	if err != nil {
		return RestoreToInitialConfigResult{Success: false, Error: fmt.Sprintf("config not found: %v", err)}
	}

	initialBackupPath := getInitialBackupPath(backupDir)
	if err := restoreDintalclawConfig(configPath, initialBackupPath); err != nil {
		return RestoreToInitialConfigResult{
			Success:    false,
			Error:      fmt.Sprintf("restore failed: %v", err),
			ConfigPath: configPath,
			BackupPath: initialBackupPath,
		}
	}
	logging.Info("[RestoreToInitialConfig] Config restored from %s to %s", initialBackupPath, configPath)

	req := &GatewayRestartRequest{
		AssetName:      dintalclawAssetName,
		AssetID:        strings.TrimSpace(assetID),
		SandboxEnabled: false,
	}

	_, processErr := restartDintalclawProcess(req)
	processRestart := processErr == nil
	if processErr != nil {
		logging.Warning("[RestoreToInitialConfig] Process restart failed: %v", processErr)
	}

	return RestoreToInitialConfigResult{
		Success:        true,
		Message:        "config restored to initial state",
		ConfigPath:     configPath,
		BackupPath:     initialBackupPath,
		ProcessRestart: processRestart,
	}
}

// RestoreToInitialConfig 恢复到初始备份（兼容无 assetID 接口）
func RestoreToInitialConfig(backupDir string) RestoreToInitialConfigResult {
	return RestoreToInitialConfigByAsset(backupDir, "")
}

// HasInitialBackup 检查初始备份是否存在
func HasInitialBackup(backupDir string) bool {
	return hasInitialBackup(backupDir)
}
