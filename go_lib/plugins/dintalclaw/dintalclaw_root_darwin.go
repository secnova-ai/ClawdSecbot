//go:build darwin

package dintalclaw

import (
	"os/exec"
	"strconv"
	"strings"
)

// dintalclawAnyPIDRunsAsRoot 判断任一 DinTalClaw 相关进程是否以 root(UID 0) 运行
func dintalclawAnyPIDRunsAsRoot() bool {
	for _, pid := range findDintalclawPIDs() {
		uid, ok := darwinProcUID(pid)
		if ok && uid == 0 {
			return true
		}
	}
	return false
}

// darwinProcUID 通过 ps 读取进程有效 UID
func darwinProcUID(pid int) (int, bool) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "uid=").Output()
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	uid, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return uid, true
}
