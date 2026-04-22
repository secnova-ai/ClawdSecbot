package shepherd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"go_lib/core/logging"
	"go_lib/core/modelfactory"
	"go_lib/core/repository"
	"go_lib/core/skillscan"

	"github.com/cloudwego/eino/components/model"
)

// PostValidationOverrideTag is appended to a ReAct decision's reason when the
// Go post-validation layer forcibly overrides an LLM-allowed decision due to
// prompt injection in tool results. Downstream layers (proxy, UI classifiers)
// detect this tag to attribute the block source without extending the decision
// struct.
const PostValidationOverrideTag = "[Post-validation: tool result prompt injection must be blocked]"

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
	sg.mu.RLock()
	reactAnalyzer := sg.reactAnalyzer
	rules := cloneUserRules(sg.userRules)
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
			reactDecision.Reason = reactDecision.Reason + " " + PostValidationOverrideTag
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

type recoveryIntentLocalePack struct {
	statusLabel      string
	reasonLabel      string
	actionLabel      string
	riskTypeLabel    string
	statusAllowed    string
	statusNeedsConf  string
	statusUnknown    string
	mockIntroBlocked string
	mockIntroConfirm string
	agentSection     string
	continueGuide    string
	cancelGuide      string
	confirmReason    string
	rejectReason     string
	noneReason       string
	noUserTextReason string
	confirmKeywords  []string
	rejectKeywords   []string
}

func getRecoveryIntentLocalePack(lang string) recoveryIntentLocalePack {
	zhConfirmKeywords := []string{
		"好的", "继续", "ok", "okay", "没问题", "确认", "可以", "行", "继续执行", "同意",
	}
	zhRejectKeywords := []string{
		"取消", "停止", "不要", "不执行", "算了", "拒绝", "终止", "不用了", "不继续", "别执行",
	}
	enConfirmKeywords := []string{
		"ok", "okay", "yes", "yep", "sure", "continue", "go ahead", "proceed", "no problem", "confirm",
	}
	enRejectKeywords := []string{
		"cancel", "stop", "no", "nope", "reject", "abort", "don't", "do not", "not now", "nevermind", "never mind",
	}

	if normalizeShepherdLanguage(lang) == "zh" {
		return recoveryIntentLocalePack{
			statusLabel:      "状态",
			reasonLabel:      "原因",
			actionLabel:      "动作",
			riskTypeLabel:    "风险类型",
			statusAllowed:    "允许",
			statusNeedsConf:  "需要确认",
			statusUnknown:    "未知",
			mockIntroBlocked: "抱歉，当前请求已被安全策略拦截，无法继续执行。",
			mockIntroConfirm: "该操作存在风险，需要你先确认后才能继续执行。",
			agentSection:     "安全智能体分析",
			continueGuide:    "继续可回复：好的、继续、OK、没问题、确认、可以",
			cancelGuide:      "取消可回复：取消、停止、不要执行、不继续",
			confirmReason:    "Matched confirmation keyword, user agreed to continue.",
			rejectReason:     "Matched rejection keyword, user canceled the pending action.",
			noneReason:       "No confirmation or rejection keyword matched, keep pending recovery.",
			noUserTextReason: "No user reply found, keep pending recovery.",
			confirmKeywords:  deduplicateRecoveryIntentKeywords(append(zhConfirmKeywords, enConfirmKeywords...)),
			rejectKeywords:   deduplicateRecoveryIntentKeywords(append(zhRejectKeywords, enRejectKeywords...)),
		}
	}

	return recoveryIntentLocalePack{
		statusLabel:      "Status",
		reasonLabel:      "Reason",
		actionLabel:      "Action",
		riskTypeLabel:    "Risk Type",
		statusAllowed:    "Allowed",
		statusNeedsConf:  "Needs Confirmation",
		statusUnknown:    "Unknown",
		mockIntroBlocked: "Sorry, this request has been blocked by security policy and cannot proceed.",
		mockIntroConfirm: "This action is risky and requires your confirmation before continuing.",
		agentSection:     "Security Agent Analysis",
		continueGuide:    "Continue replies: OK, continue, yes, no problem, confirm",
		cancelGuide:      "Cancel replies: cancel, stop, do not, reject",
		confirmReason:    "Matched confirmation keyword, user agreed to continue.",
		rejectReason:     "Matched rejection keyword, user canceled the pending action.",
		noneReason:       "No confirmation or rejection keyword matched, keep pending recovery.",
		noUserTextReason: "No user reply found, keep pending recovery.",
		confirmKeywords:  deduplicateRecoveryIntentKeywords(append(enConfirmKeywords, zhConfirmKeywords...)),
		rejectKeywords:   deduplicateRecoveryIntentKeywords(append(enRejectKeywords, zhRejectKeywords...)),
	}
}

func localizeDecisionStatus(status string, pack recoveryIntentLocalePack) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ALLOWED":
		return pack.statusAllowed
	case "NEEDS_CONFIRMATION":
		return pack.statusNeedsConf
	default:
		return pack.statusUnknown
	}
}

func deduplicateRecoveryIntentKeywords(keywords []string) []string {
	if len(keywords) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(keywords))
	out := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		normalized := normalizeIntentText(keyword)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, keyword)
	}
	return out
}

func latestUserMessage(contextMessages []ConversationMessage) string {
	for i := len(contextMessages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(contextMessages[i].Role), "user") {
			return strings.TrimSpace(contextMessages[i].Content)
		}
	}
	return ""
}

