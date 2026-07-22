//go:build windows

package dwtsclaw

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func discoverPlatformInstallRoots() []string {
	roots := make([]string, 0, 6)
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, keyPath := range []string{
			`Software\Microsoft\Windows\CurrentVersion\Uninstall\com.dwtsclaw.app`,
			`Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\com.dwtsclaw.app`,
			`Software\Microsoft\Windows\CurrentVersion\App Paths\DWTSClaw.exe`,
		} {
			key, err := registry.OpenKey(hive, keyPath, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			for _, valueName := range []string{"InstallLocation", "Path", "", "DisplayIcon"} {
				value, _, err := key.GetStringValue(valueName)
				if err == nil {
					if root := installRootFromRegistryValue(value); root != "" {
						roots = append(roots, root)
					}
				}
			}
			_ = key.Close()
		}
	}
	return roots
}

func installRootFromRegistryValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, `"`) {
		if end := strings.Index(value[1:], `"`); end >= 0 {
			value = value[1 : end+1]
		}
	}
	value = strings.TrimSpace(strings.Split(value, ",")[0])
	if strings.EqualFold(filepath.Base(value), "DWTSClaw.exe") {
		return filepath.Dir(value)
	}
	return value
}
