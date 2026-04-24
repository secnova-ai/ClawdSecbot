package hermes

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"go_lib/core"
)

//go:embed mitigation.json
var mitigationJSON []byte

//go:embed vulinfo.json
var vulInfoJSON []byte

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

func GetVulInfoJSON() []byte {
	if data, ok := loadPluginVulInfoJSON(vulInfoJSON); ok {
		return data
	}
	return vulInfoJSON
}

func loadPluginVulInfoJSON(fallback []byte) ([]byte, bool) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return fallback, false
	}

	path := filepath.Join(filepath.Dir(currentFile), "vulinfo.json")
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return fallback, false
	}
	return data, true
}

// GetHermesRulesJSON returns Hermes discovery rules.
func GetHermesRulesJSON() []byte {
	return hermesRulesJSON
}