func isCJKRune(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func containsCJK(text string) bool {
	for _, r := range text {
		if isCJKRune(r) {
			return true
		}
	}
	return false
}

func normalizeIntentText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(text))
	prevSpace := true
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || isCJKRune(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func compactIntentText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || isCJKRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasRecoveryIntentKeyword(normalizedText, compactText string, keywords []string) bool {
	if normalizedText == "" || len(keywords) == 0 {
		return false
	}

	tokens := strings.Fields(normalizedText)
	tokenSet := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		tokenSet[token] = struct{}{}
	}

	for _, keyword := range keywords {
		nk := normalizeIntentText(keyword)
		if nk == "" {
			continue
		}

		if containsCJK(nk) {
			compactKeyword := compactIntentText(nk)
			if compactKeyword != "" && strings.Contains(compactText, compactKeyword) {
				return true
			}
			continue
		}

		if strings.Contains(nk, " ") {
			if strings.Contains(normalizedText, nk) {
				return true
			}
			continue
		}

		if len(nk) <= 3 {
			if _, ok := tokenSet[nk]; ok {
				return true
			}
			continue
		}

		if strings.Contains(normalizedText, nk) {
			return true
		}
	}
	return false
}

// EvaluateRecoveryIntent determines whether the latest user reply confirms or rejects continuation.
func (sg *ShepherdGate) EvaluateRecoveryIntent(ctx context.Context, contextMessages []ConversationMessage, pendingToolCalls []ToolCallInfo, pendingReason string) (*RecoveryIntentDecision, error) {
	_ = ctx
	lang := sg.getEffectiveLanguage()
	pack := getRecoveryIntentLocalePack(lang)

	logging.ShepherdGateInfo(
		"[ShepherdGate][RecoveryIntent] Keyword analysis start: contextMessages=%d, pendingToolCalls=%d, pendingReason=%q",
		len(contextMessages),
		len(pendingToolCalls),
		strings.TrimSpace(pendingReason),
	)

	userText := latestUserMessage(contextMessages)
	if userText == "" {
		return &RecoveryIntentDecision{
			Intent: "NONE",
			Reason: pack.noUserTextReason,
		}, nil
	}

	normalized := normalizeIntentText(userText)
	compact := compactIntentText(userText)

	intent := "NONE"
	reason := pack.noneReason
	if hasRecoveryIntentKeyword(normalized, compact, pack.rejectKeywords) {
		intent = "REJECT"
		reason = pack.rejectReason
	} else if hasRecoveryIntentKeyword(normalized, compact, pack.confirmKeywords) {
		intent = "CONFIRM"
		reason = pack.confirmReason
	}

	logging.ShepherdGateInfo(
		"[ShepherdGate][RecoveryIntent] Keyword analysis done: intent=%s, reason=%s, userText=%q",
		intent,
		reason,
		userText,
	)

	return &RecoveryIntentDecision{
		Intent: intent,
		Reason: reason,
	}, nil
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

func (sg *ShepherdGate) formatSecurityAnalysisLines(decision *ShepherdDecision, withHeader bool) string {
	lang := sg.getEffectiveLanguage()
	pack := getRecoveryIntentLocalePack(lang)
	if decision == nil {
		decision = &ShepherdDecision{}
	}

	status := decision.Status
	if status == "" {
		status = "UNKNOWN"
	}
	displayStatus := localizeDecisionStatus(status, pack)
	reason := decision.Reason
	if reason == "" {
		if normalizeShepherdLanguage(lang) == "zh" {
			reason = "未知原因"
		} else {
			reason = "Unknown reason"
		}
	}

	formattedMsg := fmt.Sprintf("%s: %s | %s: %s", pack.statusLabel, displayStatus, pack.reasonLabel, reason)
	if withHeader {
		formattedMsg = fmt.Sprintf("[ShepherdGate] %s", formattedMsg)
	}
	if decision.ActionDesc != "" {
		formattedMsg += fmt.Sprintf("\n%s: %s", pack.actionLabel, decision.ActionDesc)
	}
	if decision.RiskType != "" {
		formattedMsg += fmt.Sprintf("\n%s: %s", pack.riskTypeLabel, decision.RiskType)
	}
	return formattedMsg
}

// FormatSecurityMessage formats a localized security warning message.
func (sg *ShepherdGate) FormatSecurityMessage(decision *ShepherdDecision) string {
	return sg.formatSecurityAnalysisLines(decision, true)
}

// FormatSecurityMockReply builds the final mock reply shown to users.
// It uses app-configured language and appends security agent analysis details.
func (sg *ShepherdGate) FormatSecurityMockReply(decision *ShepherdDecision) string {
	lang := sg.getEffectiveLanguage()
	pack := getRecoveryIntentLocalePack(lang)

	intro := pack.mockIntroBlocked
	needsConfirmation := decision != nil && decision.Status == "NEEDS_CONFIRMATION"
	if needsConfirmation {
		intro = pack.mockIntroConfirm
	}

	lines := []string{
		"[ShepherdGate] :",
		intro,
	}

	lines = append(lines, "")
	lines = append(lines, sg.formatSecurityAnalysisLines(decision, false))

	if needsConfirmation {
		lines = append(lines, "")
		lines = append(lines, pack.continueGuide)
		lines = append(lines, pack.cancelGuide)
	}

	return strings.Join(lines, "\n")
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
