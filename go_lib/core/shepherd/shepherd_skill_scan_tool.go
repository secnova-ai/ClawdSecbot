package shepherd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/core/skillscan"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ==================== scan_skill_security ====================

type scanSkillSecurityTool struct {
	modelConfig *repository.SecurityModelConfig
	usageSink   func(Usage)
}

func newScanSkillSecurityTool(modelConfig *repository.SecurityModelConfig, usageSink ...func(Usage)) tool.BaseTool {
	t := &scanSkillSecurityTool{modelConfig: modelConfig}
	if len(usageSink) > 0 {
		t.usageSink = usageSink[0]
	}
	return t
}

func (t *scanSkillSecurityTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "scan_skill_security",
		Desc: "Scan a downloaded skill directory for security risks including prompt injection, data theft, malicious code execution, social engineering, and supply chain attacks. Returns cached results if the skill was previously scanned, otherwise performs full AI-powered security analysis. Use this tool when you detect that the business agent has installed or downloaded a skill/plugin/MCP server.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"skill_path": {Type: schema.String, Required: true, Desc: "The absolute path to the downloaded skill directory to scan"},
		}),
	}, nil
}

type skillScanResultJSON struct {
	Safe       bool                    `json:"safe"`
	RiskLevel  string                  `json:"risk_level"`
	Risks      []skillScanRiskItemJSON `json:"risks,omitempty"`
	Summary    string                  `json:"summary"`
	ScanStatus string                  `json:"scan_status"`
	Cached     bool                    `json:"cached,omitempty"`
	TokenUsage *Usage                  `json:"token_usage,omitempty"`
}

type skillScanRiskItemJSON struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Evidence    string `json:"evidence,omitempty"`
	File        string `json:"file,omitempty"`
}

type skillScanErrorJSON struct {
	Safe       bool   `json:"safe"`
	RiskLevel  string `json:"risk_level"`
	Error      string `json:"error"`
	ScanStatus string `json:"scan_status"`
}

