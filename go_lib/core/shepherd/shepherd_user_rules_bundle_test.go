package shepherd

import (
	"os"
	"path/filepath"
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
	if len(rules.SemanticRules) == 0 {
		t.Fatal("expected bundled user rules to contain semantic rules")
	}
}

func TestSaveUserRulesPersistsToJSON(t *testing.T) {
	targetRoot := t.TempDir()
	rulesFile, err := ensureBundledShepherdRulesReleased(targetRoot)
	if err != nil {
		t.Fatalf("release bundled user rules failed: %v", err)
	}

	if err := saveUserRulesToFile(rulesFile, &UserRules{
		SemanticRules: []SemanticRule{
			{ID: "delete_file", Enabled: true, Description: "不允许删除文件"},
			{ID: "send_email", Enabled: true, Description: "不允许发送邮件"},
			{ID: "delete_file", Enabled: true, Description: "duplicate"},
			{ID: " ", Enabled: true},
		},
	}); err != nil {
		t.Fatalf("saveUserRulesToFile failed: %v", err)
	}

	loaded, err := loadUserRulesFromFile(rulesFile)
	if err != nil {
		t.Fatalf("load persisted user rules failed: %v", err)
	}

	if len(loaded.SemanticRules) != 2 {
		t.Fatalf("unexpected persisted semantic rules: got=%v", loaded.SemanticRules)
	}
	if loaded.SemanticRules[0].ID != "delete_file" || loaded.SemanticRules[1].ID != "send_email" {
		t.Fatalf("unexpected persisted semantic rule order: got=%v", loaded.SemanticRules)
	}
}
