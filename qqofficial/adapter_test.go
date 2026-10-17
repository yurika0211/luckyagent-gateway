package qqofficial

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/yurika0211/luckyagent-gateway"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBuildIntentBits(t *testing.T) {
	bits := buildIntentBits([]string{"group_messages", "c2c_messages"})
	if bits&(1<<25) == 0 {
		t.Fatal("expected group message intent bit")
	}
	if bits&(1<<26) == 0 {
		t.Fatal("expected c2c message intent bit")
	}
}

func TestParseCommand(t *testing.T) {
	isCommand, cmd, args := parseCommand("/hello world")
	if !isCommand || cmd != "/hello" || args != "world" {
		t.Fatalf("unexpected parse result: %v %q %q", isCommand, cmd, args)
	}
}

func TestStripLeadingQQMention(t *testing.T) {
	got := stripLeadingQQMention("<@!123456> 你好")
	if got != "你好" {
		t.Fatalf("stripLeadingQQMention() = %q", got)
	}
}

func TestConvertDispatchGroupAtMessage(t *testing.T) {
	a := NewAdapter(DefaultConfig())
	raw, _ := json.Marshal(incomingMessageEvent{
		ID:          "msg-1",
		Content:     "<@!123456> /ping now",
		GroupOpenID: "group-1",
		Author: messageAuthor{
			ID:       "user-1",
			Username: "tester",
		},
	})

	msg := a.convertDispatch("GROUP_AT_MESSAGE_CREATE", raw)
	if msg == nil {
		t.Fatal("expected message")
	}
	if msg.Chat.Type != gateway.ChatGroup || msg.Chat.ID != "group:group-1" {
		t.Fatalf("unexpected chat: %+v", msg.Chat)
	}
	if !msg.IsGroupTrigger || msg.TriggerType != "mention" {
		t.Fatalf("expected group trigger metadata: %+v", msg)
	}
	if !msg.IsCommand || msg.Command != "/ping" || msg.Args != "now" {
		t.Fatalf("unexpected command parsing: %+v", msg)
	}
}

func TestConvertDispatchC2CMessage(t *testing.T) {
	a := NewAdapter(DefaultConfig())
	raw, _ := json.Marshal(incomingMessageEvent{
		ID:      "msg-2",
		Content: "hello there",
		Author: messageAuthor{
			ID:       "user-2",
			Username: "tester2",
		},
	})

	msg := a.convertDispatch("C2C_MESSAGE_CREATE", raw)
	if msg == nil {
		t.Fatal("expected message")
	}
	if msg.Chat.Type != gateway.ChatPrivate || msg.Chat.ID != "c2c:user-2" {
		t.Fatalf("unexpected chat: %+v", msg.Chat)
	}
	if msg.Text != "hello there" {
		t.Fatalf("unexpected text: %q", msg.Text)
	}
}

func TestAccessTokenResponseUnmarshalExpiresInString(t *testing.T) {
	var resp accessTokenResponse
	if err := json.Unmarshal([]byte(`{"access_token":"abc","expires_in":"7200"}`), &resp); err != nil {
		t.Fatalf("unmarshal string expires_in: %v", err)
	}
	if resp.AccessToken != "abc" || resp.ExpiresIn != 7200 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestAccessTokenResponseUnmarshalExpiresInNumber(t *testing.T) {
	var resp accessTokenResponse
	if err := json.Unmarshal([]byte(`{"access_token":"abc","expires_in":7200}`), &resp); err != nil {
		t.Fatalf("unmarshal numeric expires_in: %v", err)
	}
	if resp.AccessToken != "abc" || resp.ExpiresIn != 7200 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestNewAdapterUsesDirectNetworkingWithoutProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:7897")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:7897")
	t.Setenv("ALL_PROXY", "socks5://127.0.0.1:7897")

	adapter := NewAdapter(Config{})
	transport, ok := adapter.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("HTTP transport = %T, want *http.Transport", adapter.httpClient.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("empty proxy configuration must not use proxy environment variables")
	}

	dialer, ok := adapter.gatewayDialer.(*websocket.Dialer)
	if !ok {
		t.Fatalf("gateway dialer = %T, want *websocket.Dialer", adapter.gatewayDialer)
	}
	if dialer.Proxy != nil {
		t.Fatal("websocket dialer must not use proxy environment variables")
	}
}

