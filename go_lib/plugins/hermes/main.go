package hermes

import (
	_ "embed"
	"encoding/json"

	"go_lib/core"
)

//go:embed mitigation.json
var mitigationJSON []byte

//go:embed hermes.json
var hermesRulesJSON []byte

// RiskTemplate defines the structure for risk mitigation templates.
type RiskTemplate struct {
	RiskID     string           `json:"risk_id"`
	Mitigation *core.Mitigation `json:"mitigation"`
}

var templates map[string]*core.Mitigation

func init() {
	var tempList []RiskTemplate
	if err := json.Unmarshal(mitigationJSON, &tempList); err == nil {
		templates = make(map[string]*core.Mitigation)
		for _, t := range tempList {
			templates[t.RiskID] = t.Mitigation
		}
	}
}

// GetMitigationTemplates returns risk mitigation templates.
func GetMitigationTemplates() map[string]*core.Mitigation {
	return templates
}

// GetHermesRulesJSON returns Hermes discovery rules.
func GetHermesRulesJSON() []byte {
	return hermesRulesJSON
}
