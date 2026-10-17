package turn

import "strings"

// Input is the neutral message payload exchanged between an edge adapter and
// the agent runtime. It intentionally contains no provider or gateway types.
// Adapters may add platform context in Scope; the runtime turns this DTO into
// the provider-specific message shape at the boundary.
type Input struct {
	OriginalText string
	RoutingText  string
	Attachments  []Attachment
	Scope        Scope
}

// Scope identifies the messaging context of an input without naming a
// concrete gateway implementation.
type Scope struct {
	Platform    string
	ChatID      string
	ChatType    string
	SenderID    string
	SenderName  string
	DisplayName string
}

func TextInput(text string) Input {
	text = strings.TrimSpace(text)
	return Input{OriginalText: text, RoutingText: text}
}

func (in Input) WithScope(scope Scope) Input {
	in.Scope = scope.Normalize()
	return in
}

func (in Input) WithRoutingText(text string) Input {
	in.RoutingText = strings.TrimSpace(text)
	return in
}

func (s Scope) Normalize() Scope {
	s.Platform = strings.ToLower(strings.TrimSpace(s.Platform))
	s.ChatID = strings.TrimSpace(s.ChatID)
	s.ChatType = strings.ToLower(strings.TrimSpace(s.ChatType))
	s.SenderID = strings.TrimSpace(s.SenderID)
	s.SenderName = strings.TrimSpace(s.SenderName)
	s.DisplayName = strings.TrimSpace(s.DisplayName)
	return s
}
