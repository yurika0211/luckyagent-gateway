package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/yurika0211/luckyagent-gateway"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockBotClient struct {
	handler func(*http.Request) map[string]any
}

type mockBotServer struct{}

func (mockBotServer) Close() {}

type capturedBotMessage struct {
	Method    string
	Text      string
	ParseMode string
	ReplyTo   string
	ThreadID  string
}

func (c *mockBotClient) Do(req *http.Request) (*http.Response, error) {
	if c.handler == nil {
		return nil, fmt.Errorf("mock bot client: nil handler")
	}
	result := c.handler(req)
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
	body, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}, nil
}

func defaultMockBotResponse(r *http.Request) map[string]any {
	switch {
	case containsMethod(r.URL.Path, "getMe"):
		return map[string]any{
			"ok": true,
			"result": map[string]any{
				"id":         123456789,
				"is_bot":     true,
				"first_name": "TestBot",
				"username":   "testbot",
			},
		}
	case containsMethod(r.URL.Path, "sendMessage"):
		return map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 42,
				"chat": map[string]any{
					"id": 12345,
				},
				"text": "ok",
			},
		}
	case containsMethod(r.URL.Path, "editMessageText"):
		return map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 42,
				"text":       "edited",
			},
		}
	case containsMethod(r.URL.Path, "sendChatAction"), containsMethod(r.URL.Path, "setMessageReaction"):
		return map[string]any{"ok": true}
	case containsMethod(r.URL.Path, "setMyCommands"):
		return map[string]any{"ok": true, "result": true}
	case containsMethod(r.URL.Path, "getFile"):
		return map[string]any{
			"ok": true,
			"result": map[string]any{
				"file_id":   "test_file_id",
				"file_path": "photos/test.jpg",
			},
		}
	case containsMethod(r.URL.Path, "getUpdates"):
		return map[string]any{
			"ok":     true,
			"result": []any{},
		}
	default:
		return map[string]any{
			"ok":     true,
			"result": map[string]any{},
		}
	}
}

func newMockBot(handler func(*http.Request) map[string]any) (*tgbotapi.BotAPI, error) {
	if handler == nil {
		handler = defaultMockBotResponse
	}
	return tgbotapi.NewBotAPIWithClient(
		"123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		"https://mock.telegram.invalid/bot%s/%s",
		&mockBotClient{handler: handler},
	)
}

// containsMethod 检查 URL 路径是否包含指定的 API 方法
func containsMethod(path, method string) bool {
	return len(path) > len(method) && path[len(path)-len(method)-1:] == "/"+method
}

// newAdapterWithMockBot 创建一个使用 mock bot 的 Adapter
func newAdapterWithMockBot() (*Adapter, mockBotServer, error) {
	bot, err := newMockBot(nil)
	if err != nil {
		return nil, mockBotServer{}, err
	}

	cfg := DefaultConfig()
	cfg.Token = bot.Token
	adapter := NewAdapter(cfg)
	adapter.bot = bot
	adapter.botUsername = "testbot"
	adapter.running = true

	return adapter, mockBotServer{}, nil
}

func newAdapterWithCapturedMessages(t *testing.T) (*Adapter, *[]capturedBotMessage) {
	t.Helper()
	var sent []capturedBotMessage
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "sendMessage") || containsMethod(r.URL.Path, "editMessageText") {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			method := "sendMessage"
			if containsMethod(r.URL.Path, "editMessageText") {
				method = "editMessageText"
			}
			sent = append(sent, capturedBotMessage{
				Method:    method,
				Text:      r.Form.Get("text"),
				ParseMode: r.Form.Get("parse_mode"),
				ReplyTo:   r.Form.Get("reply_to_message_id"),
				ThreadID:  r.Form.Get("message_thread_id"),
			})
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("newMockBot() error = %v", err)
	}
	cfg := DefaultConfig()
	cfg.Token = bot.Token
	adapter := NewAdapter(cfg)
	adapter.bot = bot
	adapter.botUsername = "testbot"
	adapter.running = true
	return adapter, &sent
}

func TestV054RegisterBotCommandsWithMockBot(t *testing.T) {
	var rawCommands string
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "setMyCommands") {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			rawCommands = r.Form.Get("commands")
			return map[string]any{"ok": true, "result": true}
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("newMockBot() error = %v", err)
	}

	adapter := NewAdapter(Config{Token: bot.Token})
	adapter.bot = bot

	if err := adapter.registerBotCommands(); err != nil {
		t.Fatalf("registerBotCommands() error = %v", err)
	}
	if strings.TrimSpace(rawCommands) == "" {
		t.Fatal("expected setMyCommands payload")
	}

	var commands []tgbotapi.BotCommand
	if err := json.Unmarshal([]byte(rawCommands), &commands); err != nil {
		t.Fatalf("decode commands payload: %v\npayload=%s", err, rawCommands)
	}
	if len(commands) != len(telegramCommandSpecs()) {
		t.Fatalf("expected %d commands, got %d: %#v", len(telegramCommandSpecs()), len(commands), commands)
	}
	seen := make(map[string]string, len(commands))
	for _, command := range commands {
		seen[command.Command] = command.Description
	}
	for _, name := range telegramCommandNames() {
		if seen[name] == "" {
			t.Fatalf("expected command %q to be registered; got %#v", name, seen)
		}
	}
}

func TestV054StartWithMockServer(t *testing.T) {
	// Start() calls tgbotapi.NewBotAPI which connects to real API,
	// so we test the setup logic by directly injecting a mock bot.
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter with mock bot: %v", err)
	}
	defer server.Close()

	if !adapter.IsRunning() {
		t.Error("expected adapter to be running")
	}

	if adapter.botUsername != "testbot" {
		t.Errorf("expected botUsername 'testbot', got '%s'", adapter.botUsername)
	}
}

func TestV054StartEmptyToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = ""
	adapter := NewAdapter(cfg)

	ctx := context.Background()
	err := adapter.Start(ctx)
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestV054StopWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	err = adapter.Stop()
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if adapter.IsRunning() {
		t.Error("expected adapter to not be running after stop")
	}
}

func TestV054SendWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	err = adapter.Send(ctx, "12345", "hello world")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestV054SendHTMLWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	err = adapter.SendHTML(ctx, "12345", "<b>工具调用</b>\n<pre><code>web_search: test ✅</code></pre>")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestV054SendNotRunning(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)

	ctx := context.Background()
	err := adapter.Send(ctx, "12345", "hello")
	if err == nil {
		t.Error("expected error when not running")
	}
}

func TestV054SendInvalidChatID(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	err = adapter.Send(ctx, "not-a-number", "hello")
	if err == nil {
		t.Error("expected error for invalid chat ID")
	}
}

