//go:build linux

package dintalclaw

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// dintalclawAnyPIDRunsAsRoot 判断任一 DinTalClaw 相关进程是否以 root(UID 0) 运行
func dintalclawAnyPIDRunsAsRoot() bool {
	for _, pid := range findDintalclawPIDs() {
		uid, ok := linuxProcRealUID(pid)
		if ok && uid == 0 {
			return true
		}
	}
	return false
}

// linuxProcRealUID 从 /proc/<pid>/status 读取真实 UID（Uid 行首字段）
func linuxProcRealUID(pid int) (int, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		uid, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, false
		}
		return uid, true
	}
	return 0, false
}
