package hermes

import (
	"encoding/json"
	"testing"
)

func TestEmbeddedRulesAndTemplates(t *testing.T) {
	rules := GetHermesRulesJSON()
	if len(rules) == 0 {
		t.Fatal("expected embedded hermes rules")
	}

	var parsedRules []map[string]interface{}
	if err := json.Unmarshal(rules, &parsedRules); err != nil {
		t.Fatalf("failed to parse embedded rules: %v", err)
	}
	if len(parsedRules) == 0 {
		t.Fatal("expected at least one embedded rule")
	}

	tpls := GetMitigationTemplates()
	if len(tpls) == 0 {
		t.Fatal("expected mitigation templates")
	}
	if tpl := tpls["approvals_mode_disabled"]; tpl == nil {
		t.Fatal("expected approvals_mode_disabled mitigation template")
	}
}
