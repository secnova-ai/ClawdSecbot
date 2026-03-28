package nullclaw

import (
	"fmt"
	"os"
)

// removeSkillDirectory safely removes a skill directory.
// Keep this helper in a non-cgo file so both cgo and non-cgo builds can use it.
func removeSkillDirectory(skillPath string) error {
	info, err := os.Stat(skillPath)
	if err != nil {
		return fmt.Errorf("skill path not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("skill path is not a directory")
	}
	return os.RemoveAll(skillPath)
}
