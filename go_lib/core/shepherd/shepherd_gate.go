package shepherd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"go_lib/chatmodel-routing/adapter"
	"go_lib/core/logging"
	"go_lib/core/modelfactory"
	"go_lib/core/repository"
	"go_lib/core/skillscan"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ShepherdDecision represents the decision from ShepherdGate
type ShepherdDecision struct {
	Status     string `json:"-"`                 // Internal status: ALLOWED | NEEDS_CONFIRMATION
	Allowed    *bool  `json:"allowed,omitempty"` // Primary protocol field
	Reason     string `json:"reason"`
	ActionDesc string `json:"-"` // Action description (LLM generated)
	RiskType   string `json:"-"` // Risk type classification (LLM generated)
	Skill      string `json:"-"` // Triggered security skill name
	Usage      *Usage `json:"-"` // Usage stats for the check itself
}

// RecoveryIntentDecision represents the recognition result for user confirmation.
type RecoveryIntentDecision struct {
	Intent string `json:"intent"` // CONFIRM | REJECT | NONE
	Reason string `json:"reason"`
	Usage  *Usage `json:"-"`
}

// Usage represents token usage statistics
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// UserRules holds the parsed user security rules
type UserRules struct {
	SensitiveActions []string `json:"SensitiveActions"`
}

// ShepherdGate implements the security gate logic
type ShepherdGate struct {
	mu          sync.RWMutex
	modelConfig *repository.SecurityModelConfig
	chatModel   model.ChatModel
	language    string
	assetName   string
	assetID     string

	reactAnalyzer *ToolCallReActAnalyzer
	reactSkillCfg ReActSkillRuntimeConfig
	userRules     *UserRules
}

// NewShepherdGate creates a new ShepherdGate instance
func NewShepherdGate(config *repository.SecurityModelConfig) (*ShepherdGate, error) {
	return NewShepherdGateWithRuntime(config, nil)
}

// NewShepherdGateWithRuntime creates a new ShepherdGate with optional ReAct runtime config.
func NewShepherdGateWithRuntime(config *repository.SecurityModelConfig, reactCfg *ReActSkillRuntimeConfig) (*ShepherdGate, error) {
	if err := modelfactory.ValidateSecurityModelConfig(config); err != nil {
		return nil, fmt.Errorf("invalid security model config: %w", err)
	}

	ctx := context.Background()
	chatModel, err := modelfactory.CreateChatModelFromConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model: %w", err)
	}

	defaultRules, err := loadDefaultUserRules()
	if err != nil {
		logging.Warning("[ShepherdGate] Failed to load default user rules, fallback to empty rules: %v", err)
		defaultRules = &UserRules{SensitiveActions: []string{}}
	}

	sg := &ShepherdGate{
		modelConfig:   config,
		chatModel:     chatModel,
		language:      "en",
		reactSkillCfg: normalizeReActSkillRuntimeConfig(reactCfg),
		userRules:     cloneUserRules(defaultRules),
	}

	lang := skillscan.GetLanguageFromAppSettings()
	if lang != "" {
		sg.SetLanguage(lang)
	}

	reactAnalyzer, analyzerErr := NewToolCallReActAnalyzerWithConfig(ctx, chatModel, sg.language, config, &sg.reactSkillCfg)
	if analyzerErr != nil {
		return nil, fmt.Errorf("failed to initialize ReAct analyzer: %w", analyzerErr)
	}
	sg.reactAnalyzer = reactAnalyzer

	return sg, nil
}

// NewShepherdGateForTesting creates a ShepherdGate with injected dependencies for unit testing.
// This bypasses config validation and model creation, allowing mock models.
func NewShepherdGateForTesting(chatModel model.ChatModel, language string, modelConfig *repository.SecurityModelConfig) *ShepherdGate {
	return &ShepherdGate{
		chatModel:   chatModel,
		language:    language,
		modelConfig: modelConfig,
		userRules:   &UserRules{SensitiveActions: []string{}},
	}
}

// GetUserRules returns a copy of current user rules for this gate instance.
func (sg *ShepherdGate) GetUserRules() *UserRules {
	sg.mu.RLock()
	defer sg.mu.RUnlock()
	return cloneUserRules(sg.userRules)
}

// UpdateUserRules updates user rules for this gate instance.
func (sg *ShepherdGate) UpdateUserRules(sensitiveActions []string) {
	sg.mu.Lock()
	sg.userRules = &UserRules{
		SensitiveActions: normalizeSensitiveActions(sensitiveActions),
	}
	sg.mu.Unlock()
}

