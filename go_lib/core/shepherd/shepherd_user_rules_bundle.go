package shepherd

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go_lib/core/logging"
)

const (
	bundledShepherdRulesEmbedRoot      = "bundled_shepherd_rules"
	bundledShepherdRulesReleaseDirName = "user_rules"
	bundledShepherdRulesVersionFile    = ".bundle.version"
	bundledShepherdRulesFileName       = "user_rules.json"
)

//go:embed bundled_shepherd_rules/*
var bundledShepherdRulesFS embed.FS

func resolveDefaultShepherdRulesRoot() string {
	return resolveDefaultReActSkillsRoot()
}

func ensureBundledShepherdRulesReleased(targetRoot string) (string, error) {
	if strings.TrimSpace(targetRoot) == "" {
		targetRoot = resolveDefaultShepherdRulesRoot()
	}
	releaseDir := filepath.Join(targetRoot, bundledShepherdRulesReleaseDirName)
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		return "", fmt.Errorf("create user rules release dir failed: %w", err)
	}

	desiredVersion, err := calculateBundledShepherdRulesVersion()
	if err != nil {
		return "", err
	}

	rulesFile := filepath.Join(releaseDir, bundledShepherdRulesFileName)
	versionFile := filepath.Join(releaseDir, bundledShepherdRulesVersionFile)
	currentVersion, _ := os.ReadFile(versionFile)
	if strings.TrimSpace(string(currentVersion)) == desiredVersion {
		if _, statErr := os.Stat(rulesFile); statErr == nil {
			return rulesFile, nil
		}
	}

	if err := os.RemoveAll(releaseDir); err != nil {
		return "", fmt.Errorf("reset user rules release dir failed: %w", err)
	}
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		return "", fmt.Errorf("recreate user rules release dir failed: %w", err)
	}

	if err := fs.WalkDir(bundledShepherdRulesFS, bundledShepherdRulesEmbedRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(bundledShepherdRulesEmbedRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		targetPath := filepath.Join(releaseDir, rel)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := bundledShepherdRulesFS.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0644)
	}); err != nil {
		return "", fmt.Errorf("release bundled shepherd rules failed: %w", err)
	}

	if err := os.WriteFile(versionFile, []byte(desiredVersion), 0644); err != nil {
		return "", fmt.Errorf("write user rules bundle version failed: %w", err)
	}

	logging.Info("[ShepherdGate] Bundled user rules released: file=%s, version=%s", rulesFile, desiredVersion)
	return rulesFile, nil
}

func calculateBundledShepherdRulesVersion() (string, error) {
	var records []string
	err := fs.WalkDir(bundledShepherdRulesFS, bundledShepherdRulesEmbedRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, err := bundledShepherdRulesFS.ReadFile(path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		records = append(records, path+"|"+hex.EncodeToString(hash[:]))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk bundled shepherd rules failed: %w", err)
	}

	sort.Strings(records)
	h := sha256.New()
	for _, r := range records {
		_, _ = h.Write([]byte(r))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func loadDefaultUserRules() (*UserRules, error) {
	rulesFile, err := ensureBundledShepherdRulesReleased("")
	if err != nil {
		return nil, err
	}

	rules, err := loadUserRulesFromFile(rulesFile)
	if err != nil {
		logging.Warning("[ShepherdGate] Failed to read user rules file, fallback to bundled defaults: %v", err)
		rules, err = loadBundledDefaultUserRules()
		if err != nil {
			return nil, err
		}
		if saveErr := saveUserRulesToFile(rulesFile, rules); saveErr != nil {
			logging.Warning("[ShepherdGate] Failed to repair user rules file: %v", saveErr)
		}
	}

	logging.Info("ShepherdGate: Default user rules loaded. Semantic rules: %d", len(rules.SemanticRules))
	return cloneUserRules(rules), nil
}

// GetDefaultUserRules returns bundled default user rules.
func GetDefaultUserRules() (*UserRules, error) {
	return loadDefaultUserRules()
}

func loadBundledDefaultUserRules() (*UserRules, error) {
	path := filepath.Join(bundledShepherdRulesEmbedRoot, bundledShepherdRulesFileName)
	data, err := bundledShepherdRulesFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bundled user rules failed: %w", err)
	}
	return decodeUserRules(data)
}

func loadUserRulesFromFile(path string) (*UserRules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read user rules file failed: %w", err)
	}
	return decodeUserRules(data)
}

func decodeUserRules(data []byte) (*UserRules, error) {
	type alias struct {
		SemanticRules []SemanticRule `json:"semantic_rules"`
	}

	var payload alias
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse user rules JSON failed: %w", err)
	}

	return normalizeUserRules(&UserRules{
		SemanticRules: payload.SemanticRules,
	}), nil
}

// DecodeUserRulesJSON parses structured Shepherd rules from JSON bytes.
func DecodeUserRulesJSON(data []byte) (*UserRules, error) {
	return decodeUserRules(data)
}

func saveUserRulesToFile(path string, rules *UserRules) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("user rules file path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create user rules dir failed: %w", err)
	}

	payload := cloneUserRules(rules)
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal user rules failed: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write temp user rules failed: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace user rules failed: %w", err)
	}
	return nil
}

func cloneUserRules(rules *UserRules) *UserRules {
	return normalizeUserRules(rules)
}

func normalizeUserRules(rules *UserRules) *UserRules {
	if rules == nil {
		return &UserRules{SemanticRules: []SemanticRule{}}
	}
	return &UserRules{
		SemanticRules: normalizeSemanticRules(rules.SemanticRules),
	}
}

func normalizeSemanticRules(rules []SemanticRule) []SemanticRule {
	normalized := make([]SemanticRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Scope = strings.TrimSpace(rule.Scope)
		rule.Description = strings.TrimSpace(rule.Description)
		rule.Action = strings.TrimSpace(rule.Action)
		rule.RiskType = strings.TrimSpace(rule.RiskType)
		if rule.ID == "" && rule.Description == "" {
			continue
		}
		key := rule.ID
		if key == "" {
			key = rule.Description
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		rule.AppliesTo = normalizeStringList(rule.AppliesTo)
		rule.OWASPAgentic = normalizeStringList(rule.OWASPAgentic)
		normalized = append(normalized, rule)
	}
	return normalized
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