func TestNewAdapterConfiguresHTTPProxyForHTTPAndWebSocket(t *testing.T) {
	adapter := NewAdapter(Config{Proxy: "http://127.0.0.1:7897"})
	transport, ok := adapter.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("HTTP transport = %T, want *http.Transport", adapter.httpClient.Transport)
	}
	dialer, ok := adapter.gatewayDialer.(*websocket.Dialer)
	if !ok {
		t.Fatalf("gateway dialer = %T, want *websocket.Dialer", adapter.gatewayDialer)
	}

	requestURL, err := url.Parse("https://bots.qq.com")
	if err != nil {
		t.Fatal(err)
	}
	for name, proxyFunc := range map[string]func(*http.Request) (*url.URL, error){
		"HTTP":      transport.Proxy,
		"WebSocket": dialer.Proxy,
	} {
		if proxyFunc == nil {
			t.Fatalf("%s proxy function is nil", name)
		}
		proxyURL, err := proxyFunc(&http.Request{URL: requestURL})
		if err != nil {
			t.Fatalf("%s proxy lookup: %v", name, err)
		}
		if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7897" {
			t.Fatalf("%s proxy = %v, want http://127.0.0.1:7897", name, proxyURL)
		}
	}
}

func TestNewAdapterConfiguresSOCKS5ProxyForHTTPAndWebSocket(t *testing.T) {
	adapter := NewAdapter(Config{Proxy: "socks5://127.0.0.1:7890"})
	transport, ok := adapter.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("HTTP transport = %T, want *http.Transport", adapter.httpClient.Transport)
	}
	dialer, ok := adapter.gatewayDialer.(*websocket.Dialer)
	if !ok {
		t.Fatalf("gateway dialer = %T, want *websocket.Dialer", adapter.gatewayDialer)
	}
	if transport.Proxy != nil || transport.DialContext == nil {
		t.Fatalf("unexpected HTTP SOCKS configuration: hasProxy=%t hasDialContext=%t", transport.Proxy != nil, transport.DialContext != nil)
	}
	if dialer.Proxy != nil || dialer.NetDialContext == nil {
		t.Fatalf("unexpected WebSocket SOCKS configuration: hasProxy=%t hasNetDialContext=%t", dialer.Proxy != nil, dialer.NetDialContext != nil)
	}
}

func TestStartLogsInvalidProxyConfiguration(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "logs", "qqofficial-gateway.log")
	adapter := NewAdapter(Config{
		AppID:     "app-id",
		AppSecret: "app-secret",
		Proxy:     "ftp://127.0.0.1:21",
		LogPath:   logPath,
	})
	defer adapter.Stop()

	err := adapter.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unsupported proxy scheme") {
		t.Fatalf("Start() error = %v, want unsupported proxy scheme", err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gateway log: %v", err)
	}
	for _, want := range []string{"start failed", "unsupported proxy scheme", "sandbox=false", "proxy=ftp://127.0.0.1:21"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("gateway log missing %q:\n%s", want, content)
		}
	}
}

