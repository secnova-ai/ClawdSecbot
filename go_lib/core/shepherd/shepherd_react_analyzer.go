package shepherd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go_lib/core/logging"
	"go_lib/core/repository"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"github.com/cloudwego/eino-ext/adk/backend/local"
)

// ReactRiskDecision is the ReAct+Skill risk analysis result.
type ReactRiskDecision struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"`
	RiskLevel  string `json:"risk_level,omitempty"`
	Confidence int    `json:"confidence,omitempty"`
	ActionDesc string `json:"action_desc,omitempty"`
	RiskType   string `json:"risk_type,omitempty"`
	Skill      string `json:"skill,omitempty"`
	Usage      *Usage `json:"-"`
}

type usageAccumulator struct {
	mu    sync.Mutex
	usage Usage
}

func (a *usageAccumulator) Add(usage Usage) {
	normalized := normalizeUsage(&Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	})
	if normalized == nil {
		return
	}
	a.mu.Lock()
	a.usage.PromptTokens += normalized.PromptTokens
	a.usage.CompletionTokens += normalized.CompletionTokens
	a.usage.TotalTokens += normalized.TotalTokens
	a.mu.Unlock()
}

func (a *usageAccumulator) Snapshot() *Usage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return normalizeUsage(&Usage{
		PromptTokens:     a.usage.PromptTokens,
		CompletionTokens: a.usage.CompletionTokens,
		TotalTokens:      a.usage.TotalTokens,
	})
}

type callbackUsageCollector struct {
	actual   usageAccumulator
	fallback usageAccumulator
}

func newCallbackUsageCollector() (*callbackUsageCollector, callbacks.Handler) {
	collector := &callbackUsageCollector{}
	handler := callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, _ *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			modelInput := model.ConvCallbackInput(input)
			if modelInput == nil {
				return ctx
			}
			collector.fallback.Add(Usage{
				PromptTokens: estimateMessagesAndToolsTokens(modelInput.Messages, modelInput.Tools),
			})
			return ctx
		}).
		OnEndFn(func(ctx context.Context, _ *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			modelOutput := model.ConvCallbackOutput(output)
			if modelOutput == nil {
				return ctx
			}
			if usage := usageFromModelCallbackOutput(modelOutput); usage != nil {
				collector.actual.Add(*usage)
				return ctx
			}
			if modelOutput.Message != nil {
				collector.fallback.Add(Usage{
					CompletionTokens: estimateStringTokens(modelOutput.Message.Content),
				})
			}
			return ctx
		}).
		Build()
	return collector, handler
}

func (c *callbackUsageCollector) Snapshot() *Usage {
	if c == nil {
		return nil
	}
	if usage := c.actual.Snapshot(); usage != nil {
		return usage
	}
	return c.fallback.Snapshot()
}

func usageFromModelCallbackOutput(output *model.CallbackOutput) *Usage {
	if output == nil {
		return nil
	}
	if output.TokenUsage != nil {
		return normalizeUsage(&Usage{
			PromptTokens:     output.TokenUsage.PromptTokens,
			CompletionTokens: output.TokenUsage.CompletionTokens,
			TotalTokens:      output.TokenUsage.TotalTokens,
		})
	}
	return usageFromMessageMetadata(output.Message)
}

func estimateMessagesAndToolsTokens(messages []*schema.Message, tools []*schema.ToolInfo) int {
	total := 0
	if len(messages) > 0 {
		if payload, err := json.Marshal(messages); err == nil {
			total += estimateStringTokens(string(payload))
		}
	}
	if len(tools) > 0 {
		if payload, err := json.Marshal(tools); err == nil {
			total += estimateStringTokens(string(payload))
		}
	}
	return total
}

// ==================== Unified trace logging ====================

const shepherdFlowLogPrefix = "[ShepherdGate][Flow]"

func traceGuard(sessionID, component, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	logging.ShepherdGateInfo("%s[react][%s][%s] %s", shepherdFlowLogPrefix, component, sessionID, msg)
}

// ==================== ReAct Analyzer ====================

// ToolCallReActAnalyzer uses Eino ADK (skill middleware + filesystem middleware) for tool_calls risk analysis.
type ToolCallReActAnalyzer struct {
	mu sync.RWMutex

	language  string
	assetName string
	assetID   string

	chatModel   model.ChatModel
	modelConfig *repository.SecurityModelConfig
	skillsDir   string

	skillConfig ReActSkillRuntimeConfig
	sessionSeq  uint64

	localBackend         *local.Local
	skillMiddleware      adk.ChatModelAgentMiddleware
	filesystemMiddleware adk.ChatModelAgentMiddleware
}

