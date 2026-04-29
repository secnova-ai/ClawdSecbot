package skillscan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go_lib/core/logging"
	"go_lib/core/modelfactory"
	"go_lib/core/repository"
	"go_lib/skillagent"

	"github.com/cloudwego/eino/components/model"
)

// Keyword patterns for pre-analysis skill matching
var skillKeywordPatterns = map[string][]string{
	"script_execution_analysis": {
		".sh", ".py", ".js", ".ps1", "bash", "python", "node",
		"exec", "system", "subprocess", "child_process", "eval",
	},
	"data_exfiltration_analysis": {
		"curl", "wget", "fetch", "http", "https", "ftp", "scp", "rsync",
		"upload", "post", "put", "send", "exfil", "dns",
	},
	"obfuscation_evasion_analysis": {
		"base64", "hex", "encode", "decode", "gzip", "compress", "obfuscat",
		"rot13", "xor", "pack", "eval(", "atob", "btoa",
	},
	"dependency_supply_chain_analysis": {
		"package.json", "requirements.txt", "go.mod", "gemfile", "pom.xml",
		"build.gradle", "cargo.toml", "npm", "pip", "install",
	},
	"social_engineering_trap_analysis": {
		"install", "setup", "configure", "tutorial", "guide", "step",
		"follow", "run this", "execute", "sudo", "chmod", "permission",
	},
}

// Max file size for pre-analysis (2MB)
const preAnalysisMaxFileSize = 2 * 1024 * 1024

// All available scan skills
var allScanSkills = []string{
	"script_execution_analysis",
	"data_exfiltration_analysis",
	"obfuscation_evasion_analysis",
	"dependency_supply_chain_analysis",
	"social_engineering_trap_analysis",
}

// SkillAnalysisResult represents the result of AI-based skill security analysis
type SkillAnalysisResult struct {
	Safe      bool                 `json:"safe"`
	RiskLevel string               `json:"risk_level"`
	Issues    []SkillSecurityIssue `json:"issues"`
	Summary   string               `json:"summary"`
	RawOutput string               `json:"raw_output,omitempty"`
}

// SkillSecurityIssue represents a security issue found during skill analysis
type SkillSecurityIssue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
}

// SkillSecurityAnalyzer is an AI-based skill security analyzer built on SkillAgent
type SkillSecurityAnalyzer struct {
	modelConfig   *repository.SecurityModelConfig
	chatModel     model.ChatModel
	language      string
	scanSkillsDir string // Released scanner skills directory
	mu            sync.Mutex
}

// NewSkillSecurityAnalyzer creates a SkillSecurityAnalyzer instance
func NewSkillSecurityAnalyzer(config *repository.SecurityModelConfig) (*SkillSecurityAnalyzer, error) {
	if err := modelfactory.ValidateSecurityModelConfig(config); err != nil {
		return nil, fmt.Errorf("invalid security model config: %w", err)
	}

	ctx := context.Background()
	chatModel, err := modelfactory.CreateChatModelFromConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model: %w", err)
	}

	// Release scanner skills to disk
	scanSkillsDir, err := EnsureScanSkillsReleased("")
	if err != nil {
		return nil, fmt.Errorf("failed to release scan skills: %w", err)
	}

	// Read language from app_settings
	lang := GetLanguageFromAppSettings()
	if lang == "" {
		lang = "en" // Default to English
	}

	return &SkillSecurityAnalyzer{
		modelConfig:   config,
		chatModel:     chatModel,
		language:      lang,
		scanSkillsDir: scanSkillsDir,
	}, nil
}