func TestAdapterReconnectsAfterGatewayReconnectOp(t *testing.T) {
	var connections atomic.Int32
	messageReceived := make(chan *gateway.Message, 1)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.Error(w, "unexpected token path "+r.URL.Path, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(accessTokenResponse{AccessToken: "token", ExpiresIn: 3600})
	}))
	defer tokenServer.Close()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
		n := connections.Add(1)
		if err := conn.WriteJSON(gatewayFrame{Op: helloOp, D: mustJSON(helloPayload{HeartbeatInterval: 60_000})}); err != nil {
			t.Errorf("write hello: %v", err)
			return
		}
		var identify gatewayFrame
		if err := conn.ReadJSON(&identify); err != nil {
			t.Errorf("read identify: %v", err)
			return
		}
		if identify.Op != identifyOp {
			t.Errorf("expected identify op, got %d", identify.Op)
			return
		}
		if n == 1 {
			_ = conn.WriteJSON(gatewayFrame{Op: reconnectOp})
			return
		}
		raw, _ := json.Marshal(incomingMessageEvent{
			ID:      "msg-reconnected",
			Content: "hello after reconnect",
			Author:  messageAuthor{ID: "user-1", Username: "tester"},
		})
		_ = conn.WriteJSON(gatewayFrame{
			Op: dispatchEventOp,
			T:  "C2C_MESSAGE_CREATE",
			D:  raw,
		})
		<-r.Context().Done()
	}))
	defer wsServer.Close()

	a := NewAdapter(Config{
		AppID:         "app-id",
		AppSecret:     "secret",
		GatewayURL:    wsToHTTPTestURL(t, wsServer.URL),
		ReconnectWait: 1,
	})
	a.accessTokenURL = tokenServer.URL + "/token"
	a.SetHandler(func(ctx context.Context, msg *gateway.Message) error {
		messageReceived <- msg
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer a.Stop()

	select {
	case msg := <-messageReceived:
		if msg.Text != "hello after reconnect" {
			t.Fatalf("unexpected message after reconnect: %+v", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for reconnected message; connections=%d", connections.Load())
	}
	if got := connections.Load(); got < 2 {
		t.Fatalf("expected at least 2 websocket connections, got %d", got)
	}
	if !a.IsRunning() {
		t.Fatal("adapter should remain running after reconnect")
	}
}

func wsToHTTPTestURL(t *testing.T, raw string) string {
	t.Helper()
	addr := strings.TrimPrefix(raw, "http://")
	if _, _, err := net.SplitHostPort(addr); err != nil {
		t.Fatalf("unexpected test server URL %q: %v", raw, err)
	}
	return "ws://" + addr
}

func TestSendWithReplyUsesIncrementingMsgSeq(t *testing.T) {
	var mu sync.Mutex
	var payloads []outgoingMessagePayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/users/user-1/messages" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusInternalServerError)
			return
		}
		var payload outgoingMessagePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "decode payload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		payloads = append(payloads, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := NewAdapter(Config{
		AppID:      "app-id",
		AppSecret:  "app-secret",
		APIBaseURL: server.URL,
	})
	a.accessToken = "token"
	a.tokenExpiry = time.Now().Add(time.Hour)

	if err := a.SendWithReply(context.Background(), "c2c:user-1", "msg-1", "first"); err != nil {
		t.Fatalf("first SendWithReply error = %v", err)
	}
	if err := a.SendWithReply(context.Background(), "c2c:user-1", "msg-1", "second"); err != nil {
		t.Fatalf("second SendWithReply error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
	if payloads[0].MsgID != "msg-1" || payloads[1].MsgID != "msg-1" {
		t.Fatalf("unexpected msg_id values: %#v", payloads)
	}
	if payloads[0].MsgSeq == 0 || payloads[1].MsgSeq == 0 {
		t.Fatalf("expected msg_seq to be set: %#v", payloads)
	}
	if payloads[0].MsgSeq == payloads[1].MsgSeq {
		t.Fatalf("expected msg_seq to increment, got %#v", payloads)
	}
}

func TestSendForwardedTextSendsChunksSequentially(t *testing.T) {
	var mu sync.Mutex
	var payloads []outgoingMessagePayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/groups/group-1/messages" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusInternalServerError)
			return
		}
		var payload outgoingMessagePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "decode payload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		payloads = append(payloads, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := NewAdapter(Config{
		AppID:      "app-id",
		AppSecret:  "app-secret",
		APIBaseURL: server.URL,
	})
	a.accessToken = "token"
	a.tokenExpiry = time.Now().Add(time.Hour)

	if err := a.SendForwardedText(context.Background(), "group:group-1", "LuckyAgent", []string{"first", "second"}); err != nil {
		t.Fatalf("SendForwardedText error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
	if payloads[0].Content != "first" || payloads[1].Content != "second" {
		t.Fatalf("unexpected forwarded text payloads: %#v", payloads)
	}
}

func TestSendDocumentUploadsSandboxPath(t *testing.T) {
	tmpDir := t.TempDir()
	doc := tmpDir + "/report.pdf"
	content := []byte("fake pdf")
	if err := os.WriteFile(doc, content, 0o600); err != nil {
		t.Fatalf("write document: %v", err)
	}

	var uploadSeen bool
	var sendSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/users/user-1/files":
			var payload uploadFilePayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "decode upload payload: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if payload.FileType != 4 {
				http.Error(w, "unexpected file_type", http.StatusInternalServerError)
				return
			}
			if payload.FileName != "report.pdf" {
				http.Error(w, "unexpected file_name "+payload.FileName, http.StatusInternalServerError)
				return
			}
			if payload.FileData != base64.StdEncoding.EncodeToString(content) {
				http.Error(w, "unexpected file_data", http.StatusInternalServerError)
				return
			}
			uploadSeen = true
			_, _ = w.Write([]byte(`{"file_info":"file-info-1"}`))
		case "/v2/users/user-1/messages":
			var payload outgoingMessagePayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "decode send payload: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if payload.MsgType != 7 || payload.Media == nil || payload.Media.FileInfo != "file-info-1" {
				http.Error(w, "unexpected rich media payload", http.StatusInternalServerError)
				return
			}
			sendSeen = true
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	a := NewAdapter(Config{
		AppID:      "app-id",
		AppSecret:  "app-secret",
		APIBaseURL: server.URL,
	})
	a.accessToken = "token"
	a.tokenExpiry = time.Now().Add(time.Hour)

	if err := a.SendDocument(context.Background(), "c2c:user-1", "msg-1", "sandbox:"+doc, ""); err != nil {
		t.Fatalf("SendDocument error = %v", err)
	}
	if !uploadSeen || !sendSeen {
		t.Fatalf("expected upload and send requests, upload=%t send=%t", uploadSeen, sendSeen)
	}
}

