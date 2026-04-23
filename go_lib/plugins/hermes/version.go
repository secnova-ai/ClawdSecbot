package hermes

import (
	"regexp"
	"strconv"
	"strings"

	"go_lib/core/cmdutil"
)

var hermesVersionPattern = regexp.MustCompile(`\bv?(\d+)\.(\d+)\.(\d+)\b`)

func getHermesVersion() string {
	cmd := cmdutil.Command("hermes", "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	match := hermesVersionPattern.FindStringSubmatch(strings.TrimSpace(string(output)))
	if len(match) != 4 {
		return ""
	}
	return match[1] + "." + match[2] + "." + match[3]
}

func compareHermesVersion(current, target string) (int, bool) {
	parse := func(value string) ([3]int, bool) {
		match := hermesVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
		if len(match) != 4 {
			return [3]int{}, false
		}
		var result [3]int
		for i := 1; i <= 3; i++ {
			v, err := strconv.Atoi(match[i])
			if err != nil {
				return [3]int{}, false
			}
			result[i-1] = v
		}
		return result, true
	}

	left, ok := parse(current)
	if !ok {
		return 0, false
	}
	right, ok := parse(target)
	if !ok {
		return 0, false
	}

	for i := 0; i < len(left); i++ {
		switch {
		case left[i] < right[i]:
			return -1, true
		case left[i] > right[i]:
			return 1, true
		}
	}
	return 0, true
}
