package nullclaw

import (
	"encoding/json"

	_ "embed"

	"go_lib/core"
)

//go:embed mitigation.json
var mitigationJSON []byte

//go:embed nullclaw.json
var nullclawRulesJSON []byte

// RiskTemplate defines the structure for risk mitigation templates
type RiskTemplate struct {
	RiskID     string           `json:"risk_id"`
	Mitigation *core.Mitigation `json:"mitigation"`
}

var templates map[string]*core.Mitigation

func init() {
	resetPluginSignals()

	var tempList []RiskTemplate
	if err := json.Unmarshal(mitigationJSON, &tempList); err == nil {
		templates = make(map[string]*core.Mitigation)
		for _, t := range tempList {
			templates[t.RiskID] = t.Mitigation
		}
	}
}

// GetMitigationTemplates 返回风险缓解模板
func GetMitigationTemplates() map[string]*core.Mitigation {
	return templates
}

// GetNullclawRulesJSON 返回 Nullclaw 规则 JSON
func GetNullclawRulesJSON() []byte {
	return nullclawRulesJSON
}