// getEffectiveLanguage returns the current effective language.
func (sg *ShepherdGate) getEffectiveLanguage() string {
	dbLang := strings.TrimSpace(skillscan.GetLanguageFromAppSettings())
	if dbLang == "" {
		sg.mu.RLock()
		cached := sg.language
		sg.mu.RUnlock()
		return normalizeShepherdLanguage(cached)
	}

	effective := normalizeShepherdLanguage(dbLang)
	sg.mu.Lock()
	prev := sg.language
	sg.language = effective
	reactAnalyzer := sg.reactAnalyzer
	sg.mu.Unlock()

	if reactAnalyzer != nil && prev != effective {
		reactAnalyzer.SetLanguage(effective)
	}
	return effective
}

func (sg *ShepherdGate) SetLanguage(lang string) {
	sg.mu.Lock()
	sg.language = normalizeShepherdLanguage(lang)
	reactAnalyzer := sg.reactAnalyzer
	finalLang := sg.language
	sg.mu.Unlock()

	if reactAnalyzer != nil {
		reactAnalyzer.SetLanguage(finalLang)
	}
}

// SetAssetContext sets asset identity used for security event attribution.
func (sg *ShepherdGate) SetAssetContext(assetName, assetID string) {
	sg.mu.Lock()
	sg.assetName = strings.TrimSpace(assetName)
	sg.assetID = strings.TrimSpace(assetID)
	reactAnalyzer := sg.reactAnalyzer
	normalizedAssetName := sg.assetName
	normalizedAssetID := sg.assetID
	sg.mu.Unlock()

	if reactAnalyzer != nil {
		reactAnalyzer.SetAssetContext(normalizedAssetName, normalizedAssetID)
	}
}

// UpdateModelConfig updates the model configuration and recreates the chat model.
func (sg *ShepherdGate) UpdateModelConfig(config *repository.SecurityModelConfig) error {
	ctx := context.Background()
	chatModel, err := modelfactory.CreateChatModelFromConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to recreate chat model: %w", err)
	}

	sg.mu.RLock()
	lang := sg.language
	oldAnalyzer := sg.reactAnalyzer
	reactSkillCfg := sg.reactSkillCfg
	assetName := sg.assetName
	assetID := sg.assetID
	sg.mu.RUnlock()

	newAnalyzer, analyzerErr := NewToolCallReActAnalyzerWithConfig(ctx, chatModel, lang, config, &reactSkillCfg)
	if analyzerErr != nil {
		return fmt.Errorf("failed to recreate ReAct analyzer: %w", analyzerErr)
	}
	newAnalyzer.SetAssetContext(assetName, assetID)

	sg.mu.Lock()
	sg.modelConfig = config
	sg.chatModel = chatModel
	sg.reactAnalyzer = newAnalyzer
	sg.mu.Unlock()

	if oldAnalyzer != nil {
		oldAnalyzer.Close()
	}

	logging.ShepherdGateInfo("[ShepherdGate][UpdateModelConfig][-] chat model updated successfully")
	return nil
}

// UpdateReActSkillConfig updates ReAct skill loading/runtime settings.
func (sg *ShepherdGate) UpdateReActSkillConfig(cfg *ReActSkillRuntimeConfig) error {
	sg.mu.Lock()
	sg.reactSkillCfg = normalizeReActSkillRuntimeConfig(cfg)
	analyzer := sg.reactAnalyzer
	normalized := sg.reactSkillCfg
	sg.mu.Unlock()

	if analyzer == nil {
		return nil
	}
	if err := analyzer.UpdateRuntimeConfig(context.Background(), &normalized); err != nil {
		return err
	}
	logging.ShepherdGateInfo("[ShepherdGate][UpdateReActSkillConfig][-] config updated: enableBuiltin=%v",
		normalized.EnableBuiltinSkills)
	return nil
}

// GetModelName returns the current security model name
func (sg *ShepherdGate) GetModelName() string {
	sg.mu.RLock()
	defer sg.mu.RUnlock()
	if sg.modelConfig != nil {
		return sg.modelConfig.Model
	}
	return ""
}

