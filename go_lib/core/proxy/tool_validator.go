package proxy

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

// ToolValidationMode defines the validation mode
type ToolValidationMode string

const (
	ModeWhitelist ToolValidationMode = "whitelist" // Only allow tools in the list
	ModeBlacklist ToolValidationMode = "blacklist" // Block tools in the list
	ModeDisabled  ToolValidationMode = "disabled"  // No validation
)

// ToolRule defines a rule for tool validation
type ToolRule struct {
	// ToolName is the name or pattern to match (supports wildcards like "file_*")
	ToolName string `json:"tool_name"`
	// ArgumentPatterns are regex patterns to match against arguments (optional)
	// If specified, all patterns must match for the rule to apply
	ArgumentPatterns map[string]string `json:"argument_patterns,omitempty"`
	// Description explains why this rule exists
	Description string `json:"description,omitempty"`
	// RiskLevel indicates the risk level if this tool is used
	RiskLevel string `json:"risk_level,omitempty"`
}

// ToolValidatorConfig holds the configuration for tool validation
type ToolValidatorConfig struct {
	Mode           ToolValidationMode `json:"mode"`
	Rules          []ToolRule         `json:"rules"`
	DefaultAllow   bool               `json:"default_allow"`   // For blacklist mode: allow by default
	EnableLogging  bool               `json:"enable_logging"`  // Log validation results
	SensitiveTools []string           `json:"sensitive_tools"` // Tools that require extra scrutiny
}

// ToolValidationResult represents the result of tool validation
type ToolValidationResult struct {
	Allowed     bool     `json:"allowed"`
	Reason      string   `json:"reason"`
	MatchedRule *ToolRule `json:"matched_rule,omitempty"`
	RiskLevel   string   `json:"risk_level,omitempty"`
}

// ToolValidator validates tool calls against configured rules
type ToolValidator struct {
	config  *ToolValidatorConfig
	mu      sync.RWMutex
	logChan chan string
	
	// Compiled regex patterns for performance
	compiledPatterns map[string]*regexp.Regexp
}

// NewToolValidator creates a new tool validator with default configuration
func NewToolValidator(logChan chan string) *ToolValidator {
	return &ToolValidator{
		config: &ToolValidatorConfig{
			Mode:          ModeDisabled,
			DefaultAllow:  true,
			EnableLogging: true,
			SensitiveTools: []string{
				"execute_command", "run_shell", "bash", "shell",
				"file_write", "file_delete", "write_file", "delete_file",
				"file_patch",
				"eval", "exec", "system",
				"code_run",
				"send_email", "http_request", "fetch_url",
				"database_query", "sql_execute",
			},
		},
		logChan:          logChan,
		compiledPatterns: make(map[string]*regexp.Regexp),
	}
}

// Configure updates the validator configuration
func (tv *ToolValidator) Configure(config *ToolValidatorConfig) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	
	tv.config = config
	tv.compiledPatterns = make(map[string]*regexp.Regexp)
	
	// Pre-compile regex patterns for rules
	for _, rule := range config.Rules {
		for argName, pattern := range rule.ArgumentPatterns {
			key := rule.ToolName + ":" + argName
			if compiled, err := regexp.Compile(pattern); err == nil {
				tv.compiledPatterns[key] = compiled
			}
		}
	}
}

// ValidateTool checks if a tool call is allowed
func (tv *ToolValidator) ValidateTool(toolName string, arguments string) *ToolValidationResult {
	tv.mu.RLock()
	defer tv.mu.RUnlock()
	
	if tv.config.Mode == ModeDisabled {
		return &ToolValidationResult{
			Allowed: true,
			Reason:  "validation disabled",
		}
	}
	
	// Parse arguments
	var args map[string]interface{}
	if arguments != "" {
		_ = json.Unmarshal([]byte(arguments), &args)
	}
	
	// Check against rules
	for _, rule := range tv.config.Rules {
		if tv.matchesRule(toolName, args, &rule) {
			if tv.config.Mode == ModeWhitelist {
				result := &ToolValidationResult{
					Allowed:     true,
					Reason:      "tool in whitelist",
					MatchedRule: &rule,
					RiskLevel:   rule.RiskLevel,
				}
				tv.logValidation(toolName, result)
				return result
			} else if tv.config.Mode == ModeBlacklist {
				result := &ToolValidationResult{
					Allowed:     false,
					Reason:      "tool in blacklist: " + rule.Description,
					MatchedRule: &rule,
					RiskLevel:   rule.RiskLevel,
				}
				tv.logValidation(toolName, result)
				return result
			}
		}
	}
	
	// No rule matched
	var result *ToolValidationResult
	if tv.config.Mode == ModeWhitelist {
		result = &ToolValidationResult{
			Allowed: false,
			Reason:  "tool not in whitelist",
		}
	} else {
		result = &ToolValidationResult{
			Allowed: tv.config.DefaultAllow,
			Reason:  "default policy applied",
		}
	}
	
	// Check if it's a sensitive tool
	if tv.isSensitiveTool(toolName) {
		result.RiskLevel = "high"
		if result.Reason == "default policy applied" {
			result.Reason = "sensitive tool - requires extra scrutiny"
		}
	}
	
	tv.logValidation(toolName, result)
	return result
}

