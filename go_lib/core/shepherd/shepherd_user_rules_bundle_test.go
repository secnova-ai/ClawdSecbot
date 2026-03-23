package shepherd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnsureBundledShepherdRulesReleased(t *testing.T) {
	targetRoot := t.TempDir()

	rulesFile, err := ensureBundledShepherdRulesReleased(targetRoot)
	if err != nil {
		t.Fatalf("release bundled user rules failed: %v", err)
	}

	requiredFiles := []string{
		rulesFile,
		filepath.Join(filepath.Dir(rulesFile), bundledShepherdRulesVersionFile),
	}
	for _, file := range requiredFiles {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("required user rules file not found: %s, err=%v", file, err)
		}
	}

	rules, err := loadUserRulesFromFile(rulesFile)
	if err != nil {
		t.Fatalf("failed to load released user rules: %v", err)
	}
	if len(rules.SensitiveActions) == 0 {
		t.Fatal("expected bundled user rules to contain sensitive actions")
	}
}

func TestSaveUserRulesPersistsToJSON(t *testing.T) {
	targetRoot := t.TempDir()
	rulesFile, err := ensureBundledShepherdRulesReleased(targetRoot)
	if err != nil {
		t.Fatalf("release bundled user rules failed: %v", err)
	}

	input := []string{"delete_file", "send_email", "delete_file", " "}
	if err := saveUserRulesToFile(rulesFile, &UserRules{
		SensitiveActions: normalizeSensitiveActions(input),
	}); err != nil {
		t.Fatalf("saveUserRulesToFile failed: %v", err)
	}

	loaded, err := loadUserRulesFromFile(rulesFile)
	if err != nil {
		t.Fatalf("load persisted user rules failed: %v", err)
	}

	expected := []string{"delete_file", "send_email"}
	if !reflect.DeepEqual(loaded.SensitiveActions, expected) {
		t.Fatalf("unexpected persisted sensitive actions: got=%v want=%v", loaded.SensitiveActions, expected)
	}
}
