package qclaw

import (
	"fmt"
	"path/filepath"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/core/skillscan"
	openclawplugin "go_lib/plugins/openclaw"
)

func getSkillsDirs() ([]string, error) {
	_, _, configPath, err := loadConfig()
	if err != nil {
		return nil, err
	}

	dirs := []string{
		filepath.Join(filepath.Dir(configPath), "skills"),
	}

	if state, err := loadQClawRuntimeState(); err == nil && state != nil {
		if openclawMjs := strings.TrimSpace(state.CLI.OpenclawMjs); openclawMjs != "" {
			runtimeRoot := filepath.Dir(filepath.Dir(filepath.Dir(openclawMjs)))
			dirs = append(dirs, filepath.Join(runtimeRoot, "config", "skills"))
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

func listSkills() ([]skillscan.SkillInfo, error) {
	skillDirs, err := getSkillsDirs()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	skills := make([]skillscan.SkillInfo, 0)
	for _, dir := range skillDirs {
		dirSkills, err := skillscan.ListSkillsInDir(dir)
		if err != nil {
			logging.Warning("[QClaw] Failed to list skills in %s: %v", dir, err)
			continue
		}
		for _, skill := range dirSkills {
			key := strings.ToLower(strings.TrimSpace(skill.Path))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			skills = append(skills, skill)
		}
	}

	return skills, nil
}

func buildUnscannedSkillsRisk(scannedHashes map[string]bool) *core.Risk {
	logging.Info("[QClaw] Checking unscanned skills, scanned hash count=%d", len(scannedHashes))

	skills, err := listSkills()
	if err != nil {
		logging.Warning("[QClaw] Failed to list skills for risk assessment: %v", err)
		return nil
	}

	scanRepo := repository.NewSkillSecurityScanRepository(nil)
	scannedNames := make(map[string]bool)
	if records, err := scanRepo.GetAllSkillScans(); err == nil {
		for _, record := range records {
			if record.RiskLevel != "error" {
				scannedNames[record.SkillName] = true
			}
		}
	}

	unscannedSkills := make([]string, 0)
	skillPaths := make(map[string]string)
	for _, skill := range skills {
		if !skill.HasSkillMd || skill.Hash == "" {
			continue
		}
		if scannedHashes[skill.Hash] {
			continue
		}
		if scannedNames[skill.Name] {
			continue
		}

		unscannedSkills = append(unscannedSkills, skill.Name)
		skillPaths[skill.Name] = skill.Path
	}

	if len(unscannedSkills) == 0 {
		return nil
	}

	return &core.Risk{
		ID:          "skills_not_scanned",
		Title:       "Skills Not Scanned for Prompt Injection",
		Description: fmt.Sprintf("%d skill(s) have not been scanned for prompt injection risks: %s", len(unscannedSkills), strings.Join(unscannedSkills, ", ")),
		Level:       core.RiskLevelMedium,
		Mitigation:  openclawplugin.GetMitigationTemplates()["skills_not_scanned"],
		Args: map[string]interface{}{
			"count":       len(unscannedSkills),
			"skills":      strings.Join(unscannedSkills, ", "),
			"skill_names": unscannedSkills,
			"skill_paths": skillPaths,
		},
	}
}
