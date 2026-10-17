package telegram

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestReactToMessageDoesNotRequireRunningFlag(t *testing.T) {
	reactionSeen := make(chan url.Values, 1)
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "setMessageReaction") {
			_ = r.ParseForm()
			reactionSeen <- r.Form
		}
		return defaultMockBotResponse(r)
	})
	require.NoError(t, err)

	adapter := NewAdapter(Config{Token: bot.Token})
	adapter.bot = bot
	require.False(t, adapter.IsRunning())

	adapter.ReactToMessage("-100123", "42", "👍")

	select {
	case form := <-reactionSeen:
		assert.Equal(t, "-100123", form.Get("chat_id"))
		assert.Equal(t, "42", form.Get("message_id"))
		assert.Contains(t, form.Get("reaction"), `"emoji":"👍"`)
	case <-time.After(time.Second):
		t.Fatal("expected reaction without running flag")
	}
}