// matchesRule checks if a tool call matches a rule
func (tv *ToolValidator) matchesRule(toolName string, args map[string]interface{}, rule *ToolRule) bool {
	// Check tool name (supports wildcards)
	if !tv.matchToolName(toolName, rule.ToolName) {
		return false
	}
	
	// Check argument patterns if specified
	if len(rule.ArgumentPatterns) > 0 && args != nil {
		for argName, pattern := range rule.ArgumentPatterns {
			argValue, exists := args[argName]
			if !exists {
				return false
			}
			
			argStr, ok := argValue.(string)
			if !ok {
				argStr = toString(argValue)
			}
			
			key := rule.ToolName + ":" + argName
			if compiled, exists := tv.compiledPatterns[key]; exists {
				if !compiled.MatchString(argStr) {
					return false
				}
			} else {
				// Fallback to simple pattern matching
				if !strings.Contains(argStr, pattern) {
					return false
				}
			}
		}
	}
	
	return true
}

// matchToolName matches a tool name against a pattern (supports wildcards)
func (tv *ToolValidator) matchToolName(toolName, pattern string) bool {
	// Exact match
	if toolName == pattern {
		return true
	}
	
	// Wildcard match (e.g., "file_*" matches "file_read", "file_write")
	if strings.Contains(pattern, "*") {
		regexPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), "\\*", ".*") + "$"
		matched, _ := regexp.MatchString(regexPattern, toolName)
		return matched
	}
	
	return false
}

// isSensitiveTool checks if a tool is in the sensitive tools list
func (tv *ToolValidator) isSensitiveTool(toolName string) bool {
	lowerName := strings.ToLower(toolName)
	for _, sensitive := range tv.config.SensitiveTools {
		if strings.Contains(lowerName, strings.ToLower(sensitive)) {
			return true
		}
	}
	return false
}

// logValidation sends validation result to log channel
func (tv *ToolValidator) logValidation(toolName string, result *ToolValidationResult) {
	if !tv.config.EnableLogging || tv.logChan == nil {
		return
	}
	
	var logMsg LogMessage
	if result.Allowed {
		logMsg = LogMessage{
			Key: "tool_validator_passed",
			Params: map[string]interface{}{
				"toolName": toolName,
			},
		}
	} else {
		logMsg = LogMessage{
			Key: "tool_validator_blocked",
			Params: map[string]interface{}{
				"reason": result.Reason,
			},
		}
	}
	
	jsonBytes, _ := json.Marshal(logMsg)
	select {
	case tv.logChan <- string(jsonBytes):
	default:
	}
}

// GetSensitiveTools returns the list of sensitive tools
func (tv *ToolValidator) GetSensitiveTools() []string {
	tv.mu.RLock()
	defer tv.mu.RUnlock()
	return tv.config.SensitiveTools
}

// IsSensitive checks if a tool name is sensitive
func (tv *ToolValidator) IsSensitive(toolName string) bool {
	tv.mu.RLock()
	defer tv.mu.RUnlock()
	return tv.isSensitiveTool(toolName)
}

// toString converts an interface to string
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}

// DefaultBlacklistRules returns a set of default blacklist rules for dangerous tools
func DefaultBlacklistRules() []ToolRule {
	return []ToolRule{
		{
			ToolName:    "execute_command",
			Description: "Direct command execution is dangerous",
			RiskLevel:   "critical",
		},
		{
			ToolName:    "run_shell",
			Description: "Shell execution can be exploited",
			RiskLevel:   "critical",
		},
		{
			ToolName:    "bash",
			Description: "Bash execution requires careful review",
			RiskLevel:   "high",
		},
		{
			ToolName:    "eval",
			Description: "Code evaluation is dangerous",
			RiskLevel:   "critical",
		},
		{
			ToolName:    "file_delete",
			Description: "File deletion can cause data loss",
			RiskLevel:   "high",
		},
		{
			ToolName:    "delete_file",
			Description: "File deletion can cause data loss",
			RiskLevel:   "high",
		},
		{
			ToolName:    "send_email",
			Description: "Email sending can be abused for spam/phishing",
			RiskLevel:   "medium",
		},
		{
			ToolName:    "database_query",
			ArgumentPatterns: map[string]string{
				"query": "(?i)(DROP|DELETE|TRUNCATE|ALTER)",
			},
			Description: "Destructive database operations",
			RiskLevel:   "critical",
		},
	}
}