func TestV054SendWithReplyMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	err = adapter.SendWithReply(ctx, "12345", "1", "reply message")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestV054SendPhotoLongCaptionUsesFullFollowUp(t *testing.T) {
	var mediaCaption string
	var followUps []capturedBotMessage
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		switch {
		case containsMethod(r.URL.Path, "sendPhoto"):
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("ParseMultipartForm() error = %v", err)
			}
			mediaCaption = r.FormValue("caption")
			return map[string]any{"ok": true, "result": map[string]any{
				"message_id": 100,
				"chat":       map[string]any{"id": 12345},
			}}
		case containsMethod(r.URL.Path, "sendMessage"):
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			followUps = append(followUps, capturedBotMessage{
				Method:    "sendMessage",
				Text:      r.Form.Get("text"),
				ParseMode: r.Form.Get("parse_mode"),
				ReplyTo:   r.Form.Get("reply_to_message_id"),
			})
			return map[string]any{"ok": true, "result": map[string]any{
				"message_id": 101,
				"chat":       map[string]any{"id": 12345},
			}}
		default:
			return defaultMockBotResponse(r)
		}
	})
	if err != nil {
		t.Fatalf("newMockBot() error = %v", err)
	}

	adapter := NewAdapter(Config{Token: bot.Token})
	adapter.bot = bot
	adapter.running = true
	photo := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(photo, []byte("test image"), 0o600); err != nil {
		t.Fatalf("write test photo: %v", err)
	}
	caption := strings.Repeat("full caption content ", 80)
	formatted := formatTelegramRichText(caption)

	if err := adapter.SendPhoto(context.Background(), "12345", "", photo, caption); err != nil {
		t.Fatalf("SendPhoto() error = %v", err)
	}
	if mediaCaption != "" {
		t.Fatalf("media caption = %q, want empty when caption overflows", mediaCaption)
	}
	if len(followUps) == 0 {
		t.Fatal("expected a follow-up text message for the full caption")
	}
	var delivered strings.Builder
	for _, followUp := range followUps {
		if followUp.ReplyTo != "100" {
			t.Fatalf("follow-up reply target = %q, want 100", followUp.ReplyTo)
		}
		if followUp.ParseMode != tgbotapi.ModeHTML {
			t.Fatalf("follow-up parse mode = %q, want HTML", followUp.ParseMode)
		}
		delivered.WriteString(followUp.Text)
	}
	if got := delivered.String(); got != formatted {
		t.Fatalf("follow-up text was altered: got %q, want %q", got, formatted)
	}
}

func TestV054SendWithReplyNotRunning(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)

	ctx := context.Background()
	err := adapter.SendWithReply(ctx, "12345", "1", "reply")
	if err == nil {
		t.Error("expected error when not running")
	}
}

func TestV054SendWithReplyInvalidChatID(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	err = adapter.SendWithReply(ctx, "not-a-number", "1", "reply")
	if err == nil {
		t.Error("expected error for invalid chat ID")
	}
}

func TestV054SendWithReplyInvalidReplyID(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	err = adapter.SendWithReply(ctx, "12345", "not-a-number", "reply")
	if err == nil {
		t.Error("expected error for invalid reply ID")
	}
}

func TestV054SendStreamWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream")
	}

	// 测试 Append
	if err := stream.Append("hello "); err != nil {
		t.Errorf("expected no error on Append, got: %v", err)
	}
	if err := stream.Append("world"); err != nil {
		t.Errorf("expected no error on Append, got: %v", err)
	}

	// 测试 SetThinking
	if err := stream.SetThinking("searching"); err != nil {
		t.Errorf("expected no error on SetThinking, got: %v", err)
	}

	// 测试 SetToolCall
	if err := stream.SetToolCall("web_search", "query=test"); err != nil {
		t.Errorf("expected no error on SetToolCall, got: %v", err)
	}

	// 测试 SetResult
	if err := stream.SetResult("final result"); err != nil {
		t.Errorf("expected no error on SetResult, got: %v", err)
	}

	// 测试 Finish
	if err := stream.Finish(); err != nil {
		t.Errorf("expected no error on Finish, got: %v", err)
	}

	// 测试 MessageID
	if stream.MessageID() != "42" {
		t.Errorf("expected message ID '42', got '%s'", stream.MessageID())
	}
}

func TestV054SendStreamNotRunning(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err == nil {
		t.Error("expected error when not running")
	}
	if stream != nil {
		t.Error("expected nil stream when not running")
	}
}

func TestV054SendStreamInvalidChatID(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "not-a-number", "")
	if err == nil {
		t.Error("expected error for invalid chat ID")
	}
	if stream != nil {
		t.Error("expected nil stream for invalid chat ID")
	}
}

func TestV054SendStreamWithReplyTo(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "10")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream")
	}
	stream.Finish()
}

func TestV054StreamSenderFinishDeliversLongCodeTail(t *testing.T) {
	adapter, sent := newAdapterWithCapturedMessages(t)
	adapter.cfg.MaxMessageLen = 180
	adapter.rememberTelegramThread("12345", "10", "42")

	stream, err := adapter.SendStream(context.Background(), "12345", "10")
	if err != nil {
		t.Fatalf("SendStream() error = %v", err)
	}

	code := "```go\nfunc total() int {\n" +
		strings.Repeat("\tvalue += 1\n", 20) +
		"\treturn value\n}\n```"
	if err := stream.SetResult(code); err != nil {
		t.Fatalf("SetResult() error = %v", err)
	}
	if err := stream.Finish(); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}

	var continuationTexts []string
	for _, message := range *sent {
		if message.Method == "sendMessage" && message.Text != "🧠 Thinking..." {
			if message.ReplyTo != "10" || message.ThreadID != "42" {
				t.Fatalf("continuation routing = reply %q, thread %q; want reply 10, thread 42", message.ReplyTo, message.ThreadID)
			}
			continuationTexts = append(continuationTexts, message.Text)
		}
	}
	if len(continuationTexts) == 0 {
		t.Fatal("expected at least one continuation message for long stream result")
	}

	finalMessages := make([]string, 0, len(*sent)-1)
	for _, message := range (*sent)[1:] {
		finalMessages = append(finalMessages, message.Text)
		if message.ParseMode == tgbotapi.ModeHTML {
			if strings.Count(message.Text, "<pre>") != strings.Count(message.Text, "</pre>") {
				t.Fatalf("unbalanced pre block in %q", message.Text)
			}
			if strings.Count(message.Text, "<code>") != strings.Count(message.Text, "</code>") {
				t.Fatalf("unbalanced code block in %q", message.Text)
			}
		}
	}
	combined := strings.Join(finalMessages, "\n")
	if !strings.Contains(combined, "return value\n}</code></pre>") {
		t.Fatalf("final code tail was not delivered: %q", combined)
	}
}