func extractUsage(extra map[string]interface{}, defaultPromptTokens, defaultCompletionTokens int) *Usage {
	if extra == nil {
		return &Usage{
			PromptTokens:     defaultPromptTokens,
			CompletionTokens: defaultCompletionTokens,
			TotalTokens:      defaultPromptTokens + defaultCompletionTokens,
		}
	}

	var usageVal interface{}
	var ok bool
	if usageVal, ok = extra["usage"]; !ok {
		usageVal, ok = extra["Usage"]
	}

	if ok {
		if usageMap, ok := usageVal.(map[string]interface{}); ok {
			return &Usage{
				PromptTokens:     getIntFromMap(usageMap, "prompt_tokens"),
				CompletionTokens: getIntFromMap(usageMap, "completion_tokens"),
				TotalTokens:      getIntFromMap(usageMap, "total_tokens"),
			}
		}
		if jsonBytes, err := json.Marshal(usageVal); err == nil {
			var u Usage
			if err := json.Unmarshal(jsonBytes, &u); err == nil {
				return &u
			}
		}
	}

	return &Usage{
		PromptTokens:     defaultPromptTokens,
		CompletionTokens: defaultCompletionTokens,
		TotalTokens:      defaultPromptTokens + defaultCompletionTokens,
	}
}

// CheckToolCall performs the security check
func (sg *ShepherdGate) CheckToolCall(ctx context.Context, contextMessages []ConversationMessage, toolCalls []ToolCallInfo, toolResults []ToolResultInfo, lastUserMessage string, requestID ...string) (*ShepherdDecision, error) {
	rules := sg.GetUserRules()

	sg.mu.RLock()
	reactAnalyzer := sg.reactAnalyzer
	sg.mu.RUnlock()
	lang := sg.getEffectiveLanguage()

	var toolNames []string
	for _, tc := range toolCalls {
		toolNames = append(toolNames, tc.Name)
	}
	logging.ShepherdGateInfo("[ShepherdGate][CheckToolCall][-] invoked: tools=[%s], contextMessages=%d, toolResults=%d", strings.Join(toolNames, ", "), len(contextMessages), len(toolResults))

	for i, tc := range toolCalls {
		argsDisplay := tc.RawArgs
		if len(argsDisplay) > 500 {
			argsDisplay = argsDisplay[:500] + "...(truncated)"
		}
		logging.Info("[ShepherdGate][CheckToolCall] toolCall[%d]: name=%s, id=%s, args=%s", i, tc.Name, tc.ToolCallID, argsDisplay)
	}
	for i, tr := range toolResults {
		contentDisplay := tr.Content
		if len(contentDisplay) > 500 {
			contentDisplay = contentDisplay[:500] + "...(truncated)"
		}
		logging.Info("[ShepherdGate][CheckToolCall] toolResult[%d]: func=%s, id=%s, content=%s", i, tr.FuncName, tr.ToolCallID, contentDisplay)
	}

	reactAnalyzer.SetLanguage(lang)
	reqID := ""
	if len(requestID) > 0 {
		reqID = requestID[0]
	}
	reactDecision, reactErr := reactAnalyzer.Analyze(ctx, contextMessages, toolCalls, toolResults, rules, lastUserMessage, reqID)
	if reactErr != nil {
		logging.ShepherdGateError("[ShepherdGate][CheckToolCall][-] ReAct analyzer failed: %v, fail-open", reactErr)
		allowed := true
		return &ShepherdDecision{
			Status:  "ALLOWED",
			Allowed: &allowed,
			Reason:  fmt.Sprintf("Security check bypassed due to ReAct error: %v", reactErr),
		}, nil
	}

	if reactDecision.Allowed && len(toolResults) > 0 {
		if isPromptInjectionRisk(reactDecision.RiskType) && isHighOrCriticalRisk(reactDecision.RiskLevel) {
			logging.ShepherdGateWarning("[ShepherdGate][CheckToolCall][-] post-validation override: "+
				"LLM allowed but prompt injection detected in tool result, forcing block. "+
				"risk_type=%s, risk_level=%s", reactDecision.RiskType, reactDecision.RiskLevel)
			reactDecision.Allowed = false
			reactDecision.Reason = reactDecision.Reason + " [Post-validation: tool result prompt injection must be blocked]"
		}
	}

	allowed := reactDecision.Allowed
	status := "ALLOWED"
	if !allowed {
		status = "NEEDS_CONFIRMATION"
	}
	logging.ShepherdGateInfo("[ShepherdGate][CheckToolCall][-] result: status=%s, skill=%s, confidence=%d",
		status, reactDecision.Skill, reactDecision.Confidence)
	return &ShepherdDecision{
		Status:     status,
		Allowed:    &allowed,
		Reason:     reactDecision.Reason,
		ActionDesc: reactDecision.ActionDesc,
		RiskType:   reactDecision.RiskType,
		Skill:      reactDecision.Skill,
		Usage:      mergeUsage(reactDecision.Usage, nil),
	}, nil
}

