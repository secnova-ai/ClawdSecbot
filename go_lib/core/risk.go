package core

const (
	RiskLevelLow      = "low"
	RiskLevelMedium   = "medium"
	RiskLevelHigh     = "high"
	RiskLevelCritical = "critical"
)

type Risk struct {
	ID            string                 `json:"id"`
	SourcePlugin  string                 `json:"source_plugin,omitempty"`
	AssetID       string                 `json:"asset_id,omitempty"`
	Title         string                 `json:"title"`
	TitleEN       string                 `json:"title_en,omitempty"`
	Description   string                 `json:"description"`
	DescriptionEN string                 `json:"description_en,omitempty"`
	Level         string                 `json:"level"`
	Args          map[string]interface{} `json:"args,omitempty"`
	Mitigation    *Mitigation            `json:"mitigation,omitempty"`
}

type Mitigation struct {
	Type          string            `json:"type"`
	FormSchema    []FormItem        `json:"form_schema"`
	Title         string            `json:"title,omitempty"`
	TitleEN       string            `json:"title_en,omitempty"`
	Description   string            `json:"description,omitempty"`
	DescriptionEN string            `json:"description_en,omitempty"`
	Suggestions   []SuggestionGroup `json:"suggestions,omitempty"`
}

type SuggestionGroup struct {
	Priority string           `json:"priority"`
	Category string           `json:"category"`
	Items    []SuggestionItem `json:"items"`
}

type SuggestionItem struct {
	Action   string `json:"action"`
	ActionEN string `json:"action_en,omitempty"`
	Detail   string `json:"detail"`
	DetailEN string `json:"detail_en,omitempty"`
	Command  string `json:"command,omitempty"`
}

type FormItem struct {
	Key          string      `json:"key"`
	Label        string      `json:"label"`
	Type         string      `json:"type"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Options      []string    `json:"options,omitempty"`
	Required     bool        `json:"required,omitempty"`
	MinLength    int         `json:"min_length,omitempty"`
	Regex        string      `json:"regex,omitempty"`
	RegexMsg     string      `json:"regex_msg,omitempty"`
}