// ReActSkillRuntimeConfig defines ReAct skill loading configuration.
type ReActSkillRuntimeConfig struct {
	EnableBuiltinSkills bool
}

// DefaultReActSkillRuntimeConfig returns the default ReAct skill runtime configuration.
func DefaultReActSkillRuntimeConfig() ReActSkillRuntimeConfig {
	return defaultReActSkillRuntimeConfig()
}

func defaultReActSkillRuntimeConfig() ReActSkillRuntimeConfig {
	return ReActSkillRuntimeConfig{
		EnableBuiltinSkills: true,
	}
}

func normalizeReActSkillRuntimeConfig(cfg *ReActSkillRuntimeConfig) ReActSkillRuntimeConfig {
	out := defaultReActSkillRuntimeConfig()
	if cfg == nil {
		return out
	}
	out.EnableBuiltinSkills = cfg.EnableBuiltinSkills
	return out
}

// NewToolCallReActAnalyzer creates a ReAct+Skill risk analyzer.
func NewToolCallReActAnalyzer(ctx context.Context, chatModel model.ChatModel, language string, modelConfig *repository.SecurityModelConfig) (*ToolCallReActAnalyzer, error) {
	return NewToolCallReActAnalyzerWithConfig(ctx, chatModel, language, modelConfig, nil)
}

// NewToolCallReActAnalyzerWithConfig creates a ReAct+Skill risk analyzer with runtime config.
func NewToolCallReActAnalyzerWithConfig(ctx context.Context, chatModel model.ChatModel, language string, modelConfig *repository.SecurityModelConfig, cfg *ReActSkillRuntimeConfig) (*ToolCallReActAnalyzer, error) {
	analyzer := &ToolCallReActAnalyzer{
		language:    normalizeShepherdLanguage(language),
		chatModel:   chatModel,
		modelConfig: modelConfig,
		skillConfig: normalizeReActSkillRuntimeConfig(cfg),
	}
	if err := analyzer.rebuildAgent(ctx); err != nil {
		return nil, err
	}
	traceGuard("-", "Init", "ReAct+Skill analyzer initialized (ADK): skillsDir=%s", analyzer.skillsDir)
	return analyzer, nil
}

// Close releases analyzer resources.
func (a *ToolCallReActAnalyzer) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.skillMiddleware = nil
	a.filesystemMiddleware = nil
	a.localBackend = nil
	a.skillsDir = ""
}

// SetLanguage sets the analyzer language.
func (a *ToolCallReActAnalyzer) SetLanguage(language string) {
	a.mu.Lock()
	a.language = normalizeShepherdLanguage(language)
	a.mu.Unlock()
}

// SetAssetContext sets asset identity used for security event attribution.
func (a *ToolCallReActAnalyzer) SetAssetContext(assetName, assetID string) {
	a.mu.Lock()
	a.assetName = strings.TrimSpace(assetName)
	a.assetID = strings.TrimSpace(assetID)
	a.mu.Unlock()
}

// UpdateRuntimeConfig updates runtime config and rebuilds ADK middleware.
func (a *ToolCallReActAnalyzer) UpdateRuntimeConfig(ctx context.Context, cfg *ReActSkillRuntimeConfig) error {
	a.mu.Lock()
	a.skillConfig = normalizeReActSkillRuntimeConfig(cfg)
	a.mu.Unlock()
	return a.rebuildAgent(ctx)
}

