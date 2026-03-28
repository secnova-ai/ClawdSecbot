package nullclaw

// core_imports.go provides type aliases and function wrappers for types/functions
// that have been migrated from nullclaw to core packages.
// This allows remaining nullclaw code and tests to compile without mass refactoring.

import (
	"go_lib/core/modelfactory"
	"go_lib/core/proxy"
	"go_lib/core/shepherd"
	"go_lib/core/skillscan"
)

// ==================== Type aliases from core/proxy ====================

type ProxyProtection = proxy.ProxyProtection
type BotModelConfig = proxy.BotModelConfig
type ProtectionConfig = proxy.ProtectionConfig
type ProtectionRuntimeConfig = proxy.ProtectionRuntimeConfig
type LogMessage = proxy.LogMessage
type TruthRecord = proxy.TruthRecord
type RecordToolCall = proxy.RecordToolCall
type ProxyStartResponse = proxy.ProxyStartResponse
type ProxyStatusResponse = proxy.ProxyStatusResponse

// ==================== Type aliases from core/shepherd ====================

type ShepherdGate = shepherd.ShepherdGate
type ShepherdDecision = shepherd.ShepherdDecision
type ConversationMessage = shepherd.ConversationMessage
type ToolCallInfo = shepherd.ToolCallInfo
type ToolResultInfo = shepherd.ToolResultInfo
type UserRules = shepherd.UserRules
type SecurityEvent = shepherd.SecurityEvent
type ReActSkillRuntimeConfig = shepherd.ReActSkillRuntimeConfig

// ==================== Type aliases from core/skillscan ====================

type SkillSecurityAnalyzer = skillscan.SkillSecurityAnalyzer
type SkillAnalysisResult = skillscan.SkillAnalysisResult
type SkillSecurityIssue = skillscan.SkillSecurityIssue
type SkillInfo = skillscan.SkillInfo
type SkillScanResult = skillscan.SkillScanResult

// ==================== Function wrappers from core/modelfactory ====================

var ValidateSecurityModelConfig = modelfactory.ValidateSecurityModelConfig
var CreateChatModelFromConfig = modelfactory.CreateChatModelFromConfig

// ==================== Function wrappers from core/skillscan ====================

var NewSkillSecurityAnalyzer = skillscan.NewSkillSecurityAnalyzer
var EnsureScanSkillsReleased = skillscan.EnsureScanSkillsReleased
var ScanSkillForPromptInjection = skillscan.ScanSkillForPromptInjection
var detectPromptInjectionPatterns = skillscan.DetectPromptInjectionPatterns
var listSkillsInDir = skillscan.ListSkillsInDir

// calculateSkillHash wraps skillscan.CalculateSkillHash (lowercase -> exported)
var calculateSkillHash = skillscan.CalculateSkillHash

// ==================== Function wrappers from core/shepherd ====================

var GetSecurityEventBuffer = shepherd.GetSecurityEventBuffer

// ==================== Function wrappers from core/proxy ====================

var NewProxyProtectionFromConfig = proxy.NewProxyProtectionFromConfig
var GetProxyProtection = proxy.GetProxyProtection
var GetProxyProtectionByAsset = proxy.GetProxyProtectionByAsset
var UpdateLanguage = proxy.UpdateLanguage

// toJSONString is a utility function used across multiple nullclaw files.
// It delegates to the proxy package's implementation.
func toJSONString(v interface{}) string {
	return proxy.ToJSONString(v)
}
