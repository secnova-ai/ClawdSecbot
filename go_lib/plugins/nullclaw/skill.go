package nullclaw

import (
	"fmt"
	"path/filepath"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
)

// getSkillsDirs returns all skill directories to scan (bot-specific, depends on nullclaw config)
func getSkillsDirs() ([]string, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, err
	}

	configDir := filepath.Dir(configPath)
	return []string{
		filepath.Join(configDir, "skills"),
		filepath.Join(configDir, "workspace", "skills"),
	}, nil
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
// Uses hash-only matching so the UI risk aligns with GetScannedSkillHashes:
// any skill whose current content hash is not in scannedHashes is treated as unscanned.
func checkUnscannedSkills(scannedHashes map[string]bool, risks *[]core.Risk) {
	logging.Info("[checkUnscannedSkills] scannedHashes count: %d", len(scannedHashes))
	skills, err := listSkills()
	if err != nil {
		return
	}

	var unscannedSkills []string
	// skillName -> absolute path for Flutter to use
	skillPaths := make(map[string]string)
	for _, skill := range skills {
		if skill.HasSkillMd && skill.Hash != "" {
			if _, ok := scannedHashes[skill.Hash]; ok {
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
