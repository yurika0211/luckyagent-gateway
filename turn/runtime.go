package turn

import "context"

// ChatRuntime is the narrow runtime contract needed by a message adapter for
// one session turn. It deliberately exposes only neutral input and event DTOs;
// the runtime may still own provider, memory, tools, and session persistence
// behind this boundary.
type ChatRuntime interface {
	ChatWithSessionTurn(ctx context.Context, sessionID string, input Input) (string, error)
	ChatWithSessionStreamTurnEvents(ctx context.Context, sessionID string, input Input) (<-chan ChatEvent, error)
}
