package turn

import "time"

// EventType classifies a streaming agent event. Its numeric order is kept
// compatible with the legacy agent.ChatEvent constants.
type EventType int

const (
	ChatEventThinking EventType = iota
	ChatEventToolCall
	ChatEventToolResult
	ChatEventContent
	ChatEventDone
	ChatEventError
	ChatEventObservation
	ChatEventApprovalRequired
	ChatEventReasoningContent
	ChatEventCompact
	ChatEventUIDelta
)

// ChatEvent is the provider- and gateway-neutral event emitted by a turn.
// Err is kept for in-process consumers; transport adapters decide how to
// serialize it.
type ChatEvent struct {
	Type        EventType
	Content     string
	TaskID      string
	Name        string
	Args        string
	Result      string
	Round       int
	Observation *ObservationEvent
	Approval    *ApprovalEvent
	Compact     *CompactProgress
	UI          *UIDelta
	Usage       *TokenUsage
	CreatedAt   *time.Time
	Err         error
}

// UIDelta is a catalog-validated OpenUI patch. Unknown components must
// already have been dropped by the runtime catalog.
type UIDelta struct {
	TreeID string   `json:"tree_id"`
	State  string   `json:"state"`
	Nodes  []UINode `json:"nodes,omitempty"`
	Patch  bool     `json:"patch,omitempty"`
}

type UINode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Policy   string         `json:"policy,omitempty"`
	Props    map[string]any `json:"props,omitempty"`
	Children []string       `json:"children,omitempty"`
}

type ObservationEvent struct {
	FrameID      string  `json:"frame_id,omitempty"`
	FilePath     string  `json:"-"`
	MimeType     string  `json:"mime_type,omitempty"`
	Width        int     `json:"width,omitempty"`
	Height       int     `json:"height,omitempty"`
	ScaleFactor  float64 `json:"scale_factor,omitempty"`
	DisplayID    string  `json:"display_id,omitempty"`
	ActiveWindow string  `json:"active_window,omitempty"`
}

type ApprovalEvent struct {
	RequestID string            `json:"request_id,omitempty"`
	Tool      string            `json:"tool,omitempty"`
	Action    string            `json:"action,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	FrameID   string            `json:"frame_id,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Prompt    string            `json:"prompt,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	Secure    bool              `json:"secure,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

type CompactProgress struct {
	Phase               string `json:"phase"`
	Trigger             string `json:"trigger,omitempty"`
	Message             string `json:"message,omitempty"`
	BoundaryID          string `json:"boundary_id,omitempty"`
	SummarySource       string `json:"summary_source,omitempty"`
	Chunk               int    `json:"chunk,omitempty"`
	Chunks              int    `json:"chunks,omitempty"`
	PreTokenEstimate    int    `json:"pre_token_estimate,omitempty"`
	PostTokenEstimate   int    `json:"post_token_estimate,omitempty"`
	DroppedMessages     int    `json:"dropped_messages,omitempty"`
	RetainedMessages    int    `json:"retained_messages,omitempty"`
	RestoredAttachments int    `json:"restored_attachments,omitempty"`
	Model               string `json:"model,omitempty"`
	Error               string `json:"error,omitempty"`
}

// TokenUsage is the neutral, user-facing usage record for one agent turn.
type TokenUsage struct {
	InputTokens       int    `json:"input_tokens"`
	OutputTokens      int    `json:"output_tokens"`
	TotalTokens       int    `json:"total_tokens"`
	CachedInputTokens int    `json:"cached_input_tokens,omitempty"`
	Model             string `json:"model,omitempty"`
}
