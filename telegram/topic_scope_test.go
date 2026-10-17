package telegram

import (
	"github.com/yurika0211/luckyagent-gateway"
	"testing"
)

func TestTelegramConversationScopeIsolatesTopics(t *testing.T) {
	base := gateway.Chat{ID: "-100123", Type: gateway.ChatSuperGroup}
	general := &gateway.Message{Chat: base}
	topicA := &gateway.Message{Chat: base, ThreadID: "42"}
	topicB := &gateway.Message{Chat: base, ThreadID: "84"}

	if got := telegramConversationScope(general); got != "-100123" {
		t.Fatalf("general scope = %q, want chat ID", got)
	}
	if got, want := telegramConversationScope(topicA), "-100123::thread::42"; got != want {
		t.Fatalf("topic A scope = %q, want %q", got, want)
	}
	if got, want := telegramConversationScope(topicB), "-100123::thread::84"; got != want {
		t.Fatalf("topic B scope = %q, want %q", got, want)
	}
	if telegramConversationScope(topicA) == telegramConversationScope(topicB) {
		t.Fatal("distinct Telegram topics share a conversation scope")
	}
}