func (a *ToolCallReActAnalyzer) rebuildAgent(ctx context.Context) error {
	a.mu.RLock()
	cfg := a.skillConfig
	oldDir := a.skillsDir
	a.mu.RUnlock()

	if a.chatModel == nil {
		return fmt.Errorf("chat model is nil")
	}

	skillsDir, err := prepareEffectiveSkillsWorkspace(cfg)
	if err != nil {
		return err
	}

	if oldDir == skillsDir {
		return nil
	}

	traceGuard("-", "ADK", "rebuilding ADK middleware: skillsDir=%s", skillsDir)

	localBackend, err := local.NewBackend(ctx, &local.Config{
		ValidateCommand: createGuardValidateCommand(),
	})
	if err != nil {
		return fmt.Errorf("create local backend failed: %w", err)
	}

	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: localBackend,
		BaseDir: skillsDir,
	})
	if err != nil {
		return fmt.Errorf("create skill backend failed: %w", err)
	}

	skillMw, err := skill.NewMiddleware(ctx, &skill.Config{
		Backend: skillBackend,
	})
	if err != nil {
		return fmt.Errorf("create skill middleware failed: %w", err)
	}

	fsMw, err := filesystem.New(ctx, &filesystem.MiddlewareConfig{
		Backend:             localBackend,
		StreamingShell:      localBackend,
		WriteFileToolConfig: &filesystem.ToolConfig{Disable: true},
		EditFileToolConfig:  &filesystem.ToolConfig{Disable: true},
	})
	if err != nil {
		return fmt.Errorf("create filesystem middleware failed: %w", err)
	}

	a.mu.Lock()
	a.localBackend = localBackend
	a.skillMiddleware = skillMw
	a.filesystemMiddleware = fsMw
	a.skillsDir = skillsDir
	a.mu.Unlock()

	names := listSkillDirNames(skillsDir)
	traceGuard("-", "ADK", "middleware ready, discovered %d skills: [%s]", len(names), strings.Join(names, ", "))
	return nil
}

func prepareEffectiveSkillsWorkspace(cfg ReActSkillRuntimeConfig) (string, error) {
	if !cfg.EnableBuiltinSkills {
		return "", fmt.Errorf("no skills available: builtin skills are disabled")
	}
	dir, err := ensureBundledReActSkillsReleased("")
	if err != nil {
		return "", fmt.Errorf("release bundled skills failed: %w", err)
	}
	if !hasAnySkill(dir) {
		return "", fmt.Errorf("no react guard skills in: %s", dir)
	}
	return dir, nil
}

func hasAnySkill(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); err == nil {
			return true
		}
	}
	return false
}

// ==================== Core analysis flow ====================

