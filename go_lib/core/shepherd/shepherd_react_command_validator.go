package shepherd

import (
	"fmt"
	"strings"

	"go_lib/core/logging"
)

// createGuardValidateCommand validates shell commands used by ADK filesystem middleware.
// It is intentionally strict and only allows read-only command family.
func createGuardValidateCommand() func(string) error {
	whitelist := map[string]struct{}{
		"cat":     {},
		"head":    {},
		"tail":    {},
		"grep":    {},
		"sed":     {},
		"awk":     {},
		"wc":      {},
		"ls":      {},
		"file":    {},
		"strings": {},
		"echo":    {},
	}

	return func(command string) error {
		command = strings.TrimSpace(command)
		if command == "" {
			return fmt.Errorf("command is required")
		}

		for _, op := range []string{"|", ">", "<", ";", "&&", "||", "`", "$("} {
			if strings.Contains(command, op) {
				logging.ShepherdGateWarning("%s[react][ValidateCommand] blocked: forbidden_operator=%q command=%s",
					shepherdFlowLogPrefix, op, command)
				return fmt.Errorf("command contains forbidden shell operator '%s'", op)
			}
		}

		fields := strings.Fields(command)
		if len(fields) == 0 {
			return fmt.Errorf("empty command")
		}
		baseCmd := fields[0]
		if _, ok := whitelist[baseCmd]; !ok {
			logging.ShepherdGateWarning("%s[react][ValidateCommand] rejected non_whitelisted_command=%s",
				shepherdFlowLogPrefix, baseCmd)
			return fmt.Errorf("command '%s' is not in whitelist", baseCmd)
		}

		return nil
	}
}