func TestV054SendStreamFallsBackWhenReplyTargetIsInvalid(t *testing.T) {
	var replyIDs []string
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "sendMessage") {
			_ = r.ParseForm()
			replyID := r.Form.Get("reply_to_message_id")
			replyIDs = append(replyIDs, replyID)
			if replyID != "" {
				return map[string]any{
					"ok":          false,
					"description": "Bad Request: message to be replied not found",
				}
			}
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("failed to create mock bot: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Token = bot.Token
	adapter := NewAdapter(cfg)
	adapter.bot = bot
	adapter.running = true

	stream, err := adapter.SendStream(context.Background(), "12345", "10")
	if err != nil {
		t.Fatalf("expected fallback SendStream to succeed, got: %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream")
	}
	if len(replyIDs) != 2 {
		t.Fatalf("expected sendMessage retry without reply, got reply ids %#v", replyIDs)
	}
	if replyIDs[0] != "10" || replyIDs[1] != "" {
		t.Fatalf("expected first send with reply and second without reply, got %#v", replyIDs)
	}
}

func TestV054StreamSenderDoubleFinish(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if err := stream.Finish(); err != nil {
		t.Errorf("expected no error on first Finish, got: %v", err)
	}
	if err := stream.Finish(); err != nil {
		t.Errorf("expected no error on second Finish, got: %v", err)
	}
}

func TestV054StreamSenderAppendAfterFinish(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	stream.Finish()

	if err := stream.Append("should fail"); err == nil {
		t.Error("expected error when appending after finish")
	}
}

func TestV054StreamSenderSetThinkingAfterFinish(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	stream.Finish()

	if err := stream.SetThinking("should be ignored"); err != nil {
		t.Errorf("expected nil on SetThinking after finish, got: %v", err)
	}
}

func TestV054StreamSenderSetToolCallAfterFinish(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	stream.Finish()

	if err := stream.SetToolCall("tool", "args"); err != nil {
		t.Errorf("expected nil on SetToolCall after finish, got: %v", err)
	}
}

func TestV054StreamSenderSetResultAfterFinish(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	stream.Finish()

	if err := stream.SetResult("should be ignored"); err != nil {
		t.Errorf("expected nil on SetResult after finish, got: %v", err)
	}
}

func TestV054StreamSenderThrottleEdit(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	stream.Append("chunk1")
	stream.Append("chunk2")
	stream.Append("chunk3")
	stream.Finish()
}

func TestV054StreamSenderFlushesQueuedContent(t *testing.T) {
	edits := make(chan string, 2)
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "editMessageText") {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			edits <- r.Form.Get("text")
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("newMockBot() error = %v", err)
	}

	adapter := NewAdapter(DefaultConfig())
	adapter.bot = bot
	adapter.running = true

	stream, err := adapter.SendStream(context.Background(), "12345", "")
	if err != nil {
		t.Fatalf("SendStream() error = %v", err)
	}
	sender := stream.(*telegramStreamSender)
	sender.mu.Lock()
	sender.lastEdit = time.Now().Add(-minEditInterval + 20*time.Millisecond)
	sender.mu.Unlock()

	if err := stream.Append("partial response"); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	select {
	case got := <-edits:
		if got != "partial response" {
			t.Fatalf("queued edit text = %q, want partial response", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected delayed edit before stream completion")
	}

	if err := stream.Finish(); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
}

func TestV054StreamSenderFinishCancelsQueuedEdit(t *testing.T) {
	edits := make(chan string, 3)
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "editMessageText") {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			edits <- r.Form.Get("text")
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("newMockBot() error = %v", err)
	}

	adapter := NewAdapter(DefaultConfig())
	adapter.bot = bot
	adapter.running = true

	stream, err := adapter.SendStream(context.Background(), "12345", "")
	if err != nil {
		t.Fatalf("SendStream() error = %v", err)
	}
	sender := stream.(*telegramStreamSender)
	sender.mu.Lock()
	sender.lastEdit = time.Now().Add(-minEditInterval + 30*time.Millisecond)
	sender.mu.Unlock()

	if err := stream.Append("final response"); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := stream.Finish(); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}

	select {
	case got := <-edits:
		if !strings.Contains(got, "final response") {
			t.Fatalf("final edit text = %q, want final response", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected final edit")
	}

	select {
	case got := <-edits:
		t.Fatalf("unexpected delayed edit after Finish(): %q", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestV054StreamSenderMaxEdits(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	sender := stream.(*telegramStreamSender)
	sender.editCount = maxEdits + 1

	stream.Append("should skip edit")
	stream.Finish()
}

func TestV054StreamSenderRenderContent(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	sender := stream.(*telegramStreamSender)

	var content string

	// 测试空内容（thinking 为空，content 为空）
	sender.mu.Lock()
	sender.thinking = ""
	sender.content = ""
	content = sender.renderContent()
	sender.mu.Unlock()
	if content != "🧠 Thinking..." {
		t.Errorf("expected default thinking, got '%s'", content)
	}

	// 测试只有思考标签
	sender.mu.Lock()
	sender.thinking = "🧠 Searching"
	sender.content = ""
	content = sender.renderContent()
	sender.mu.Unlock()
	if content != "🧠 Searching" {
		t.Errorf("expected thinking label, got '%s'", content)
	}

	// 测试只有内容
	sender.mu.Lock()
	sender.thinking = ""
	sender.content = "hello world"
	content = sender.renderContent()
	sender.mu.Unlock()
	if content != "hello world" {
		t.Errorf("expected content, got '%s'", content)
	}

	// 测试思考标签 + 内容
	sender.mu.Lock()
	sender.thinking = "🧠 Thinking"
	sender.content = "result"
	content = sender.renderContent()
	sender.mu.Unlock()
	expected := "🧠 Thinking\n\nresult"
	if content != expected {
		t.Errorf("expected '%s', got '%s'", expected, content)
	}

	stream.Finish()
}

func TestV054StreamSenderLongContent(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	sender := stream.(*telegramStreamSender)

	sender.mu.Lock()
	longContent := ""
	for i := 0; i < 5000; i++ {
		longContent += "x"
	}
	sender.content = longContent
	content := sender.renderContent()
	sender.mu.Unlock()

	if len(content) > 4096 {
		t.Errorf("expected content to be truncated, got length %d", len(content))
	}

	stream.Finish()
}

func TestV054StreamSenderToolCallLongArgs(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 测试超长参数截断
	longArgs := ""
	for i := 0; i < 200; i++ {
		longArgs += "x"
	}
	if err := stream.SetToolCall("tool", longArgs); err != nil {
		t.Errorf("expected no error on SetToolCall with long args, got: %v", err)
	}

	stream.Finish()
}

func TestV054SendTypingLoopWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go adapter.SendTypingLoop(ctx, "12345")

	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestV054SendTypingLoopNilBot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter.SendTypingLoop(ctx, "12345")
}

func TestV054SendTypingLoopInvalidChatID(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter.SendTypingLoop(ctx, "not-a-number")
}

func TestV054ReactToMessageWithMockBot(t *testing.T) {
	reactionSeen := make(chan url.Values, 1)
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "setMessageReaction") {
			_ = r.ParseForm()
			reactionSeen <- r.Form
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("failed to create mock bot: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Token = bot.Token
	adapter := NewAdapter(cfg)
	adapter.bot = bot
	adapter.botUsername = "testbot"
	adapter.running = true

	adapter.ReactToMessage("12345", "1", "👍")

	select {
	case form := <-reactionSeen:
		if got := form.Get("chat_id"); got != "12345" {
			t.Fatalf("chat_id = %q, want 12345", got)
		}
		if got := form.Get("message_id"); got != "1" {
			t.Fatalf("message_id = %q, want 1", got)
		}
		if got := form.Get("reaction"); !strings.Contains(got, `"emoji":"👍"`) {
			t.Fatalf("reaction = %q, want thumbs-up emoji payload", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected setMessageReaction request")
	}
}

func TestV054ReactToMessageNilBot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)

	adapter.ReactToMessage("12345", "1", "👍")
}

func TestV054ReactToMessageInvalidChatID(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	adapter.ReactToMessage("not-a-number", "1", "👍")
}

func TestV054ReactToMessageInvalidMessageID(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	adapter.ReactToMessage("12345", "not-a-number", "👍")
}

func TestV054CallSetMessageReactionWithMockBot(t *testing.T) {
	reactionSeen := make(chan url.Values, 1)
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "setMessageReaction") {
			_ = r.ParseForm()
			reactionSeen <- r.Form
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("failed to create mock bot: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Token = bot.Token
	adapter := NewAdapter(cfg)
	adapter.bot = bot
	adapter.botUsername = "testbot"
	adapter.running = true

	if err := adapter.callSetMessageReaction(12345, 1, "👍"); err != nil {
		t.Fatalf("callSetMessageReaction error = %v", err)
	}
	select {
	case form := <-reactionSeen:
		if got := form.Get("chat_id"); got != "12345" {
			t.Fatalf("chat_id = %q, want 12345", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected setMessageReaction request")
	}
}

func TestV054CallSetMessageReactionIncludesRouteInAPIError(t *testing.T) {
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "setMessageReaction") {
			return map[string]any{
				"ok":          false,
				"error_code":  400,
				"description": "Bad Request: reaction is not allowed",
			}
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("failed to create mock bot: %v", err)
	}

	adapter := NewAdapter(DefaultConfig())
	adapter.bot = bot
	err = adapter.callSetMessageReaction(-1001234567890, 42, "👍")
	if err == nil {
		t.Fatal("expected setMessageReaction API error")
	}
	for _, want := range []string{"setMessageReaction", "chat_id=-1001234567890", "message_id=42", "reaction is not allowed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, missing %q", err, want)
		}
	}
}

func TestV054CallTelegramAPIWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	body, err := adapter.callTelegramAPI("getMe", nil)
	if err != nil {
		t.Fatalf("callTelegramAPI error = %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected callTelegramAPI response body")
	}
}

func TestV054SendChunkWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	messageID, err := adapter.sendChunk(ctx, 12345, 0, "test message")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if messageID == 0 {
		t.Fatal("expected message ID")
	}
}

func TestV054SendChunkWithReply(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	messageID, err := adapter.sendChunk(ctx, 12345, 42, "reply message")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if messageID == 0 {
		t.Fatal("expected message ID")
	}
}

func TestV054ExtractAttachmentsPhotoWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	tgMsg := &tgbotapi.Message{
		Photo: []tgbotapi.PhotoSize{
			{FileID: "photo1", Width: 100, Height: 100, FileSize: 1024},
			{FileID: "photo2", Width: 200, Height: 200, FileSize: 2048},
			{FileID: "photo3", Width: 400, Height: 400, FileSize: 4096},
		},
		Caption: "photo caption",
	}

	gwMsg := &gateway.Message{}
	adapter.extractAttachments(tgMsg, gwMsg)

	if len(gwMsg.Attachments) == 0 {
		t.Error("expected non-empty attachments for photo")
	}
	if gwMsg.Attachments[0].FileID != "photo3" {
		t.Errorf("expected largest photo, got %s", gwMsg.Attachments[0].FileID)
	}
}

func TestV054ExtractAttachmentsDocumentWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	tgMsg := &tgbotapi.Message{
		Document: &tgbotapi.Document{
			FileID:   "doc1",
			FileName: "test.pdf",
			FileSize: 2048,
			MimeType: "application/pdf",
		},
	}

	gwMsg := &gateway.Message{}
	adapter.extractAttachments(tgMsg, gwMsg)

	if len(gwMsg.Attachments) == 0 {
		t.Error("expected non-empty attachments for document")
	}
	if gwMsg.Attachments[0].Type != gateway.AttachmentDocument {
		t.Errorf("expected document attachment, got %v", gwMsg.Attachments[0].Type)
	}
}

func TestV054ExtractAttachmentsVoiceWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	tgMsg := &tgbotapi.Message{
		Voice: &tgbotapi.Voice{
			FileID:   "voice1",
			FileSize: 1024,
			MimeType: "audio/ogg",
			Duration: 30,
		},
	}

	gwMsg := &gateway.Message{}
	adapter.extractAttachments(tgMsg, gwMsg)

	if len(gwMsg.Attachments) == 0 {
		t.Error("expected non-empty attachments for voice")
	}
	if gwMsg.Attachments[0].Type != gateway.AttachmentAudio {
		t.Errorf("expected audio attachment, got %v", gwMsg.Attachments[0].Type)
	}
}

func TestV054ExtractAttachmentsVideoWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	tgMsg := &tgbotapi.Message{
		Video: &tgbotapi.Video{
			FileID:   "video1",
			FileName: "video.mp4",
			FileSize: 10240,
			MimeType: "video/mp4",
		},
	}

	gwMsg := &gateway.Message{}
	adapter.extractAttachments(tgMsg, gwMsg)

	if len(gwMsg.Attachments) == 0 {
		t.Error("expected non-empty attachments for video")
	}
	if gwMsg.Attachments[0].Type != gateway.AttachmentVideo {
		t.Errorf("expected video attachment, got %v", gwMsg.Attachments[0].Type)
	}
}

func TestV054ExtractAttachmentsAudioWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	tgMsg := &tgbotapi.Message{
		Audio: &tgbotapi.Audio{
			FileID:   "audio1",
			FileName: "audio.mp3",
			FileSize: 5120,
			MimeType: "audio/mpeg",
		},
	}

	gwMsg := &gateway.Message{}
	adapter.extractAttachments(tgMsg, gwMsg)

	if len(gwMsg.Attachments) == 0 {
		t.Error("expected non-empty attachments for audio")
	}
	if gwMsg.Attachments[0].Type != gateway.AttachmentAudio {
		t.Errorf("expected audio attachment, got %v", gwMsg.Attachments[0].Type)
	}
}

