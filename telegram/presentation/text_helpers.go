package presentation

import (
	utils "github.com/yurika0211/luckyagent-tools/textutil"
)

func TruncateString(s string, maxLen int) string {
	return utils.TruncateKeepLength(s, maxLen)
}
