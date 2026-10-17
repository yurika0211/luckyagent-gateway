package gateway

import (
	"context"
	"time"

	"github.com/yurika0211/luckyagent-gateway/turn"
)

// ChatType represents the type of chat.
type ChatType int

const (
	ChatPrivate ChatType = iota
	ChatGroup
	ChatSuperGroup
	ChatChannel
)

// String returns a human-readable name for the ChatType.
func (ct ChatType) String() string {
	switch ct {
	case ChatPrivate:
		return "private"
	case ChatGroup:
		return "group"
	case ChatSuperGroup:
		return "supergroup"
	case ChatChannel:
		return "channel"
	default:
		return "unknown"
	}
}

// Chat represents a chat conversation.
type Chat struct {
	ID       string
	Type     ChatType
	Title    string
	Username string
}

// User represents a messaging platform user.
type User struct {
	ID        string
	Username  string
	FirstName string
	LastName  string
}

// DisplayName returns the best available display name for the user.
func (u User) DisplayName() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	if u.FirstName != "" {
		if u.LastName != "" {
			return u.FirstName + " " + u.LastName
		}
		return u.FirstName
	}
	return u.ID
}

// Message represents an incoming message from a messaging platform.
type Message struct {
	ID   string
	Chat Chat
	// ThreadID identifies a platform-specific conversation thread or topic.
	// It is empty when the platform does not provide a thread identifier.
	ThreadID  string
	Sender    User
	Text      string
	ReplyTo   *Message // if this is a reply
	Timestamp time.Time
	IsCommand bool
	Command   string // e.g., "/start"
	Args      string // everything after the command

	// v0.36.0: 多媒体附件
	Attachments []Attachment

	// v0.44.0: 群聊触发信息
	IsGroupTrigger bool   // 是否在群聊中被触发（@或回复bot）
	TriggerType    string // 触发方式: "mention" | "reply"
}

// Attachment is an alias kept for gateway-facing compatibility. The neutral
// representation lives in internal/turn so provider code does not depend on
// this package.
type Attachment = turn.Attachment

// ForwardedMediaItem represents one media node in a platform forwarded-message envelope.
type ForwardedMediaItem struct {
	Type    AttachmentType
	Source  string
	Caption string
}

// AttachmentType is an alias kept for gateway-facing compatibility.
type AttachmentType = turn.AttachmentType

const (
	AttachmentImage    = turn.AttachmentImage
	AttachmentAudio    = turn.AttachmentAudio
	AttachmentVideo    = turn.AttachmentVideo
	AttachmentDocument = turn.AttachmentDocument
)

// MessageHandler is the callback type for handling incoming messages.
type MessageHandler func(ctx context.Context, msg *Message) error

// ForwardedMediaSender is implemented by gateways that can bundle multiple media
// items into one forwarded-message envelope instead of sending each item separately.
type ForwardedMediaSender interface {
	SendForwardedMedia(ctx context.Context, chatID string, title string, items []ForwardedMediaItem) error
}

// ForwardedTextSender is implemented by gateways that can deliver long text as
// multiple grouped chunks. Platforms without a native forwarded-message envelope
// may send the chunks sequentially.
type ForwardedTextSender interface {
	SendForwardedText(ctx context.Context, chatID string, title string, chunks []string) error
}
