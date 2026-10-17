package telegram

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yurika0211/luckyagent-gateway"
	"regexp"
	"strings"
	"testing"
)

func TestTelegramCommandSpecsIncludeTUICompatibleCommands(t *testing.T) {
	allowedName := regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
	seen := make(map[string]struct{})
	for _, spec := range telegramCommandSpecs() {
		if !allowedName.MatchString(spec.Command) {
			t.Fatalf("command %q does not satisfy Telegram command naming rules", spec.Command)
		}
		seen[spec.Command] = struct{}{}
	}
	for _, name := range []string{
		"review",
		"init",
		"config",
		"version",
		"rename",
		"chat",
		"models",
		"rag",
		"context",
		"fc",
		"embedder",
		"remember",
		"remember_long",
		"recall",
		"memstats",
		"memdecay",
		"promote",
		"mcp",
		"approve",
		"deny",
		"cron",
		"watch",
		"msg_gateway",
		"profile",
	} {
		if _, ok := seen[name]; !ok {
			t.Fatalf("expected Telegram command metadata to include %q", name)
		}
	}
	for _, localOnly := range []string{"exit", "quit", "q", "clear", "jobs", "lh", "serve", "msg-gateway", "remember-long"} {
		if _, ok := seen[localOnly]; ok {
			t.Fatalf("local-only TUI command %q should not be exposed as Telegram bot command", localOnly)
		}
	}
}

func TestTelegramBotCommandsEncodeAllSupportedCommands(t *testing.T) {
	data, err := json.Marshal(telegramBotCommands())
	require.NoError(t, err)
	payload := string(data)
	for _, name := range telegramCommandNames() {
		assert.Contains(t, payload, `"command":"`+name+`"`)
	}
}

// TestHandleMessageCommand verifies command routing.
func TestHandleMessageCommand(t *testing.T) {
	msg := &gateway.Message{
		ID:        "1",
		Chat:      gateway.Chat{ID: "123", Type: gateway.ChatPrivate},
		Sender:    gateway.User{ID: "1"},
		Text:      "/start",
		IsCommand: true,
		Command:   "start",
	}

	assert.True(t, msg.IsCommand)
	assert.Equal(t, "start", msg.Command)
}

// TestHandleMessagePrivateNonCommand verifies private chat routing.
func TestHandleMessagePrivateNonCommand(t *testing.T) {
	msg := &gateway.Message{
		ID:        "1",
		Chat:      gateway.Chat{ID: "123", Type: gateway.ChatPrivate},
		Sender:    gateway.User{ID: "1"},
		Text:      "Hello, bot!",
		IsCommand: false,
	}

	assert.Equal(t, gateway.ChatPrivate, msg.Chat.Type)
	assert.False(t, msg.IsCommand)
	assert.Equal(t, "Hello, bot!", msg.Text)
}

// TestHandleMessageGroupNonCommand verifies group chat routing.
func TestHandleMessageGroupNonCommand(t *testing.T) {
	msg := &gateway.Message{
		ID:        "1",
		Chat:      gateway.Chat{ID: "-100", Type: gateway.ChatGroup},
		Sender:    gateway.User{ID: "1"},
		Text:      "Hello, everyone!",
		IsCommand: false,
	}

	assert.Equal(t, gateway.ChatGroup, msg.Chat.Type)
	assert.False(t, msg.IsCommand)
}

// TestCommandRouting verifies all known commands are recognized.
func TestCommandRouting(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    string
	}{
		{"start", "start", ""},
		{"help", "help", ""},
		{"model no args", "model", ""},
		{"model with args", "model", "gpt-4"},
		{"soul", "soul", ""},
		{"tools", "tools", ""},
		{"reset", "reset", ""},
		{"history", "history", ""},
		{"session", "session", ""},
		{"chat with args", "chat", "hello world"},
		{"unknown", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &gateway.Message{
				ID:        "1",
				Chat:      gateway.Chat{ID: "123", Type: gateway.ChatPrivate},
				Sender:    gateway.User{ID: "1", Username: "testuser"},
				Text:      "/" + tt.command + " " + tt.args,
				IsCommand: true,
				Command:   tt.command,
				Args:      strings.TrimSpace(tt.args),
			}
			assert.NotNil(t, msg)
			assert.Equal(t, tt.command, msg.Command)
		})
	}
}

// TestWelcomeMessage verifies the welcome message content.
func TestWelcomeMessage(t *testing.T) {
	welcome := `🍀 *LuckyAgent Bot*

I'm an AI assistant powered by LuckyAgent.

*Available commands:*
/chat _message_ — Send a message to the AI
/model — Show current model
/soul — Show current SOUL info
/tools — List available tools
/reset — Reset conversation
/history — Show conversation history
/session — Show current session info
/help — Show this help

You can also just type a message directly!`

	assert.Contains(t, welcome, "LuckyAgent")
	assert.Contains(t, welcome, "/chat")
	assert.Contains(t, welcome, "/help")
	assert.Contains(t, welcome, "/model")
	assert.Contains(t, welcome, "/history")
	assert.Contains(t, welcome, "/session")
}

// TestHelpMessage verifies the help message content.
func TestHelpMessage(t *testing.T) {
	help := `*Available Commands:*

/start — Welcome message
/help — This help message
/chat _message_ — Send a message to the AI
/model \[name] — Get/set current model
/soul — Show current SOUL info
/tools — List available tools
/reset — Reset conversation
/history — Show conversation history
/session — Show current session info

*Tips:*
• In private chats, just type your message directly
• In groups, mention @bot or reply to a bot message
• Each chat has its own conversation session`

	assert.Contains(t, help, "/start")
	assert.Contains(t, help, "/chat")
	assert.Contains(t, help, "/model")
	assert.Contains(t, help, "/soul")
	assert.Contains(t, help, "/tools")
	assert.Contains(t, help, "/reset")
	assert.Contains(t, help, "/history")
	assert.Contains(t, help, "/session")
}

// TestGatewayMessageTypes verifies message type construction.
func TestGatewayMessageTypes(t *testing.T) {
	// Test ChatType
	assert.Equal(t, "private", gateway.ChatPrivate.String())
	assert.Equal(t, "group", gateway.ChatGroup.String())
	assert.Equal(t, "supergroup", gateway.ChatSuperGroup.String())
	assert.Equal(t, "channel", gateway.ChatChannel.String())

	// Test User.DisplayName
	u := gateway.User{ID: "1", Username: "test", FirstName: "Test", LastName: "User"}
	assert.Equal(t, "@test", u.DisplayName())

	u2 := gateway.User{ID: "2", FirstName: "Test", LastName: "User"}
	assert.Equal(t, "Test User", u2.DisplayName())

	u3 := gateway.User{ID: "3", FirstName: "Test"}
	assert.Equal(t, "Test", u3.DisplayName())

	u4 := gateway.User{ID: "4"}
	assert.Equal(t, "4", u4.DisplayName())
}