func (t *scanSkillSecurityTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		SkillPath string `json:"skill_path"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return t.errorResponse(fmt.Sprintf("invalid arguments: %v", err)), nil
	}

	skillPath := args.SkillPath
	logging.ShepherdGateInfo("[ScanSkillSecurity] invoked: skill_path=%s", skillPath)

	info, err := os.Stat(skillPath)
	if err != nil {
		if os.IsNotExist(err) {
			return t.errorResponse(fmt.Sprintf("skill path does not exist: %s", skillPath)), nil
		}
		return t.errorResponse(fmt.Sprintf("failed to access skill path: %v", err)), nil
	}
	if !info.IsDir() {
		return t.errorResponse(fmt.Sprintf("skill path is not a directory: %s", skillPath)), nil
	}

	hash, err := skillscan.CalculateSkillHash(skillPath)
	if err != nil {
		logging.ShepherdGateWarning("[ScanSkillSecurity] failed to calculate hash: %v", err)
		return t.errorResponse(fmt.Sprintf("failed to calculate skill hash: %v", err)), nil
	}
	logging.ShepherdGateInfo("[ScanSkillSecurity] calculated hash=%s for path=%s", hash, skillPath)

	repo := repository.NewSkillSecurityScanRepository(nil)
	cached, err := repo.GetSkillScanByHash(hash)
	if err != nil {
		logging.ShepherdGateWarning("[ScanSkillSecurity] cache lookup failed: %v", err)
	}

	if cached != nil && storedSkillScanReusable(skillPath, cached) {
		logging.ShepherdGateInfo("[ScanSkillSecurity] cache hit: hash=%s, safe=%v", hash, cached.Safe)

		riskLevel := cached.RiskLevel
		if riskLevel == "" {
			riskLevel = inferRiskLevelFromSafe(cached.Safe, len(cached.Issues) > 0)
		}

		risks := convertIssuesToRisks(cached.Issues)

		result := skillScanResultJSON{
			Safe:       cached.Safe,
			RiskLevel:  riskLevel,
			Risks:      risks,
			Summary:    fmt.Sprintf("Cached scan result from %s", cached.ScannedAt),
			ScanStatus: "cached",
			Cached:     true,
		}
		return t.successResponse(result), nil
	}

	logging.ShepherdGateInfo("[ScanSkillSecurity] cache miss, starting AI analysis: hash=%s", hash)

	if t.modelConfig == nil {
		return t.errorResponse("security model config not available"), nil
	}

	analyzer, err := skillscan.NewSkillSecurityAnalyzer(t.modelConfig)
	if err != nil {
		logging.ShepherdGateError("[ScanSkillSecurity] failed to create analyzer: %v", err)
		return t.errorResponse(fmt.Sprintf("failed to create security analyzer: %v", err)), nil
	}
	defer analyzer.Close()

	analysisCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	analysisResult, err := analyzer.AnalyzeSkill(analysisCtx, skillPath)
	if err != nil {
		if promptTokens, completionTokens, totalTokens, ok := skillscan.UsageFromAnalysisError(err); ok {
			t.recordUsage(&Usage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      totalTokens,
			})
		}
		logging.ShepherdGateError("[ScanSkillSecurity] analysis failed: %v", err)
		return t.errorResponse(fmt.Sprintf("security analysis failed: %v", err)), nil
	}

	logging.ShepherdGateInfo("[ScanSkillSecurity] analysis complete: safe=%v, risk_level=%s",
		analysisResult.Safe, analysisResult.RiskLevel)
	usage := normalizeUsage(&Usage{
		PromptTokens:     analysisResult.PromptTokens,
		CompletionTokens: analysisResult.CompletionTokens,
		TotalTokens:      analysisResult.TotalTokens,
	})
	t.recordUsage(usage)

	skillName := filepath.Base(skillPath)
	issues := extractIssueStrings(analysisResult.Issues)
	record := &repository.SkillScanRecord{
		SkillName: skillName,
		SkillHash: hash,
		SkillPath: skillPath,
		Safe:      analysisResult.Safe,
		RiskLevel: analysisResult.RiskLevel,
		Issues:    issues,
	}
	if saveErr := repo.SaveSkillScanResult(record); saveErr != nil {
		logging.ShepherdGateWarning("[ScanSkillSecurity] failed to save result: %v", saveErr)
	}

	risks := extractRisks(analysisResult.Issues)
	result := skillScanResultJSON{
		Safe:       analysisResult.Safe,
		RiskLevel:  analysisResult.RiskLevel,
		Risks:      risks,
		Summary:    analysisResult.Summary,
		ScanStatus: "completed",
		Cached:     false,
		TokenUsage: usage,
	}
	return t.successResponse(result), nil
}

func storedSkillScanReusable(skillPath string, record *repository.SkillScanRecord) bool {
	if record == nil {
		return false
	}
	if record.Safe || len(record.Issues) == 0 {
		return true
	}
	filteredIssues, _ := skillscan.ValidateStoredIssueStrings(skillPath, record.Issues)
	return len(filteredIssues) > 0
}

func (t *scanSkillSecurityTool) recordUsage(usage *Usage) {
	if t == nil || t.usageSink == nil || usage == nil {
		return
	}
	t.usageSink(*usage)
}

func (t *scanSkillSecurityTool) errorResponse(errMsg string) string {
	resp := skillScanErrorJSON{
		Error:      errMsg,
		Safe:       false,
		RiskLevel:  "unknown",
		ScanStatus: "error",
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func (t *scanSkillSecurityTool) successResponse(result skillScanResultJSON) string {
	b, _ := json.Marshal(result)
	return string(b)
}

func extractIssueStrings(issues []skillscan.SkillSecurityIssue) []string {
	if len(issues) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(issues))
	for _, issue := range issues {
		result = append(result, skillscan.SerializeSkillIssue(issue))
	}
	return result
}

func extractRisks(issues []skillscan.SkillSecurityIssue) []skillScanRiskItemJSON {
	if len(issues) == 0 {
		return []skillScanRiskItemJSON{}
	}
	result := make([]skillScanRiskItemJSON, 0, len(issues))
	for _, issue := range issues {
		result = append(result, skillScanRiskItemJSON{
			Type:        issue.Type,
			Severity:    issue.Severity,
			Description: issue.Description,
			Evidence:    issue.Evidence,
			File:        issue.File,
		})
	}
	return result
}

func convertIssuesToRisks(issues []string) []skillScanRiskItemJSON {
	if len(issues) == 0 {
		return []skillScanRiskItemJSON{}
	}
	result := make([]skillScanRiskItemJSON, 0, len(issues))
	for _, issue := range issues {
		riskItem := skillScanRiskItemJSON{
			Description: issue,
			Severity:    "unknown",
			Type:        "unknown",
		}
		if err := json.Unmarshal([]byte(issue), &riskItem); err == nil {
			if riskItem.Description == "" {
				riskItem.Description = issue
			}
		} else if len(issue) > 4 && issue[0] == '[' {
			riskItem = parseIssueString(issue)
		}
		result = append(result, riskItem)
	}
	return result
}

func parseIssueString(issue string) skillScanRiskItemJSON {
	result := skillScanRiskItemJSON{
		Description: issue,
		Severity:    "unknown",
		Type:        "unknown",
	}

	parts := []string{}
	remaining := issue
	for len(remaining) > 0 && remaining[0] == '[' {
		end := -1
		for i, c := range remaining {
			if c == ']' {
				end = i
				break
			}
		}
		if end == -1 {
			break
		}
		parts = append(parts, remaining[1:end])
		remaining = remaining[end+1:]
	}

	remaining = strings.TrimSpace(remaining)

	if len(parts) >= 2 {
		result.Severity = parts[0]
		result.Type = parts[1]
		result.Description = remaining
	}
	if len(parts) >= 3 {
		result.File = parts[2]
	}

	return result
}

func inferRiskLevelFromSafe(safe bool, hasIssues bool) string {
	if safe {
		return "none"
	}
	if hasIssues {
		return "medium"
	}
	return "low"
}
