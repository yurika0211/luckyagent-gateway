package presentation

import (
	"fmt"
	"strings"
)

func ExtractRoundNumber(thinking string) int {
	var round int
	if _, err := fmt.Sscanf(strings.TrimSpace(thinking), "Thinking... (round %d)", &round); err == nil && round > 0 {
		return round
	}
	return 0
}