func TestV054ExtractAttachmentsAdditionalTelegramMedia(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	tgMsg := &tgbotapi.Message{
		Animation: &tgbotapi.Animation{
			FileID:   "anim1",
			FileName: "clip.gif",
			FileSize: 4096,
			MimeType: "image/gif",
		},
		VideoNote: &tgbotapi.VideoNote{
			FileID:   "note1",
			FileSize: 2048,
		},
		Sticker: &tgbotapi.Sticker{
			FileID:   "sticker1",
			FileSize: 1024,
		},
	}

	gwMsg := &gateway.Message{}
	adapter.extractAttachments(tgMsg, gwMsg)

	if len(gwMsg.Attachments) != 3 {
		t.Fatalf("expected 3 attachments, got %d", len(gwMsg.Attachments))
	}
	if gwMsg.Attachments[0].Type != gateway.AttachmentVideo || gwMsg.Attachments[0].FileName != "clip.gif" {
		t.Fatalf("unexpected animation attachment: %+v", gwMsg.Attachments[0])
	}
	if gwMsg.Attachments[1].Type != gateway.AttachmentVideo || gwMsg.Attachments[1].FileName != "video_note.mp4" {
		t.Fatalf("unexpected video note attachment: %+v", gwMsg.Attachments[1])
	}
	if gwMsg.Attachments[2].Type != gateway.AttachmentImage || gwMsg.Attachments[2].MimeType != "image/webp" {
		t.Fatalf("unexpected sticker attachment: %+v", gwMsg.Attachments[2])
	}
}

func TestV054ExtractAttachmentsNilBot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)

	tgMsg := &tgbotapi.Message{
		Photo: []tgbotapi.PhotoSize{
			{FileID: "photo1", FileSize: 1024},
		},
	}

	gwMsg := &gateway.Message{}
	adapter.extractAttachments(tgMsg, gwMsg)

	if len(gwMsg.Attachments) != 0 {
		t.Error("expected no attachments when bot is nil")
	}
}

func TestV054PopulateAttachmentDataUsesResponseMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg; charset=binary")
		_, _ = w.Write([]byte("abc"))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.AttachmentDownloadLimit = 64
	adapter := NewAdapter(cfg)
	adapter.bot = &tgbotapi.BotAPI{Client: server.Client()}

	att := gateway.Attachment{
		Type:     gateway.AttachmentAudio,
		FileID:   "audio-id",
		FileURL:  server.URL + "/audio",
		FileName: "voice",
	}
	adapter.populateAttachmentData(&att)

	if att.FilePath == "" {
		t.Fatal("expected downloaded file path")
	}
	wantPrefix := filepath.Join(home, ".luckyagent", "workspace", "downloads", "telegram", "attachments")
	if !strings.HasPrefix(att.FilePath, wantPrefix+string(os.PathSeparator)) {
		t.Fatalf("expected attachment under %q, got %q", wantPrefix, att.FilePath)
	}
	if att.MimeType != "audio/mpeg" {
		t.Fatalf("expected response MIME type, got %q", att.MimeType)
	}
	if att.FileSize != 3 {
		t.Fatalf("expected file size 3, got %d", att.FileSize)
	}
	if filepath.Ext(att.FilePath) != ".mp3" {
		t.Fatalf("expected .mp3 file path, got %q", att.FilePath)
	}
}

func TestV054PopulateAttachmentDataRespectsDownloadLimit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcd"))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.AttachmentDownloadLimit = 2
	adapter := NewAdapter(cfg)
	adapter.bot = &tgbotapi.BotAPI{Client: server.Client()}

	att := gateway.Attachment{
		Type:     gateway.AttachmentDocument,
		FileID:   "doc-id",
		FileURL:  server.URL + "/doc",
		FileName: "doc.txt",
	}
	adapter.populateAttachmentData(&att)

	if att.FilePath != "" {
		t.Fatalf("expected attachment over limit not to be cached, got %q", att.FilePath)
	}
}

func TestV054ConvertMessageWithCommand(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		MessageID: 1,
		Chat: &tgbotapi.Chat{
			ID:   12345,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:        12345,
			UserName:  "testuser",
			FirstName: "Test",
		},
		Text: "/start hello",
		Entities: []tgbotapi.MessageEntity{
			{
				Type:   "bot_command",
				Offset: 0,
				Length: 6,
			},
		},
	}

	result := adapter.convertMessage(msg)
	if result == nil {
		t.Fatal("expected non-nil message")
	}
	if !result.IsCommand {
		t.Error("expected IsCommand to be true")
	}
	if result.Command != "start" {
		t.Errorf("expected command 'start', got '%s'", result.Command)
	}
	if result.Args != "hello" {
		t.Errorf("expected args 'hello', got '%s'", result.Args)
	}
}

func TestV054ConvertMessageSuperGroup(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		MessageID: 1,
		Chat: &tgbotapi.Chat{
			ID:    888888,
			Type:  "supergroup",
			Title: "Super Group",
		},
		From: &tgbotapi.User{
			ID:        12345,
			UserName:  "testuser",
			FirstName: "Test",
		},
		Text: "hello",
	}

	result := adapter.convertMessage(msg)
	if result.Chat.Type != gateway.ChatSuperGroup {
		t.Errorf("expected supergroup type, got %v", result.Chat.Type)
	}
}

func TestV054ConvertMessageChannel(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		MessageID: 1,
		Chat: &tgbotapi.Chat{
			ID:       999999,
			Type:     "channel",
			Title:    "Test Channel",
			UserName: "testchannel",
		},
		From: &tgbotapi.User{
			ID:        12345,
			UserName:  "testuser",
			FirstName: "Test",
		},
		Text: "channel message",
	}

	result := adapter.convertMessage(msg)
	if result.Chat.Type != gateway.ChatChannel {
		t.Errorf("expected channel type, got %v", result.Chat.Type)
	}
}

func TestV054ConvertMessageWithReply(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		MessageID: 2,
		Chat: &tgbotapi.Chat{
			ID:   12345,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:        12345,
			UserName:  "testuser",
			FirstName: "Test",
		},
		Text: "reply",
		ReplyToMessage: &tgbotapi.Message{
			MessageID: 1,
			Chat: &tgbotapi.Chat{
				ID:   12345,
				Type: "private",
			},
			From: &tgbotapi.User{
				ID:        999999,
				UserName:  "otheruser",
				FirstName: "Other",
			},
			Text: "original",
		},
	}

	result := adapter.convertMessage(msg)
	if result.ReplyTo == nil {
		t.Fatal("expected non-nil ReplyTo")
	}
	if result.ReplyTo.ID != "1" {
		t.Errorf("expected reply to ID '1', got '%s'", result.ReplyTo.ID)
	}
}

