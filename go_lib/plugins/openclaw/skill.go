package openclaw

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
)

// getSkillsDirs returns all skill directories to scan (bot-specific, depends on openclaw config)
func getSkillsDirs() ([]string, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, err
	}

	configDir := filepath.Dir(configPath)
	config, _, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	dirs := []string{
		filepath.Join(configDir, "skills"),
		filepath.Join(configDir, "workspace", "skills"),
	}

	if workspace := strings.TrimSpace(config.Agents.Defaults.Workspace); workspace != "" {
		dirs = append(dirs, filepath.Join(expandSkillDir(workspace), "skills"))
	}
	for _, extraDir := range config.Skills.Load.ExtraDirs {
		if expanded := strings.TrimSpace(expandSkillDir(extraDir)); expanded != "" {
			dirs = append(dirs, expanded)
		}
	}

	seen := make(map[string]struct{})
	result := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absDir, err := filepath.Abs(dir)
		if err != nil {
			absDir = dir
		}
		key := strings.ToLower(strings.TrimSpace(absDir))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, absDir)
	}

	return result, nil
}

func expandSkillDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

// isWithinSkillsDirs checks whether a path is inside a valid skill directory
func isWithinSkillsDirs(targetPath string) bool {
	dirs, err := getSkillsDirs()
	if err != nil {
		return false
	}
	absTarget, _ := filepath.Abs(targetPath)
	for _, dir := range dirs {
		absDir, _ := filepath.Abs(dir)
		if strings.HasPrefix(absTarget, absDir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// listSkills lists skills from all bot skill directories
func listSkills() ([]SkillInfo, error) {
	skillsDirs, err := getSkillsDirs()
	if err != nil {
		return nil, err
	}

	var skills []SkillInfo
	for _, skillsDir := range skillsDirs {
		dirSkills, err := listSkillsInDir(skillsDir)
		if err != nil {
			continue
		}
		skills = append(skills, dirSkills...)
	}

	return skills, nil
}

// checkUnscannedSkills checks for unscanned skills and adds risks.
// Uses a two-tier check: first by skill_name in DB (robust), then by hash match.
// A skill with a DB record (by name) but mismatched hash means it was modified since
// last scan; a skill with no DB record at all was never scanned.
func checkUnscannedSkills(scannedHashes map[string]bool, risks *[]core.Risk) {
	logging.Info("[checkUnscannedSkills] scannedHashes count: %d", len(scannedHashes))
	skills, err := listSkills()
	if err != nil {
		return
	}

	// Query all successfully scanned skill names from DB for name-based lookup.
	// Error scans (risk_level='error') are excluded so they can be retried.
	scanRepo := repository.NewSkillSecurityScanRepository(nil)
	scannedNames := make(map[string]bool)
	if records, err := scanRepo.GetAllSkillScans(); err == nil {
		for _, r := range records {
			if r.RiskLevel != "error" {
				scannedNames[r.SkillName] = true
			}
		}
	}

	var unscannedSkills []string
	// skillName -> absolute path for Flutter to use
	skillPaths := make(map[string]string)
	for _, skill := range skills {
		if skill.HasSkillMd && skill.Hash != "" {
			// Primary check: hash-based (exact version match)
			if _, ok := scannedHashes[skill.Hash]; ok {
				continue
			}
			// Fallback: name-based (skill was scanned before but content changed)
			if scannedNames[skill.Name] {
				logging.Info("[checkUnscannedSkills] Skill %s hash changed but has a previous scan record, skipping", skill.Name)
				continue
			}
			logging.Warning("[checkUnscannedSkills] Skill %s is unscanned (hash=%s...)", skill.Name, skill.Hash[:min(12, len(skill.Hash))])
			unscannedSkills = append(unscannedSkills, skill.Name)
			skillPaths[skill.Name] = skill.Path
		}
	}

	if len(unscannedSkills) > 0 {
		*risks = append(*risks, core.Risk{
			ID:          "skills_not_scanned",
			Title:       "Skills Not Scanned for Prompt Injection",
			Description: fmt.Sprintf("%d skill(s) have not been scanned for prompt injection risks: %s", len(unscannedSkills), strings.Join(unscannedSkills, ", ")),
			Level:       core.RiskLevelMedium,
			Args: map[string]interface{}{
				"count":       len(unscannedSkills),
				"skills":      strings.Join(unscannedSkills, ", "),
				"skill_names": unscannedSkills,
				"skill_paths": skillPaths,
			},
		})
	}
}