// AnalyzeSkill performs a complete analysis of a skill directory using SkillAgent
func (sa *SkillSecurityAnalyzer) AnalyzeSkill(ctx context.Context, skillPath string) (*SkillAnalysisResult, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	skillName := filepath.Base(skillPath)

	// Create target-scoped custom tools
	tools := GetSkillTools(skillPath)

	// Create SkillAgent
	agent, err := skillagent.NewSkillAgent(ctx, &skillagent.SkillAgentConfig{
		ChatModel:              sa.chatModel,
		SkillsDir:              sa.scanSkillsDir,
		CustomTools:            tools,
		SystemPromptPrefix:     GetSystemPromptWithLanguage(sa.language),
		MaxStep:                100,
		ExecutionTimeout:       10 * time.Minute,
		DisableFilesystemTools: true, // Scan mode: only allow scoped custom tools
		ValidateCommand: func(cmd string) error {
			return fmt.Errorf("command execution is not allowed in skill scanning mode")
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create skill agent: %w", err)
	}
	defer agent.Close()

	// Pre-analyze skill to determine recommended skills
	recommendedSkills := sa.preAnalyzeSkill(skillPath)
	userPrompt := buildUserPromptWithRecommendations(skillPath, skillName, recommendedSkills)

	// Execute
	result, err := agent.Execute(ctx, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("skill agent execution failed: %w", err)
	}

	// Parse output
	return parseAgentOutput(result.Output, skillPath)
}

// AnalyzeSkillStream performs streaming analysis of a skill directory using SkillAgent
func (sa *SkillSecurityAnalyzer) AnalyzeSkillStream(ctx context.Context, skillPath string, logChan chan<- string) (*SkillAnalysisResult, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	skillName := filepath.Base(skillPath)

	// Send initial log
	SendLog(logChan, fmt.Sprintf("Starting AI analysis of skill: %s", skillName))
	SendLog(logChan, fmt.Sprintf("skill directory: %s", skillPath))
	SendLog(logChan, fmt.Sprintf("Scanner skills directory: %s", sa.scanSkillsDir))

	// Create target-scoped custom tools
	tools := GetSkillTools(skillPath)

	// Log available custom tools
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		info, _ := t.Info(ctx)
		toolNames = append(toolNames, info.Name)
	}
	SendLog(logChan, fmt.Sprintf("Available custom tools: %v", toolNames))

	// Create SkillAgent
	SendLog(logChan, "Creating SkillAgent with scanner skills...")
	agent, err := skillagent.NewSkillAgent(ctx, &skillagent.SkillAgentConfig{
		ChatModel:              sa.chatModel,
		SkillsDir:              sa.scanSkillsDir,
		CustomTools:            tools,
		SystemPromptPrefix:     GetSystemPromptWithLanguage(sa.language),
		MaxStep:                100,
		ExecutionTimeout:       10 * time.Minute,
		DisableFilesystemTools: true, // Scan mode: only allow scoped custom tools
		ValidateCommand: func(cmd string) error {
			return fmt.Errorf("command execution is not allowed in skill scanning mode")
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create skill agent: %w", err)
	}
	defer agent.Close()

	// Pre-analyze skill to determine recommended skills
	recommendedSkills := sa.preAnalyzeSkill(skillPath)
	if len(recommendedSkills) > 0 {
		SendLog(logChan, fmt.Sprintf("Pre-analysis recommended skills: %v", recommendedSkills))
	}

	// List available scanner skills
	skills, err := agent.ListSkills(ctx)
	if err == nil && len(skills) > 0 {
		var skillNames []string
		for _, s := range skills {
			skillNames = append(skillNames, s.Name)
		}
		SendLog(logChan, fmt.Sprintf("Available scanner skills: %v", skillNames))
	}

	SendLog(logChan, "Agent is analyzing skill (this may take a while)...")

	// Build user prompt with recommendations
	userPrompt := buildUserPromptWithRecommendations(skillPath, skillName, recommendedSkills)

	// Execute agent
	result, err := agent.Execute(ctx, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("skill agent execution failed: %w", err)
	}

	// Send the response content to log
	if result.Output != "" {
		SendLog(logChan, result.Output)
	}

	SendLog(logChan, "\n\n--- Analysis Complete ---")

	// Parse the final result
	analysisResult, err := parseAgentOutput(result.Output, skillPath)
	if err != nil {
		SendLog(logChan, fmt.Sprintf("Warning: Failed to parse structured output: %v", err))
		// Return a basic result based on content analysis
		return &SkillAnalysisResult{
			Safe:      !containsRiskIndicators(result.Output),
			RiskLevel: "unknown",
			Summary:   "Analysis completed but structured output parsing failed",
			RawOutput: result.Output,
		}, nil
	}

	return analysisResult, nil
}

// SendLog sends a log message to the log channel with timeout protection
func SendLog(logChan chan<- string, msg string) {
	if logChan != nil {
		select {
		case logChan <- msg:
		case <-time.After(100 * time.Millisecond):
			// Skip if channel is full
		}
	}
}

func parseAgentOutput(output string, skillPath string) (*SkillAnalysisResult, error) {
	// Try to find all JSON blocks in the output, prefer the last one (usually final conclusion)
	jsonRegex := regexp.MustCompile("(?s)```json\\s*([\\s\\S]*?)\\s*```")
	allMatches := jsonRegex.FindAllStringSubmatch(output, -1)

	// Process from last to first, take the last valid JSON
	for i := len(allMatches) - 1; i >= 0; i-- {
		if len(allMatches[i]) >= 2 {
			var result SkillAnalysisResult
			if err := json.Unmarshal([]byte(allMatches[i][1]), &result); err == nil {
				result.RawOutput = output
				validateAnalysisEvidence(skillPath, &result)
				return &result, nil
			}
		}
	}

	// Try to find raw JSON objects with nested structure support
	jsonObjRegex := regexp.MustCompile(`(?s)\{[^{}]*(?:\{[^{}]*\}[^{}]*)*"safe"\s*:\s*(true|false)[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`)
	allObjMatches := jsonObjRegex.FindAllString(output, -1)

	// Try from last to first
	for i := len(allObjMatches) - 1; i >= 0; i-- {
		var result SkillAnalysisResult
		if err := json.Unmarshal([]byte(allObjMatches[i]), &result); err == nil {
			result.RawOutput = output
			validateAnalysisEvidence(skillPath, &result)
			return &result, nil
		}
	}

	// Check if output contains risk indicators even if JSON parsing failed
	hasRiskIndicators := containsRiskIndicators(output)
	riskLevel := inferRiskLevel(output)

	// If output contains risk info but JSON failed, don't default to safe
	if hasRiskIndicators || (riskLevel != "none" && riskLevel != "unknown") {
		result := &SkillAnalysisResult{
			Safe:      false,
			RiskLevel: riskLevel,
			Issues:    buildManualReviewIssues(skillPath),
			Summary:   extractSummary(output),
			RawOutput: output,
		}
		return result, nil
	}

	// Fallback: analyze the text content for risk indicators
	return &SkillAnalysisResult{
		Safe:      true,
		RiskLevel: riskLevel,
		Summary:   extractSummary(output),
		RawOutput: output,
	}, nil
}

func validateAnalysisEvidence(skillPath string, result *SkillAnalysisResult) {
	if result == nil || len(result.Issues) == 0 {
		return
	}

	filtered := make([]SkillSecurityIssue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		if isManualReviewIssue(issue) {
			filtered = append(filtered, issue)
			continue
		}
		if issueEvidenceExists(skillPath, issue) {
			filtered = append(filtered, issue)
			continue
		}
		logging.Warning(
			"skill analysis issue dropped because evidence was not found in target files: skill_path=%s, file=%s, type=%s",
			skillPath,
			issue.File,
			issue.Type,
		)
	}
	result.Issues = filtered

	if !result.Safe && len(result.Issues) == 0 {
		result.Safe = true
		result.RiskLevel = "none"
		result.Summary = "No verifiable security issue evidence was found in the target skill files."
	}
}

// ValidateStoredIssueStrings removes structured issues whose evidence cannot be
// found in the current skill files. Legacy plain-text issue strings are kept
// because they do not carry machine-checkable evidence.
func ValidateStoredIssueStrings(skillPath string, issues []string) ([]string, int) {
	if len(issues) == 0 {
		return []string{}, 0
	}

	filtered := make([]string, 0, len(issues))
	dropped := 0
	for _, issueText := range issues {
		trimmed := strings.TrimSpace(issueText)
		if trimmed == "" {
			continue
		}
		var issue SkillSecurityIssue
		if err := json.Unmarshal([]byte(trimmed), &issue); err != nil {
			filtered = append(filtered, issueText)
			continue
		}
		if isManualReviewIssue(issue) {
			filtered = append(filtered, issueText)
			continue
		}
		if issueEvidenceExists(skillPath, issue) {
			filtered = append(filtered, issueText)
			continue
		}
		dropped++
	}
	if filtered == nil {
		filtered = []string{}
	}
	return filtered, dropped
}

func isManualReviewIssue(issue SkillSecurityIssue) bool {
	return strings.TrimSpace(issue.Type) == "manual_review_required"
}

func issueEvidenceExists(skillPath string, issue SkillSecurityIssue) bool {
	evidence := strings.TrimSpace(issue.Evidence)
	if evidence == "" {
		return false
	}

	for _, path := range candidateEvidenceFiles(skillPath, issue.File) {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if contentContainsEvidence(string(content), evidence) {
			return true
		}
	}
	return false
}

func candidateEvidenceFiles(skillPath string, issueFile string) []string {
	var candidates []string
	addCandidate := func(path string) {
		info, ok := regularFileInsideSkill(path, skillPath)
		if !ok || info.Size() > preAnalysisMaxFileSize {
			return
		}
		candidates = append(candidates, path)
	}

	cleanIssueFile := strings.TrimSpace(issueFile)
	if cleanIssueFile != "" {
		cleanIssueFile = filepath.Clean(cleanIssueFile)
		if filepath.IsAbs(cleanIssueFile) {
			addCandidate(cleanIssueFile)
		} else if !strings.HasPrefix(cleanIssueFile, "..") {
			addCandidate(filepath.Join(skillPath, cleanIssueFile))
		}
		if len(candidates) > 0 {
			return candidates
		}
	}

	_ = filepath.Walk(skillPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if skipTransientDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if skipTransientFile(info.Name()) ||
			info.Mode()&os.ModeSymlink != 0 ||
			info.Size() > preAnalysisMaxFileSize {
			return nil
		}
		candidates = append(candidates, path)
		return nil
	})
	return candidates
}

func regularFileInsideSkill(path string, root string) (os.FileInfo, bool) {
	if path == "" || !isPathInside(path, root) {
		return nil, false
	}
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, false
	}
	if !isPathInside(realPath, realRoot) {
		return nil, false
	}
	return info, true
}

func isPathInside(path string, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	if absPath == absRoot {
		return true
	}
	return strings.HasPrefix(absPath, absRoot+string(filepath.Separator))
}

func contentContainsEvidence(content string, evidence string) bool {
	normalizedContent := normalizeEvidenceText(content)
	normalizedEvidence := normalizeEvidenceText(evidence)
	if normalizedEvidence == "" {
		return false
	}
	if strings.Contains(normalizedContent, normalizedEvidence) {
		return true
	}

	contentFields := strings.Join(strings.Fields(normalizedContent), " ")
	evidenceFields := strings.Join(strings.Fields(normalizedEvidence), " ")
	if evidenceFields != "" && strings.Contains(contentFields, evidenceFields) {
		return true
	}

	return false
}

func normalizeEvidenceText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

// buildManualReviewIssues 构建解析失败场景的人工复核问题列表
func buildManualReviewIssues(skillPath string) []SkillSecurityIssue {
	return []SkillSecurityIssue{
		{
			Type:        "manual_review_required",
			Severity:    "medium",
			File:        strings.TrimSpace(skillPath),
			Description: "模型输出解析失败，需人工复核",
			Evidence:    "structured JSON parsing failed",
		},
	}
}

func containsRiskIndicators(text string) bool {
	loweredText := strings.ToLower(text)

	// Extended English risk indicators
	riskWords := []string{
		// Original
		"malicious", "dangerous", "risk", "unsafe", "suspicious",
		"backdoor", "exfiltration", "injection", "vulnerability",
		"critical", "high risk", "security issue",
		// New additions
		"poison", "inject", "exfiltrat", "trojan", "exploit",
		"compromis", "tamper", "hijack", "manipulat",
		"payload", "shellcode", "reverse shell", "command injection",
		"code execution", "rce", "data leak", "credential",
	}

	// Chinese risk indicators
	chineseRiskWords := []string{
		"风险", "危险", "恶意", "注入", "投毒", "后门", "可疑", "泄露",
		"木马", "漏洞", "攻击", "篡改", "劫持", "窃取", "提权",
	}

	// Negative contexts that negate risk words
	negativeContexts := []string{
		"no risk", "no issues", "not malicious", "no malicious",
		"no vulnerability", "no security issue", "appears safe",
		"is safe", "considered safe", "deemed safe",
		"没有风险", "无风险", "安全的", "无恶意",
	}

	// Check if there are more negative contexts than risk indicators
	negativeCount := 0
	for _, neg := range negativeContexts {
		if strings.Contains(loweredText, neg) {
			negativeCount++
		}
	}

	// Check English risk words
	for _, word := range riskWords {
		if strings.Contains(loweredText, word) {
			// Only consider it a risk if negative contexts don't dominate
			if negativeCount == 0 {
				return true
			}
		}
	}

	// Check Chinese risk words (case not applicable)
	for _, word := range chineseRiskWords {
		if strings.Contains(text, word) {
			if negativeCount == 0 {
				return true
			}
		}
	}

	return false
}

func inferRiskLevel(text string) string {
	loweredText := strings.ToLower(text)

	if strings.Contains(loweredText, "critical") {
		return "critical"
	}
	if strings.Contains(loweredText, "high risk") || strings.Contains(loweredText, "high severity") {
		return "high"
	}
	if strings.Contains(loweredText, "medium risk") || strings.Contains(loweredText, "medium severity") {
		return "medium"
	}
	if strings.Contains(loweredText, "low risk") || strings.Contains(loweredText, "low severity") {
		return "low"
	}
	if strings.Contains(loweredText, "safe") || strings.Contains(loweredText, "no issues") {
		return "none"
	}

	return "unknown"
}

func extractSummary(text string) string {
	// Try to find a summary section
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lowered := strings.ToLower(line)
		if strings.Contains(lowered, "summary") || strings.Contains(lowered, "conclusion") {
			// Return the next few non-empty lines
			var summary []string
			for j := i + 1; j < len(lines) && j < i+4; j++ {
				trimmed := strings.TrimSpace(lines[j])
				if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "```") {
					summary = append(summary, trimmed)
				}
			}
			if len(summary) > 0 {
				return strings.Join(summary, " ")
			}
		}
	}

	// Return a portion of the last paragraph
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) > 0 {
		lastPara := strings.TrimSpace(paragraphs[len(paragraphs)-1])
		if len(lastPara) > 200 {
			lastPara = lastPara[:200] + "..."
		}
		return lastPara
	}

	return "Analysis completed"
}

