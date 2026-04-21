package core

// RiskLevel 定义风险等级常量
const (
	RiskLevelLow      = "low"
	RiskLevelMedium   = "medium"
	RiskLevelHigh     = "high"
	RiskLevelCritical = "critical"
)

// Risk 定义通用的风险结构,所有插件返回的风险都应遵循此结构
type Risk struct {
	// ID 风险的唯一标识符（用于国际化和去重）
	ID string `json:"id"`
	// SourcePlugin identifies which plugin produced this risk (auto-injected by PluginManager)
	SourcePlugin string `json:"source_plugin,omitempty"`
	// AssetID identifies the concrete asset instance for mitigation routing.
	AssetID string `json:"asset_id,omitempty"`
	// Title 风险标题（默认英文,UI可根据ID覆盖）
	Title string `json:"title"`
	// Description 风险描述
	Description string `json:"description"`
	// Level 风险等级：low, medium, high, critical
	Level string `json:"level"`
	// Args 动态参数,用于在UI展示时填充描述中的占位符（如文件路径、端口号等）
	Args map[string]interface{} `json:"args,omitempty"`
	// Mitigation 风险处置机制
	Mitigation *Mitigation `json:"mitigation,omitempty"`
}

// Mitigation 定义风险处置的元数据
type Mitigation struct {
	Type        string            `json:"type"`                  // "form", "auto", "suggestion"
	FormSchema  []FormItem        `json:"form_schema"`           // 表单字段列表（用于form类型）
	Title       string            `json:"title,omitempty"`       // 建议标题（用于suggestion类型）
	Description string            `json:"description,omitempty"` // 建议描述（用于suggestion类型）
	Suggestions []SuggestionGroup `json:"suggestions,omitempty"` // 建议列表（用于suggestion类型）
}

// SuggestionGroup 定义建议组（按优先级分组）
type SuggestionGroup struct {
	Priority string           `json:"priority"` // P0, P1, P2
	Category string           `json:"category"` // 分类名称
	Items    []SuggestionItem `json:"items"`    // 建议项列表
}

// SuggestionItem 定义单个建议项
type SuggestionItem struct {
	Action  string `json:"action"`            // 操作说明
	Detail  string `json:"detail"`            // 详细说明
	Command string `json:"command,omitempty"` // 可选的命令示例
}

// FormItem 定义表单项
type FormItem struct {
	Key          string      `json:"key"`
	Label        string      `json:"label"`
	Type         string      `json:"type"` // "text", "boolean", "select", "password"
	DefaultValue interface{} `json:"default_value,omitempty"`
	Options      []string    `json:"options,omitempty"` // For select
	// Validation rules
	Required  bool   `json:"required,omitempty"`
	MinLength int    `json:"min_length,omitempty"`
	Regex     string `json:"regex,omitempty"`
	RegexMsg  string `json:"regex_msg,omitempty"`
}