// Analyze performs risk analysis on tool_calls and their execution results.
func (a *ToolCallReActAnalyzer) Analyze(ctx context.Context, toolCalls []ToolCallInfo, toolResults []ToolResultInfo, rules *UserRules, requestID ...string) (*ReactRiskDecision, error) {
	if len(toolCalls) == 0 && len(toolResults) == 0 {
		return &ReactRiskDecision{
			Allowed:    true,
			Reason:     "No tool calls found.",
			RiskLevel:  "low",
			Confidence: 100,
		}, nil
	}

	reqID := ""
	if len(requestID) > 0 {
		reqID = requestID[0]
	}
	sessionID := a.nextAnalyzeSessionID()
	traceGuard(sessionID, "Analyze", "started: toolCalls=%d, toolResults=%d, tools=%s",
		len(toolCalls), len(toolResults), summarizeToolCallsForLog(toolCalls))

	a.mu.RLock()
	chatModel := a.chatModel
	modelConfig := a.modelConfig
	skillMw := a.skillMiddleware
	fsMw := a.filesystemMiddleware
	language := a.language
	assetName := a.assetName
	assetID := a.assetID
	a.mu.RUnlock()

	if chatModel == nil || skillMw == nil || fsMw == nil {
		return nil, fmt.Errorf("analyzer not initialized")
	}

	nestedUsage := &usageAccumulator{}
	customTools := []tool.BaseTool{
		NewRecordSecurityEventTool(assetName, assetID, reqID),
		newScanSkillSecurityTool(modelConfig, nestedUsage.Add),
	}

	systemPrompt := a.buildGuardSystemPrompt(rules, language)

	execCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	toolLogMw := newGuardToolLogMiddleware(sessionID)
	adkAgent, err := adk.NewChatModelAgent(execCtx, &adk.ChatModelAgentConfig{
		Name:          "shepherd_gate_guard",
		Description:   "ClawSecbot security risk analyzer for AI Agent tool calls",
		Instruction:   systemPrompt,
		Model:         chatModel,
		MaxIterations: 25,
		Handlers:      []adk.ChatModelAgentMiddleware{fsMw, skillMw, toolLogMw},
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: customTools,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create ADK agent failed: %w", err)
	}

	userMessage := buildGuardAgentInput(toolCalls, toolResults, rules, language)
	traceGuard(sessionID, "ADK", "prompt built: systemPromptLen=%d", len(systemPrompt))

	logging.ShepherdGateInfo("%s[react][Analyze][%s] prompt_summary: system_prompt_len=%d user_payload_len=%d",
		shepherdFlowLogPrefix, sessionID, len(systemPrompt), len(userMessage))

	traceGuard(sessionID, "ADK", "starting progressive skill analysis")

	input := &adk.AgentInput{
		Messages: []*schema.Message{
			schema.UserMessage(userMessage),
		},
	}
	callbackCollector, callbackHandler := newCallbackUsageCollector()
	execCtx = callbacks.InitCallbacks(execCtx, &callbacks.RunInfo{
		Name:      "shepherd_gate_guard_model_usage",
		Type:      "chat_model",
		Component: "model",
	}, callbackHandler)
	iter := adkAgent.Run(execCtx, input)

	var lastContent string
	var lastErr error
	adkUsage := &Usage{}
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			lastErr = event.Err
			continue
		}
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			continue
		}
		if msgUsage := usageFromMessageMetadata(msg); msgUsage != nil {
			adkUsage = mergeUsage(adkUsage, msgUsage)
		}
		if msg != nil && msg.Content != "" {
			lastContent = msg.Content
		}
	}

	modelUsage := callbackCollector.Snapshot()
	if modelUsage == nil {
		modelUsage = adkUsage
	}
	fallbackUsage := &Usage{
		PromptTokens:     estimateStringTokens(systemPrompt + userMessage),
		CompletionTokens: estimateStringTokens(lastContent),
		TotalTokens:      estimateStringTokens(systemPrompt+userMessage) + estimateStringTokens(lastContent),
	}
	analysisUsage := mergeUsage(usageWithFallbackFloor(modelUsage, fallbackUsage), nestedUsage.Snapshot())

	if lastErr != nil && lastContent == "" {
		traceGuard(sessionID, "ADK", "agent execution error=%v", lastErr)
		return nil, newUsageError(fmt.Errorf("ADK agent execution failed: %w", lastErr), analysisUsage)
	}

	traceGuard(sessionID, "ADK", "agent completed, output_len=%d", len(lastContent))
	logging.ShepherdGateInfo("%s[react][Analyze][%s] adk_agent_output_len=%d", shepherdFlowLogPrefix, sessionID, len(lastContent))

	parsed, ok := parseReactRiskDecision(lastContent)
	if !ok {
		traceGuard(sessionID, "ADK", "output parse failed")
		return nil, newUsageError(fmt.Errorf("ADK agent output not parseable"), analysisUsage)
	}
	parsed = normalizeReactRiskDecisionConsistency(parsed)
	parsed.Skill = "progressive_guard"
	parsed.Usage = analysisUsage
	traceGuard(sessionID, "Analyze", "decision: allowed=%v, risk=%s, confidence=%d, reason=%s",
		parsed.Allowed, parsed.RiskLevel, parsed.Confidence, shortenForLog(parsed.Reason, 300))
	logging.Info("%s[react][Analyze][%s] final decision: allowed=%v, skill=%s, risk=%s, confidence=%d, reason=%s",
		shepherdFlowLogPrefix,
		sessionID, parsed.Allowed, parsed.Skill, parsed.RiskLevel, parsed.Confidence, shortenForLog(parsed.Reason, 300))

	return parsed, nil
}

// ==================== Prompt building ====================

func (a *ToolCallReActAnalyzer) buildGuardSystemPrompt(rules *UserRules, language string) string {
	return renderPromptTemplate(
		toolCallGuardSystemPromptTemplate,
		"{{LANGUAGE}}", securityAnalysisLanguageName(language),
		"{{SEMANTIC_RULES_SECTION}}", buildSemanticRulesPromptSection(semanticRulesForPromptStages(rules, []string{"tool_call", "tool_call_result"}, true), "tool_call or tool_result"),
	)
}

