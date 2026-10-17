package qqofficial

const qqProgressTraceChunkLimit = 1800

func splitQQMessageChunks(text string, limit int) []string {
	if text == "" {
		return nil
	}
	if limit <= 0 || qqRuneLen(text) <= limit {
		return []string{text}
	}

	runes := []rune(text)
	chunks := make([]string, 0, len(runes)/limit+1)
	for len(runes) > limit {
		cut := limit
		// Prefer a natural line boundary but retain that newline in the
		// preceding chunk so concatenating chunks recreates the source exactly.
		for i := limit; i > 0; i-- {
			if runes[i-1] == '\n' {
				cut = i
				break
			}
		}
		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

func qqRuneLen(text string) int {
	return len([]rune(text))
}

var SplitQQMessageChunks = splitQQMessageChunks
var QqRuneLen = qqRuneLen

const QqProgressTraceChunkLimit = qqProgressTraceChunkLimit