func TestSplitQQMessageChunks(t *testing.T) {
	long := strings.Repeat("你", qqProgressTraceChunkLimit+20)
	chunks := splitQQMessageChunks(long, qqProgressTraceChunkLimit)

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if qqRuneLen(chunk) > qqProgressTraceChunkLimit {
			t.Fatalf("chunk exceeds limit: %d", qqRuneLen(chunk))
		}
	}
}

func TestQQOutgoingChunksPreserveFullText(t *testing.T) {
	message := " \n" + strings.Repeat("你", qqProgressTraceChunkLimit+20) + "\n "
	chunks := qqOutgoingChunks(message)
	if got := strings.Join(chunks, ""); got != message {
		t.Fatalf("reassembled chunks = %q, want %q", got, message)
	}
	for _, chunk := range chunks {
		if got := qqRuneLen(chunk); got > qqProgressTraceChunkLimit {
			t.Fatalf("chunk rune length = %d, limit %d", got, qqProgressTraceChunkLimit)
		}
	}
}

func TestSendStreamReturnsSenderWhenRunning(t *testing.T) {
	a := NewAdapter(DefaultConfig())
	a.running = true

	sender, err := a.SendStream(context.Background(), "c2c:user-1", "msg-1")
	if err != nil {
		t.Fatalf("SendStream error = %v", err)
	}
	if sender == nil {
		t.Fatal("expected non-nil sender")
	}
	if sender.MessageID() != "msg-1" {
		t.Fatalf("expected message id msg-1, got %q", sender.MessageID())
	}
}