func isPromptInjectionRisk(riskType string) bool {
	lower := strings.ToLower(riskType)
	return strings.Contains(lower, "inject") ||
		strings.Contains(lower, "注入") ||
		strings.Contains(lower, "hijack") ||
		strings.Contains(lower, "劫持")
}

func isHighOrCriticalRisk(riskLevel string) bool {
	return riskLevel == "high" || riskLevel == "critical"
}

func mergeUsage(left *Usage, right *Usage) *Usage {
	if left == nil && right == nil {
		return nil
	}
	merged := &Usage{}
	if left != nil {
		merged.PromptTokens += left.PromptTokens
		merged.CompletionTokens += left.CompletionTokens
		merged.TotalTokens += left.TotalTokens
	}
	if right != nil {
		merged.PromptTokens += right.PromptTokens
		merged.CompletionTokens += right.CompletionTokens
		merged.TotalTokens += right.TotalTokens
	}
	return merged
}

// NormalizeRecoveryIntent normalizes a recovery intent string to CONFIRM/REJECT/NONE.
func NormalizeRecoveryIntent(intent string) string {
	return normalizeRecoveryIntent(intent)
}

func normalizeRecoveryIntent(intent string) string {
	switch strings.ToUpper(strings.TrimSpace(intent)) {
	case "CONFIRM":
		return "CONFIRM"
	case "REJECT":
		return "REJECT"
	default:
		return "NONE"
	}
}

// EvaluateRecoveryIntent uses the security model to determine if the user confirms continuation.
func (sg *ShepherdGate) EvaluateRecoveryIntent(ctx context.Context, contextMessages []ConversationMessage, pendingToolCalls []ToolCallInfo, pendingReason string) (*RecoveryIntentDecision, error) {
	sg.mu.RLock()
	chatModel := sg.chatModel
	modelCfg := sg.modelConfig
	sg.mu.RUnlock()
	lang := sg.getEffectiveLanguage()

	if chatModel == nil {
		return nil, fmt.Errorf("chat model is nil")
	}

	languageName := "English"
	if lang == "zh" {
		languageName = "Chinese (Simplified)"
	}

	maxTokens := 1024
	if modelCfg != nil {
		maxTokens = adapter.GetModelMaxOutputTokens(modelCfg.Model)
	}

	filtered := contextMessages
	if len(filtered) > 80 {
		filtered = filtered[len(filtered)-80:]
	}
	contextBytes, _ := json.MarshalIndent(filtered, "", "  ")
	toolCallsBytes, _ := json.MarshalIndent(pendingToolCalls, "", "  ")

	systemPrompt := fmt.Sprintf(`You are ShepherdGate Recovery Intent Analyzer.
Your task: decide whether the latest user message confirms, rejects, or does not clearly decide execution of the pending blocked tool calls.

Output STRICT JSON only with this schema:
{
  "intent": "CONFIRM" | "REJECT" | "NONE",
  "reason": "short reason in %s, <= 40 words"
}

Rules:
1) Focus on semantic intent, not keyword matching.
2) CONFIRM: user clearly agrees to proceed with the pending action.
3) REJECT: user clearly refuses/cancels the pending action.
4) NONE: unclear/irrelevant/new task/question.
5) Use the latest user message as primary signal, but use conversation context for disambiguation.
6) If user intent does not match pending action scope, return NONE.
7) Return JSON only, no markdown and no extra text.
`, languageName)

	userPrompt := fmt.Sprintf(`CONTEXT:
%s

PENDING_TOOL_CALLS:
%s

PENDING_RISK_REASON:
%s
`, string(contextBytes), string(toolCallsBytes), strings.TrimSpace(pendingReason))

	logging.ShepherdGateInfo("[ShepherdGate][RecoveryIntent] Analyze start: contextMessages=%d, pendingToolCalls=%d", len(filtered), len(pendingToolCalls))

	resp, err := chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userPrompt),
	},
		model.WithTemperature(0),
		model.WithMaxTokens(maxTokens),
	)
	if err != nil {
		logging.ShepherdGateError("[ShepherdGate][RecoveryIntent] LLM generation failed: %v", err)
		return nil, err
	}

	content := extractJSON(resp.Content)
	if strings.TrimSpace(content) == "" {
		logging.ShepherdGateWarning("[ShepherdGate][RecoveryIntent] Empty model response")
		return &RecoveryIntentDecision{
			Intent: "NONE",
			Reason: "Empty model response",
			Usage:  extractUsage(resp.Extra, 0, 0),
		}, nil
	}

	var parsed RecoveryIntentDecision
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		logging.ShepherdGateWarning("[ShepherdGate][RecoveryIntent] Parse response failed: %v, raw=%q", err, resp.Content)
		return &RecoveryIntentDecision{
			Intent: "NONE",
			Reason: "Failed to parse model response",
			Usage:  extractUsage(resp.Extra, 0, 0),
		}, nil
	}

	parsed.Intent = normalizeRecoveryIntent(parsed.Intent)
	parsed.Reason = strings.TrimSpace(parsed.Reason)
	if parsed.Reason == "" {
		parsed.Reason = "No reason provided by model."
	}
	parsed.Usage = extractUsage(resp.Extra, 0, 0)
	logging.ShepherdGateInfo("[ShepherdGate][RecoveryIntent] Analyze done: intent=%s, reason=%s, tokens=%d", parsed.Intent, parsed.Reason, parsed.Usage.TotalTokens)
	return &parsed, nil
}