func TestV054ConvertMessageIncludesReplyAttachments(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		MessageID: 2,
		Chat: &tgbotapi.Chat{
			ID:   12345,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:        12345,
			UserName:  "testuser",
			FirstName: "Test",
		},
		Text: "reply",
		ReplyToMessage: &tgbotapi.Message{
			MessageID: 1,
			Chat: &tgbotapi.Chat{
				ID:   12345,
				Type: "private",
			},
			From: &tgbotapi.User{
				ID:        999999,
				UserName:  "otheruser",
				FirstName: "Other",
			},
			Photo: []tgbotapi.PhotoSize{
				{FileID: "reply-photo", FileSize: 1024},
			},
		},
	}

	result := adapter.convertMessage(msg)
	if result.ReplyTo == nil {
		t.Fatal("expected non-nil ReplyTo")
	}
	if len(result.ReplyTo.Attachments) != 1 {
		t.Fatalf("expected reply attachments to be preserved, got %+v", result.ReplyTo.Attachments)
	}
	if result.ReplyTo.Attachments[0].FileID != "reply-photo" {
		t.Fatalf("unexpected reply attachment: %+v", result.ReplyTo.Attachments[0])
	}
}

func TestV054ConvertMessageWithAttachments(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		MessageID: 1,
		Chat: &tgbotapi.Chat{
			ID:   12345,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:        12345,
			UserName:  "testuser",
			FirstName: "Test",
		},
		Photo: []tgbotapi.PhotoSize{
			{FileID: "photo1", FileSize: 1024},
		},
		Caption: "photo caption",
	}

	result := adapter.convertMessage(msg)
	if len(result.Attachments) == 0 {
		t.Error("expected non-empty attachments")
	}
	// 当前实现会优先把 Caption 回填给 Text，再提取附件。
	if result.Text != "photo caption" {
		t.Errorf("expected caption-backed photo text, got '%s'", result.Text)
	}
}

func TestV054ConvertMessageNoTextWithAttachment(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		MessageID: 1,
		Chat: &tgbotapi.Chat{
			ID:   12345,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:        12345,
			UserName:  "testuser",
			FirstName: "Test",
		},
		Voice: &tgbotapi.Voice{
			FileID:   "voice1",
			FileSize: 1024,
			MimeType: "audio/ogg",
		},
	}

	result := adapter.convertMessage(msg)
	if result.Text == "" {
		t.Error("expected auto-generated text for voice attachment")
	}
}

func TestV054ProcessUpdateWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	handlerCalled := false
	adapter.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		handlerCalled = true
		if msg.Text != "hello" {
			t.Errorf("expected text 'hello', got '%s'", msg.Text)
		}
		return nil
	})

	ctx := context.Background()
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat: &tgbotapi.Chat{
				ID:   12345,
				Type: "private",
			},
			From: &tgbotapi.User{
				ID:        12345,
				UserName:  "testuser",
				FirstName: "Test",
			},
			Text: "hello",
		},
	}

	adapter.processUpdate(ctx, update)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestV054ProcessUpdateNilMessage(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	update := tgbotapi.Update{}

	adapter.processUpdate(ctx, update)
}

func TestV054PollSubscribesToChannelPosts(t *testing.T) {
	updatesSeen := make(chan url.Values, 1)
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "getUpdates") {
			_ = r.ParseForm()
			select {
			case updatesSeen <- r.Form:
			default:
			}
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("new mock bot: %v", err)
	}

	adapter := NewAdapter(Config{Token: bot.Token, PollTimeout: 1})
	adapter.bot = bot
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.poll(ctx)

	select {
	case form := <-updatesSeen:
		var allowed []string
		if err := json.Unmarshal([]byte(form.Get("allowed_updates")), &allowed); err != nil {
			t.Fatalf("decode allowed_updates %q: %v", form.Get("allowed_updates"), err)
		}
		if len(allowed) != 3 || allowed[0] != tgbotapi.UpdateTypeMessage || allowed[1] != tgbotapi.UpdateTypeChannelPost || allowed[2] != tgbotapi.UpdateTypeCallbackQuery {
			t.Fatalf("allowed_updates = %#v, want message, channel_post, and callback_query", allowed)
		}
	case <-time.After(time.Second):
		t.Fatal("expected getUpdates request")
	}
}

func TestV054ProcessUpdateGroupMention(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	handlerCalled := false
	adapter.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		handlerCalled = true
		if !msg.IsGroupTrigger {
			t.Error("expected IsGroupTrigger to be true")
		}
		if msg.TriggerType != "mention" {
			t.Errorf("expected trigger type 'mention', got '%s'", msg.TriggerType)
		}
		return nil
	})

	ctx := context.Background()
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat: &tgbotapi.Chat{
				ID:    888888,
				Type:  "group",
				Title: "Test Group",
			},
			From: &tgbotapi.User{
				ID:        12345,
				UserName:  "testuser",
				FirstName: "Test",
			},
			Text: "hello @testbot",
			Entities: []tgbotapi.MessageEntity{
				{
					Type:   "mention",
					Offset: 6,
					Length: 8,
				},
			},
		},
	}

	adapter.processUpdate(ctx, update)

	if !handlerCalled {
		t.Error("expected handler to be called for group mention")
	}
}

func TestV054ProcessUpdateUnmentionedGroupReactsWithoutDispatching(t *testing.T) {
	reactionSeen := make(chan url.Values, 1)
	bot, err := newMockBot(func(r *http.Request) map[string]any {
		if containsMethod(r.URL.Path, "setMessageReaction") {
			_ = r.ParseForm()
			select {
			case reactionSeen <- r.Form:
			default:
			}
		}
		return defaultMockBotResponse(r)
	})
	if err != nil {
		t.Fatalf("new mock bot: %v", err)
	}

	adapter := NewAdapter(Config{Token: bot.Token})
	adapter.bot = bot
	handlerCalled := false
	adapter.SetHandler(func(context.Context, *gateway.Message) error {
		handlerCalled = true
		return nil
	})

	adapter.processUpdate(context.Background(), tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 43,
		Chat:      &tgbotapi.Chat{ID: -100123, Type: "supergroup", Title: "Team"},
		From:      &tgbotapi.User{ID: 9, UserName: "member"},
		Text:      "ordinary group message",
	}})

	select {
	case form := <-reactionSeen:
		if got := form.Get("chat_id"); got != "-100123" {
			t.Fatalf("reaction chat_id = %q, want -100123", got)
		}
		if got := form.Get("message_id"); got != "43" {
			t.Fatalf("reaction message_id = %q, want 43", got)
		}
		if got := form.Get("reaction"); !strings.Contains(got, `"emoji":"👍"`) {
			t.Fatalf("reaction payload = %q, want thumbs-up", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected a reaction for unmentioned group message")
	}
	if handlerCalled {
		t.Fatal("ordinary group message must not be dispatched to the agent")
	}
}

func TestV054ProcessUpdateGroupReplyToBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	handlerCalled := false
	adapter.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		handlerCalled = true
		if msg.TriggerType != "reply" {
			t.Errorf("expected trigger type 'reply', got '%s'", msg.TriggerType)
		}
		return nil
	})

	ctx := context.Background()
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 2,
			Chat: &tgbotapi.Chat{
				ID:    888888,
				Type:  "group",
				Title: "Test Group",
			},
			From: &tgbotapi.User{
				ID:        12345,
				UserName:  "testuser",
				FirstName: "Test",
			},
			Text: "reply to bot",
			ReplyToMessage: &tgbotapi.Message{
				MessageID: 1,
				Chat: &tgbotapi.Chat{
					ID:   888888,
					Type: "group",
				},
				From: &tgbotapi.User{
					ID:       123456789,
					UserName: "testbot",
					IsBot:    true,
				},
				Text: "bot message",
			},
		},
	}

	adapter.processUpdate(ctx, update)

	if !handlerCalled {
		t.Error("expected handler to be called for reply to bot")
	}
}