func buildGuardAgentInput(toolCalls []ToolCallInfo, toolResults []ToolResultInfo, rules *UserRules, language string) string {
	payload := map[string]interface{}{
		"tool_calls": toolCalls,
		"language":   language,
		"output_schema": map[string]interface{}{
			"allowed":     "boolean",
			"reason":      "string",
			"risk_level":  "low|medium|high|critical",
			"confidence":  "0-100",
			"action_desc": "brief description of the tool action in user's language",
			"risk_type":   "risk enum, empty string if safe",
		},
	}
	if len(toolResults) > 0 {
		payload["tool_results"] = toolResults
	}
	if rules != nil && len(rules.SemanticRules) > 0 {
		payload["semantic_rules"] = rules.SemanticRules
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func semanticRulesForPromptStages(rules *UserRules, stages []string, includeCustomRegardlessOfStage bool) []SemanticRule {
	if rules == nil || len(rules.SemanticRules) == 0 {
		return nil
	}
	out := make([]SemanticRule, 0, len(rules.SemanticRules))
	for _, rule := range rules.SemanticRules {
		if !rule.Enabled {
			continue
		}
		if semanticRuleAppliesToAnyStage(rule, stages) || (includeCustomRegardlessOfStage && isCustomSemanticRule(rule)) {
			out = append(out, rule)
		}
	}
	return out
}

func semanticRuleAppliesToAnyStage(rule SemanticRule, stages []string) bool {
	if len(rule.AppliesTo) == 0 || len(stages) == 0 {
		return true
	}
	for _, appliesTo := range rule.AppliesTo {
		normalizedAppliesTo := strings.TrimSpace(strings.ToLower(appliesTo))
		for _, stage := range stages {
			if normalizedAppliesTo == strings.TrimSpace(strings.ToLower(stage)) {
				return true
			}
		}
	}
	return false
}

func isCustomSemanticRule(rule SemanticRule) bool {
	scope := strings.TrimSpace(strings.ToLower(rule.Scope))
	id := strings.TrimSpace(strings.ToLower(rule.ID))
	return scope == "custom" || strings.HasPrefix(id, "custom_") || strings.HasPrefix(id, "user_rule_")
}

func writeSemanticRulesPromptSection(sb *strings.Builder, rules []SemanticRule, stageLabel string) {
	if len(rules) == 0 {
		return
	}
	sb.WriteString("\n\n## User-Defined Semantic Rules\n")
	sb.WriteString("Enabled semantic rules are natural-language risk criteria, not keyword lists. Do not match them by substring only; judge whether the ")
	sb.WriteString(stageLabel)
	sb.WriteString(" semantically violates the rule description.\n")
	sb.WriteString("User-defined semantic rules take precedence when they apply:\n")
	for _, rule := range rules {
		sb.WriteString("- ")
		if rule.Description != "" {
			sb.WriteString(rule.Description)
		} else {
			sb.WriteString(rule.ID)
		}
		if len(rule.AppliesTo) > 0 {
			sb.WriteString(" | applies_to=")
			sb.WriteString(strings.Join(rule.AppliesTo, ","))
		}
		if rule.Action != "" {
			sb.WriteString(" | action=")
			sb.WriteString(rule.Action)
		}
		if rule.RiskType != "" {
			sb.WriteString(" | risk_type=")
			sb.WriteString(rule.RiskType)
		}
		sb.WriteString("\n")
	}
}

func buildSemanticRulesPromptSection(rules []SemanticRule, stageLabel string) string {
	var sb strings.Builder
	writeSemanticRulesPromptSection(&sb, rules, stageLabel)
	return sb.String()
}

func (a *ToolCallReActAnalyzer) nextAnalyzeSessionID() string {
	next := atomic.AddUint64(&a.sessionSeq, 1)
	return fmt.Sprintf("guard_session_%d", next)
}

type guardToolLogMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	sessionID string
}

func newGuardToolLogMiddleware(sessionID string) adk.ChatModelAgentMiddleware {
	return &guardToolLogMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		sessionID:                    sessionID,
	}
}

func (m *guardToolLogMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	if tCtx == nil {
		return endpoint, nil
	}
	toolName := tCtx.Name
	callID := tCtx.CallID
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		extra := formatToolLogExtra(toolName, argumentsInJSON)
		traceGuard(m.sessionID, "ToolCall", "call id=%s tool=%s%s args=%s",
			callID, toolName, extra, shortenForLog(argumentsInJSON, 240))

		result, err := endpoint(ctx, argumentsInJSON, opts...)
		if err != nil {
			traceGuard(m.sessionID, "ToolCall", "error id=%s tool=%s%s err=%v",
				callID, toolName, extra, err)
			return result, err
		}

		traceGuard(m.sessionID, "ToolCall", "result id=%s tool=%s%s output=%s",
			callID, toolName, extra, shortenForLog(result, 320))
		return result, nil
	}, nil
}