func TestResolveOutboundMediaResponse(t *testing.T) {
	tmpDir := t.TempDir()
	img := tmpDir + "/a.png"
	doc := tmpDir + "/b.pdf"
	if err := os.WriteFile(img, []byte("img"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := os.WriteFile(doc, []byte("doc"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	text, media, err := resolveOutboundMediaResponse("图片如下\nMEDIA:" + img + "\nMEDIA:" + doc)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse error = %v", err)
	}
	if text != "图片如下" {
		t.Fatalf("unexpected text %q", text)
	}
	if len(media) != 2 {
		t.Fatalf("expected 2 media items, got %d", len(media))
	}
}

func TestResolveOutboundMediaResponseSandboxPath(t *testing.T) {
	tmpDir := t.TempDir()
	doc := tmpDir + "/report.pdf"
	if err := os.WriteFile(doc, []byte("doc"), 0o600); err != nil {
		t.Fatalf("write document: %v", err)
	}

	text, media, err := resolveOutboundMediaResponse("文件已生成\nMEDIA:sandbox:" + doc)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse error = %v", err)
	}
	if text != "文件已生成" {
		t.Fatalf("unexpected text %q", text)
	}
	if len(media) != 1 || media[0].Source != "sandbox:"+doc || media[0].Kind != outboundMediaDocument {
		t.Fatalf("unexpected media %#v", media)
	}
}

func TestResolveOutboundMediaResponseMarkdownSandboxLink(t *testing.T) {
	tmpDir := t.TempDir()
	img := tmpDir + "/chart.png"
	if err := os.WriteFile(img, []byte("img"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}

	text, media, err := resolveOutboundMediaResponse("PNG 在这里：[chart](sandbox:" + img + ")")
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse error = %v", err)
	}
	if text != "PNG 在这里：[chart](sandbox:"+img+")" {
		t.Fatalf("unexpected text %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("markdown sandbox link should not auto-send media, got %#v", media)
	}
}

func TestResolveOutboundMediaResponseBareLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	doc := tmpDir + "/result.pdf"
	if err := os.WriteFile(doc, []byte("doc"), 0o600); err != nil {
		t.Fatalf("write document: %v", err)
	}

	text, media, err := resolveOutboundMediaResponse("文件在这里 " + doc)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse error = %v", err)
	}
	if text != "文件在这里 "+doc {
		t.Fatalf("unexpected text %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("bare local file should not auto-send media, got %#v", media)
	}
}

func TestResolveOutboundMediaResponseDoesNotSendConfigJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"api_key":"secret"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	text, media, err := resolveOutboundMediaResponse("配置在这里 " + configPath)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse error = %v", err)
	}
	if text != "配置在这里 "+configPath {
		t.Fatalf("unexpected text %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("config.json should not be sent as media, got %#v", media)
	}

	text, media, err = resolveOutboundMediaResponse("MEDIA:" + configPath)
	if err != nil {
		t.Fatalf("resolveOutboundMediaResponse MEDIA config error = %v", err)
	}
	if text != "MEDIA:"+configPath {
		t.Fatalf("expected sensitive MEDIA tag to remain text, got %q", text)
	}
	if len(media) != 0 {
		t.Fatalf("explicit config.json MEDIA tag should be blocked, got %#v", media)
	}
}

func TestQQMediaDeliveryGuidance(t *testing.T) {
	got := qqMediaDeliveryGuidance("生成报告")
	if !strings.Contains(got, "MEDIA:/absolute/path/to/file.ext") {
		t.Fatalf("guidance should mention MEDIA path, got %q", got)
	}
	again := qqMediaDeliveryGuidance(got)
	if strings.Count(again, "[QQ delivery rule]") != 1 {
		t.Fatalf("guidance should not be duplicated, got %q", again)
	}
}