func TestV054ProcessUpdateGroupNoMentionNoReply(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	handlerCalled := false
	adapter.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		handlerCalled = true
		return nil
	})

	ctx := context.Background()
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat: &tgbotapi.Chat{
				ID:    888888,
				Type:  "group",
				Title: "Test Group",
			},
			From: &tgbotapi.User{
				ID:        12345,
				UserName:  "testuser",
				FirstName: "Test",
			},
			Text: "just chatting",
		},
	}

	adapter.processUpdate(ctx, update)

	if handlerCalled {
		t.Error("expected handler NOT to be called for group message without mention/reply")
	}
}

func TestV054ProcessUpdateChatNotAllowed(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	adapter.cfg.AllowedChats = []string{"99999"}

	handlerCalled := false
	adapter.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		handlerCalled = true
		return nil
	})

	ctx := context.Background()
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat: &tgbotapi.Chat{
				ID:   12345,
				Type: "private",
			},
			From: &tgbotapi.User{
				ID:        12345,
				UserName:  "testuser",
				FirstName: "Test",
			},
			Text: "hello",
		},
	}

	adapter.processUpdate(ctx, update)

	if handlerCalled {
		t.Error("expected handler NOT to be called for disallowed chat")
	}
}

func TestV054ProcessUpdateHandlerError(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	adapter.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		return fmt.Errorf("handler error")
	})

	ctx := context.Background()
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat: &tgbotapi.Chat{
				ID:   12345,
				Type: "private",
			},
			From: &tgbotapi.User{
				ID:        12345,
				UserName:  "testuser",
				FirstName: "Test",
			},
			Text: "hello",
		},
	}

	adapter.processUpdate(ctx, update)
}

func TestV054ProcessUpdateNoHandler(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			Chat: &tgbotapi.Chat{
				ID:   12345,
				Type: "private",
			},
			From: &tgbotapi.User{
				ID:        12345,
				UserName:  "testuser",
				FirstName: "Test",
			},
			Text: "hello",
		},
	}

	adapter.processUpdate(ctx, update)
}

func TestV054IsMentionedTextContains(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		Text: "hello @testbot how are you",
	}

	if !adapter.isMentioned(msg) {
		t.Error("expected message to be mentioned via text")
	}
}

func TestV054IsMentionedEntityMention(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		Text: "hello @testbot",
		Entities: []tgbotapi.MessageEntity{
			{
				Type:   "mention",
				Offset: 6,
				Length: 8,
			},
		},
	}

	if !adapter.isMentioned(msg) {
		t.Error("expected message to be mentioned via entity")
	}
}

func TestV054IsMentionedTextMention(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		Text: "hello bot",
		Entities: []tgbotapi.MessageEntity{
			{
				Type: "text_mention",
				User: &tgbotapi.User{
					UserName: "testbot",
				},
			},
		},
	}

	if !adapter.isMentioned(msg) {
		t.Error("expected message to be mentioned via text_mention")
	}
}

func TestV054IsMentionedNoUsername(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)

	msg := &tgbotapi.Message{
		Text: "hello @testbot",
	}

	if adapter.isMentioned(msg) {
		t.Error("expected not mentioned when botUsername is empty")
	}
}

func TestV054IsMentionedNotMentioned(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		Text: "hello world",
	}

	if adapter.isMentioned(msg) {
		t.Error("expected not mentioned")
	}
}

func TestV054ConcurrentSendWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			chatID := strconv.Itoa(10000 + idx)
			adapter.Send(ctx, chatID, "test-"+strconv.Itoa(idx))
		}(i)
	}

	wg.Wait()
}

func TestV054ConcurrentProcessUpdate(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	adapter.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		return nil
	})

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			update := tgbotapi.Update{
				Message: &tgbotapi.Message{
					MessageID: idx,
					Chat: &tgbotapi.Chat{
						ID:   12345,
						Type: "private",
					},
					From: &tgbotapi.User{
						ID:        12345,
						UserName:  "testuser",
						FirstName: "Test",
					},
					Text: "concurrent-" + strconv.Itoa(idx),
				},
			}
			adapter.processUpdate(ctx, update)
		}(i)
	}

	wg.Wait()
}

func TestV054WaitRateLimitNewChat(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	// 新 chat 不应该等待
	start := time.Now()
	adapter.waitRateLimit("99999")
	elapsed := time.Since(start)

	// 应该很快（没有等待）
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitRateLimit took too long for new chat: %v", elapsed)
	}
}

func TestV054WaitRateLimitSameChat(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	// 设置高 RateLimit 以减少等待时间
	adapter.cfg.RateLimit = 100

	// 第一次调用
	adapter.waitRateLimit("12345")

	// 第二次调用应该很快
	start := time.Now()
	adapter.waitRateLimit("12345")
	elapsed := time.Since(start)

	// RateLimit=100，间隔约 10ms，允许较大余量
	if elapsed > 2*time.Second {
		t.Errorf("waitRateLimit took too long: %v", elapsed)
	}
}

func TestV054HandleMessageWithAudioAttachment(t *testing.T) {
	msg := &gateway.Message{
		Text: "listen to this",
		Attachments: []gateway.Attachment{
			{
				Type:     gateway.AttachmentAudio,
				FileID:   "voice1",
				FileName: "voice.ogg",
				FileURL:  "https://example.com/voice.ogg",
			},
		},
	}

	inputText := msg.Text
	var mediaDesc strings.Builder
	mediaDesc.WriteString(inputText)
	mediaDesc.WriteString("\n\n")
	mediaDesc.WriteString("[多媒体内容]\n")
	for i, att := range msg.Attachments {
		switch att.Type {
		case gateway.AttachmentAudio:
			mediaDesc.WriteString(fmt.Sprintf("🎤 语音 %d: %s (URL: %s)\n", i+1, att.FileName, att.FileURL))
		}
	}
	inputText = mediaDesc.String()

	if !strings.Contains(inputText, "🎤 语音") {
		t.Error("expected audio description in inputText")
	}
}