func (m *guardToolLogMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	if tCtx == nil {
		return endpoint, nil
	}
	toolName := tCtx.Name
	callID := tCtx.CallID
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		extra := formatToolLogExtra(toolName, argumentsInJSON)
		traceGuard(m.sessionID, "ToolCall", "stream call id=%s tool=%s%s args=%s",
			callID, toolName, extra, shortenForLog(argumentsInJSON, 240))

		reader, err := endpoint(ctx, argumentsInJSON, opts...)
		if err != nil {
			traceGuard(m.sessionID, "ToolCall", "stream error id=%s tool=%s%s err=%v",
				callID, toolName, extra, err)
			return nil, err
		}

		traceGuard(m.sessionID, "ToolCall", "stream opened id=%s tool=%s%s",
			callID, toolName, extra)
		return reader, nil
	}, nil
}

func extractSkillToolName(argumentsInJSON string) string {
	var payload struct {
		Skill string `json:"skill"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Skill)
}

func displaySkillName(skillName string) string {
	if strings.TrimSpace(skillName) == "" {
		return "<unknown>"
	}
	return skillName
}

func formatToolLogExtra(toolName, argumentsInJSON string) string {
	if toolName != "skill" {
		return ""
	}
	return fmt.Sprintf(" skill=%s", displaySkillName(extractSkillToolName(argumentsInJSON)))
}

// ==================== Output parsing ====================

func parseReactRiskDecision(output string) (*ReactRiskDecision, bool) {
	type response struct {
		Allowed    *bool  `json:"allowed"`
		Reason     string `json:"reason"`
		RiskLevel  string `json:"risk_level"`
		Confidence int    `json:"confidence"`
		ActionDesc string `json:"action_desc"`
		RiskType   string `json:"risk_type"`
	}

	cleaned := extractJSON(output)
	var r response
	if err := json.Unmarshal([]byte(cleaned), &r); err != nil {
		re := regexp.MustCompile(`\{[\s\S]*"allowed"[\s\S]*\}`)
		match := re.FindString(output)
		if match == "" {
			return nil, false
		}
		if err := json.Unmarshal([]byte(extractJSON(match)), &r); err != nil {
			return nil, false
		}
	}
	if r.Allowed == nil {
		return nil, false
	}

	if r.Confidence < 0 {
		r.Confidence = 0
	}
	if r.Confidence > 100 {
		r.Confidence = 100
	}

	reason := strings.TrimSpace(r.Reason)
	if reason == "" {
		if *r.Allowed {
			reason = "Allowed by ReAct skill analyzer."
		} else {
			reason = "Risk detected by ReAct skill analyzer."
		}
	}

	return &ReactRiskDecision{
		Allowed:    *r.Allowed,
		Reason:     reason,
		RiskLevel:  strings.TrimSpace(strings.ToLower(r.RiskLevel)),
		Confidence: r.Confidence,
		ActionDesc: strings.TrimSpace(r.ActionDesc),
		RiskType:   strings.TrimSpace(r.RiskType),
	}, true
}

// normalizeReactRiskDecisionConsistency 统一修正模型输出中的低风险判定一致性。
func normalizeReactRiskDecisionConsistency(decision *ReactRiskDecision) *ReactRiskDecision {
	if decision == nil {
		return nil
	}

	if decision.RiskLevel == "low" && !decision.Allowed && !isSemanticRuleBlockDecision(decision) {
		decision.Allowed = true
		if strings.TrimSpace(decision.Reason) == "" {
			decision.Reason = "Low-risk decisions should be allowed."
		} else {
			decision.Reason = decision.Reason + " [normalized: low-risk decision forced to allow]"
		}
	}

	return decision
}

// isSemanticRuleBlockDecision checks whether the block is caused by a user semantic rule.
func isSemanticRuleBlockDecision(decision *ReactRiskDecision) bool {
	if decision == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(decision.Reason + " " + decision.RiskType))
	if text == "" {
		return false
	}
	return strings.Contains(text, "semantic rule") ||
		strings.Contains(text, "user-defined rule") ||
		strings.Contains(text, "用户规则") ||
		strings.Contains(text, "语义规则")
}

// ==================== Helper functions ====================

func summarizeToolCallsForLog(toolCalls []ToolCallInfo) string {
	if len(toolCalls) == 0 {
		return "none"
	}
	var parts []string
	for i, tc := range toolCalls {
		if i >= 6 {
			parts = append(parts, fmt.Sprintf("...+%d more", len(toolCalls)-i))
			break
		}
		parts = append(parts, tc.Name)
	}
	return strings.Join(parts, ", ")
}

func shortenForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func listSkillDirNames(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); statErr != nil {
			continue
		}
		result = append(result, entry.Name())
	}
	return result
}

func containsAny(content string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(content, strings.ToLower(w)) {
			return true
		}
	}
	return false
}
