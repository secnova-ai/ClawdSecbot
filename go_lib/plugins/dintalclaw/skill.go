package dintalclaw

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
)

// getSkillsDirs 返回 dintalclaw 技能扫描目录
// 技能目录为 <install_root>/memory（安装根下 memory，非 temp）
func getSkillsDirs() ([]string, error) {
	root := findInstallRoot()
	if root == "" {
		return nil, fmt.Errorf("dintalclaw install root not found")
	}

	return []string{
		filepath.Join(root, "memory"),
	}, nil
}

// isWithinSkillsDirs 检查路径是否位于合法的技能目录内
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

// listSkills 列举所有技能目录中的技能
func listSkills() ([]SkillInfo, error) {
	skillsDirs, err := getSkillsDirs()
	if err != nil {
		return nil, err
	}

	var skills []SkillInfo
	for _, skillsDir := range skillsDirs {
		dirSkills, err := listDintalclawMemoryFiles(skillsDir)
		if err != nil {
			continue
		}
		skills = append(skills, dirSkills...)
	}

	return skills, nil
}

// listDintalclawMemoryFiles 列举 memory 目录下所有非 .txt 文件作为技能项
func listDintalclawMemoryFiles(memoryDir string) ([]SkillInfo, error) {
	if _, err := os.Stat(memoryDir); os.IsNotExist(err) {
		return nil, nil
	}

	var skills []SkillInfo
	err := filepath.Walk(memoryDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext == ".txt" {
			return nil
		}

		hash, hashErr := calculateFileHash(path)
		if hashErr != nil {
			hash = ""
		}

		relPath, relErr := filepath.Rel(memoryDir, path)
		if relErr != nil {
			relPath = info.Name()
		}

		// 对于 dintalclaw，非 .txt 文件即有效技能，HasSkillMd 恒为 true 以复用既有流程。
		skills = append(skills, SkillInfo{
			Name:       relPath,
			Path:       path,
			Hash:       hash,
			Scanned:    false,
			HasSkillMd: true,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return skills, nil
}

// calculateFileHash 计算单文件 SHA-256，用于 dintalclaw 文件型技能去重
func calculateFileHash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// checkUnscannedSkills 检查未扫描的技能，添加风险项
// 仅按 hash 与 scannedHashes 对齐，与 GetScannedSkillHashes 主口径一致。
func checkUnscannedSkills(scannedHashes map[string]bool, risks *[]core.Risk) {
	logging.Info("[checkUnscannedSkills] scannedHashes count: %d", len(scannedHashes))
	skills, err := listSkills()
	if err != nil {
		return
	}

	var unscannedSkills []string
	skillPaths := make(map[string]string)
	for _, skill := range skills {
		if skill.Hash == "" {
			continue
		}
		if _, ok := scannedHashes[skill.Hash]; ok {
			continue
		}
		logging.Warning("[checkUnscannedSkills] Skill %s is unscanned (hash=%s...)", skill.Name, skill.Hash[:min(12, len(skill.Hash))])
		unscannedSkills = append(unscannedSkills, skill.Name)
		skillPaths[skill.Name] = skill.Path
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
