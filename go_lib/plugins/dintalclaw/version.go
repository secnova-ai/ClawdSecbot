package dintalclaw

import (
	"strconv"
	"strings"
)

func compareDintalclawVersion(current, target string) (int, bool) {
	parse := func(value string) ([3]int, bool) {
		parts := strings.Split(strings.TrimSpace(value), ".")
		if len(parts) != 3 {
			return [3]int{}, false
		}
		var result [3]int
		for i := 0; i < len(parts); i++ {
			v, err := strconv.Atoi(parts[i])
			if err != nil {
				return [3]int{}, false
			}
			result[i] = v
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