// Close releases resources held by the agent
func (sa *SkillSecurityAnalyzer) Close() error {
	// No explicit cleanup needed for most models
	return nil
}

// preAnalyzeSkill scans skill directory content to identify which scenario skills should be loaded.
// Returns a list of recommended skill names based on keyword matching.
func (sa *SkillSecurityAnalyzer) preAnalyzeSkill(skillPath string) []string {
	recommendedSkills := make(map[string][]string) // skill -> detected keywords

	err := filepath.Walk(skillPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logging.Warning("pre-analysis: skip file due to error, path=%s, err=%v", path, err)
			return nil // Continue walking, don't fail entire analysis
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip files larger than 2MB to avoid memory issues
		if info.Size() > preAnalysisMaxFileSize {
			logging.Warning("pre-analysis: skip large file, path=%s, size=%d", path, info.Size())
			return nil
		}

		// Check filename for patterns
		lowerFilename := strings.ToLower(info.Name())
		for skill, keywords := range skillKeywordPatterns {
			for _, keyword := range keywords {
				if strings.Contains(lowerFilename, strings.ToLower(keyword)) {
					if _, exists := recommendedSkills[skill]; !exists {
						recommendedSkills[skill] = []string{}
					}
					recommendedSkills[skill] = append(recommendedSkills[skill], keyword+" in filename")
				}
			}
		}

		// Read file content for analysis
		content, err := os.ReadFile(path)
		if err != nil {
			logging.Warning("pre-analysis: failed to read file, path=%s, err=%v", path, err)
			return nil
		}

		lowerContent := strings.ToLower(string(content))

		// Check content for patterns
		for skill, keywords := range skillKeywordPatterns {
			for _, keyword := range keywords {
				if strings.Contains(lowerContent, strings.ToLower(keyword)) {
					if _, exists := recommendedSkills[skill]; !exists {
						recommendedSkills[skill] = []string{}
					}
					// Avoid duplicate keyword entries
					alreadyAdded := false
					for _, k := range recommendedSkills[skill] {
						if strings.HasPrefix(k, keyword) {
							alreadyAdded = true
							break
						}
					}
					if !alreadyAdded {
						recommendedSkills[skill] = append(recommendedSkills[skill], keyword)
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		logging.Warning("pre-analysis: directory walk failed, err=%v", err)
		// Return all skills as fallback
		return allScanSkills
	}

	// If no skills matched, return all skills (LLM may find patterns humans miss)
	if len(recommendedSkills) == 0 {
		logging.Info("pre-analysis: no keywords matched, loading all skills")
		return allScanSkills
	}

	// Extract skill names
	result := make([]string, 0, len(recommendedSkills))
	for skill := range recommendedSkills {
		result = append(result, skill)
	}

	logging.Info("pre-analysis: recommended skills count=%d, skills=%v", len(result), result)
	return result
}

// buildUserPromptWithRecommendations builds user prompt with pre-analysis recommendations
func buildUserPromptWithRecommendations(skillPath, skillName string, recommendedSkills []string) string {
	basePrompt := GetUserPrompt(skillPath, skillName)

	if len(recommendedSkills) == 0 {
		return basePrompt
	}

	// Build recommendation section
	var sb strings.Builder
	sb.WriteString("\n\n---\n")
	sb.WriteString("**IMPORTANT: Pre-Analysis Recommendations**\n\n")
	sb.WriteString("Based on pre-analysis of the skill files, the following analysis skills are STRONGLY RECOMMENDED to be loaded and applied:\n\n")

	for _, skill := range recommendedSkills {
		sb.WriteString(fmt.Sprintf("- %s\n", skill))
	}

	sb.WriteString("\nYou MUST load and apply these skills during your analysis. Do NOT skip them.\n")
	sb.WriteString("---\n")

	return basePrompt + sb.String()
}

// ConvertSkillIssuesToStrings converts SkillSecurityIssue slice to string slice
func ConvertSkillIssuesToStrings(issues []SkillSecurityIssue) []string {
	result := make([]string, len(issues))
	for i, issue := range issues {
		result[i] = SerializeSkillIssue(issue)
	}
	return result
}

// SerializeSkillIssue converts a structured issue into a JSON string so it can
// be stored in the legacy string-array column without losing evidence details.
func SerializeSkillIssue(issue SkillSecurityIssue) string {
	data, err := json.Marshal(issue)
	if err != nil {
		return fmt.Sprintf("[%s] %s in %s: %s", issue.Severity, issue.Type, issue.File, issue.Description)
	}
	return string(data)
}