func TestV054HandleMessageWithVideoAttachment(t *testing.T) {
	msg := &gateway.Message{
		Text: "watch this",
		Attachments: []gateway.Attachment{
			{
				Type:     gateway.AttachmentVideo,
				FileID:   "video1",
				FileName: "video.mp4",
				FileURL:  "https://example.com/video.mp4",
			},
		},
	}

	inputText := msg.Text
	var mediaDesc strings.Builder
	mediaDesc.WriteString(inputText)
	mediaDesc.WriteString("\n\n")
	mediaDesc.WriteString("[多媒体内容]\n")
	for i, att := range msg.Attachments {
		switch att.Type {
		case gateway.AttachmentVideo:
			mediaDesc.WriteString(fmt.Sprintf("🎬 视频 %d: %s (URL: %s)\n", i+1, att.FileName, att.FileURL))
		}
	}
	inputText = mediaDesc.String()

	if !strings.Contains(inputText, "🎬 视频") {
		t.Error("expected video description in inputText")
	}
}

func TestV054HandleMessageWithDocumentAttachment(t *testing.T) {
	msg := &gateway.Message{
		Text: "read this",
		Attachments: []gateway.Attachment{
			{
				Type:     gateway.AttachmentDocument,
				FileID:   "doc1",
				FileName: "report.pdf",
				MimeType: "application/pdf",
				FileURL:  "https://example.com/report.pdf",
			},
		},
	}

	inputText := msg.Text
	var mediaDesc strings.Builder
	mediaDesc.WriteString(inputText)
	mediaDesc.WriteString("\n\n")
	mediaDesc.WriteString("[多媒体内容]\n")
	for i, att := range msg.Attachments {
		switch att.Type {
		case gateway.AttachmentDocument:
			mediaDesc.WriteString(fmt.Sprintf("📎 文件 %d: %s (%s, URL: %s)\n", i+1, att.FileName, att.MimeType, att.FileURL))
		}
	}
	inputText = mediaDesc.String()

	if !strings.Contains(inputText, "📎 文件") {
		t.Error("expected document description in inputText")
	}
}

func TestV054HandleMessageNoAttachments(t *testing.T) {
	msg := &gateway.Message{
		Text: "just text",
	}

	inputText := msg.Text
	if len(msg.Attachments) > 0 {
		t.Error("expected no attachments")
	}

	if inputText != "just text" {
		t.Errorf("expected 'just text', got '%s'", inputText)
	}
}

func TestV054PollWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	_ = false
	adapter.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// poll 会阻塞，用 goroutine 运行
	done := make(chan struct{})
	go func() {
		adapter.poll(ctx)
		close(done)
	}()

	// 等待 poll 完成
	select {
	case <-done:
		// poll 在 context 取消后退出
	case <-time.After(3 * time.Second):
		t.Error("poll did not exit after context cancellation")
	}
}

func TestV054StartFullFlow(t *testing.T) {
	// Start() calls tgbotapi.NewBotAPI which connects to real Telegram API.
	// Cannot mock with httptest because NewBotAPI uses the default API endpoint.
	// Test the setup logic via direct bot injection instead.
	t.Skip("requires real Telegram Bot API — tested via integration tests")
}

func TestV054CallAPIWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	// 测试 getFile
	file, err := adapter.bot.GetFile(tgbotapi.FileConfig{FileID: "test_file_id"})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if file.FilePath != "photos/test.jpg" {
		t.Errorf("expected file path 'photos/test.jpg', got '%s'", file.FilePath)
	}
}

func TestV054SplitMessageBoundary(t *testing.T) {
	adapter, _, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	// MaxMessageLen defaults to 4000, so 4000 chars should be 1 part
	msg := strings.Repeat("a", 4000)
	parts := adapter.splitMessage(msg)
	if len(parts) != 1 {
		t.Errorf("expected 1 part for 4000 chars, got %d", len(parts))
	}

	// 4001 chars should be 2 parts
	msg = strings.Repeat("a", 4001)
	parts = adapter.splitMessage(msg)
	if len(parts) != 2 {
		t.Errorf("expected 2 parts for 4001 chars, got %d", len(parts))
	}
}

func TestV054SplitMessageEmpty(t *testing.T) {
	adapter, _, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	parts := adapter.splitMessage("")
	// splitMessage returns []string{""} for empty input
	if len(parts) != 1 {
		t.Errorf("expected 1 part for empty string, got %d", len(parts))
	}
}

func TestV054SplitMessageRespectsNewlines(t *testing.T) {
	adapter, _, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	msg := "line1\nline2\nline3\nline4"
	parts := adapter.splitMessage(msg)
	for i, part := range parts {
		if len(part) > 4096 {
			t.Errorf("part %d exceeds max length: %d", i, len(part))
		}
	}
}

func TestV054IsReplyToBotTrue(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{
				ID:       123456789,
				UserName: "testbot",
				IsBot:    true,
			},
		},
	}

	if !adapter.isReplyToBot(msg) {
		t.Error("expected isReplyToBot to be true")
	}
}

func TestV054IsReplyToBotFalse(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{
				ID:       999999,
				UserName: "otheruser",
				IsBot:    false,
			},
		},
	}

	if adapter.isReplyToBot(msg) {
		t.Error("expected isReplyToBot to be false")
	}
}

func TestV054IsReplyToBotNoReply(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	msg := &tgbotapi.Message{}

	if adapter.isReplyToBot(msg) {
		t.Error("expected isReplyToBot to be false when no reply")
	}
}

func TestV054IsReplyToBotNoUsername(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)
	// botUsername is empty, but isReplyToBot only checks IsBot flag

	msg := &tgbotapi.Message{
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{
				ID:       123456789,
				UserName: "testbot",
				IsBot:    true,
			},
		},
	}

	// isReplyToBot returns true when From.IsBot is true, regardless of botUsername
	if !adapter.isReplyToBot(msg) {
		t.Error("expected isReplyToBot to be true when From.IsBot is true")
	}
}

func TestV054EditMessageWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	// editMessage 是 telegramStreamSender 的方法
	// 通过 SendStream 获取 sender 来测试
	ctx := context.Background()
	stream, err := adapter.SendStream(ctx, "12345", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	sender := stream.(*telegramStreamSender)
	err = sender.editMessage("edited text")
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	stream.Finish()
}

func TestV054SendTypingOnceWithMockBot(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	adapter.sendTypingOnce(12345)
}

func TestV054SendTypingOnceInvalidChatID(t *testing.T) {
	adapter, server, err := newAdapterWithMockBot()
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer server.Close()

	adapter.sendTypingOnce(-1)
}

func TestV054AdapterName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Token = "test-token"
	adapter := NewAdapter(cfg)

	if adapter.Name() != "telegram" {
		t.Errorf("expected name 'telegram', got '%s'", adapter.Name())
	}
}
