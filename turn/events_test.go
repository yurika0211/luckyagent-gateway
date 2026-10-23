package turn

import (
	"testing"
	"time"
)

func TestChatEventCarriesToolDuration(t *testing.T) {
	event := ChatEvent{
		Type:     ChatEventToolResult,
		Name:     "terminal",
		Result:   "command timed out",
		Duration: 30 * time.Second,
	}
	if event.Type != ChatEventToolResult || event.Duration != 30*time.Second {
		t.Fatalf("event = %#v", event)
	}
	unmeasured := ChatEvent{Type: ChatEventToolCall, Name: "terminal"}
	if unmeasured.Duration != 0 {
		t.Fatal("a tool call must not invent a duration")
	}
}