// NormalizeShepherdLanguage normalizes a language string to a standard form (e.g., "zh", "en").
func NormalizeShepherdLanguage(lang string) string {
	return normalizeShepherdLanguage(lang)
}

func normalizeShepherdLanguage(lang string) string {
	lang = strings.TrimSpace(strings.ToLower(lang))
	lang = strings.ReplaceAll(lang, "_", "-")
	lang = strings.ReplaceAll(lang, " ", "")

	if lang == "" {
		return "en"
	}

	if lang == "cn" || strings.HasPrefix(lang, "zh") || strings.Contains(lang, "chinese") {
		return "zh"
	}
	if strings.HasPrefix(lang, "en") || strings.Contains(lang, "english") {
		return "en"
	}

	switch lang {
	case "zh-hans", "zh-hant", "zh-cn", "zh-tw", "zh-hk":
		return "zh"
	}
	return lang
}

func getIntFromMap(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

// FormatSecurityMessage formats the security warning message in English.
func (sg *ShepherdGate) FormatSecurityMessage(decision *ShepherdDecision) string {
	status := decision.Status
	if status == "" {
		status = "UNKNOWN"
	}
	reason := decision.Reason
	if reason == "" {
		reason = "Unknown reason"
	}

	formattedMsg := fmt.Sprintf("[ShepherdGate] Status: %s | Reason: %s", status, reason)
	if decision.ActionDesc != "" {
		formattedMsg += fmt.Sprintf("\nAction: %s", decision.ActionDesc)
	}
	if decision.RiskType != "" {
		formattedMsg += fmt.Sprintf("\nRisk Type: %s", decision.RiskType)
	}
	if status == "NEEDS_CONFIRMATION" {
		formattedMsg += "\n\nPlease confirm to proceed."
	}
	return formattedMsg
}

func estimateStringTokens(text string) int {
	if text == "" {
		return 0
	}
	tokenCount := 0.0
	for _, r := range text {
		if r < 128 {
			tokenCount += 0.25
		} else {
			tokenCount += 1.5
		}
	}
	count := int(tokenCount)
	if tokenCount > float64(count) {
		count++
	}
	if count == 0 && len(text) > 0 {
		return 1
	}
	return count
}

func extractJSON(content string) string {
	content = strings.TrimSpace(content)
	if idx := strings.Index(content, "```"); idx >= 0 {
		start := idx + 3
		if nl := strings.IndexByte(content[start:], '\n'); nl >= 0 {
			start += nl + 1
		}
		if end := strings.Index(content[start:], "```"); end >= 0 {
			content = strings.TrimSpace(content[start : start+end])
		} else {
			content = strings.TrimSpace(content[start:])
		}
	}
	firstBrace := strings.IndexByte(content, '{')
	lastBrace := strings.LastIndexByte(content, '}')
	if firstBrace >= 0 && lastBrace > firstBrace {
		return content[firstBrace : lastBrace+1]
	}
	return content
}
