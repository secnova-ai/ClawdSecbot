package shepherd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureBundledReActSkillsReleased(t *testing.T) {
	targetRoot := t.TempDir()

	releaseDir, err := ensureBundledReActSkillsReleased(targetRoot)
	if err != nil {
		t.Fatalf("release bundled skills failed: %v", err)
	}

	requiredFiles := []string{
		filepath.Join(releaseDir, "command_execution_guard", "SKILL.md"),
		filepath.Join(releaseDir, "general_tool_risk_guard", "SKILL.md"),
		filepath.Join(releaseDir, "email_operation_guard", "SKILL.md"),
		filepath.Join(releaseDir, "script_execution_guard", "SKILL.md"),
		filepath.Join(releaseDir, "data_exfiltration_guard", "SKILL.md"),
		filepath.Join(releaseDir, "browser_web_access_guard", "SKILL.md"),
		filepath.Join(releaseDir, "file_access_guard", "SKILL.md"),
		filepath.Join(releaseDir, "supply_chain_guard", "SKILL.md"),
		filepath.Join(releaseDir, "persistence_backdoor_guard", "SKILL.md"),
		filepath.Join(releaseDir, "lateral_movement_guard", "SKILL.md"),
		filepath.Join(releaseDir, "resource_exhaustion_guard", "SKILL.md"),
		filepath.Join(releaseDir, "skill_installation_guard", "SKILL.md"),
		filepath.Join(releaseDir, bundledSkillsVersionFile),
	}
	for _, file := range requiredFiles {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("required released file not found: %s, err=%v", file, err)
		}
	}
}

func TestEnsureBundledReActSkillsReleased_Idempotent(t *testing.T) {
	targetRoot := t.TempDir()

	releaseDir1, err := ensureBundledReActSkillsReleased(targetRoot)
	if err != nil {
		t.Fatalf("first release failed: %v", err)
	}
	version1, err := os.ReadFile(filepath.Join(releaseDir1, bundledSkillsVersionFile))
	if err != nil {
		t.Fatalf("read version 1 failed: %v", err)
	}

	releaseDir2, err := ensureBundledReActSkillsReleased(targetRoot)
	if err != nil {
		t.Fatalf("second release failed: %v", err)
	}
	version2, err := os.ReadFile(filepath.Join(releaseDir2, bundledSkillsVersionFile))
	if err != nil {
		t.Fatalf("read version 2 failed: %v", err)
	}

	if releaseDir1 != releaseDir2 {
		t.Fatalf("expected same release dir, got %s vs %s", releaseDir1, releaseDir2)
	}
	if string(version1) != string(version2) {
		t.Fatalf("expected same bundle version across repeated release")
	}
}
