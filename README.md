# LuckyAgent Gateway

An independent Go module for messaging transports, extracted from LuckyAgent.
It contains platform protocols and presentation utilities without importing the
LuckyAgent runtime, model providers, memory store or application config.

```text
github.com/yurika0211/luckyagent-gateway
```

The root package is named `gateway`. Platform adapters live in `telegram`,
`qqofficial`, `napcat`, `feishu`, `weixin`, and `openclawweixin`. `collector`
collects message segments, and `turn` defines neutral input, attachment, event,
and chat-runtime contracts shared by the host and API transports.
`telegram/presentation` renders tool/Skill metadata, progress cards and memory
traces using the public `luckyagent-tools` contracts. NapCat group history uses
the same module's neutral `chat` values, so the host needs no duplicate DTOs.

## Use a platform adapter

```go
package main

import (
    "context"
    "os"
    "os/signal"

    gateway "github.com/yurika0211/luckyagent-gateway"
    "github.com/yurika0211/luckyagent-gateway/telegram"
)

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()
    adapter := telegram.NewAdapter(telegram.Config{Token: os.Getenv("TELEGRAM_BOT_TOKEN")})
    adapter.SetHandler(func(ctx context.Context, message *gateway.Message) error {
        return adapter.Send(ctx, message.Chat.ID, "Received: "+message.Text)
    })
    if err := adapter.Start(ctx); err != nil {
        panic(err)
    }
    defer adapter.Stop()
    <-ctx.Done()
}
```

The host supplies the message callback and owns application operations such as
model switching, session administration, memory writes and tool approval. The
module owns platform connections, authentication with platforms, retries,
message splitting, media conversion, topic/reply anchors and streaming output.
Existing permission checks, download limits and platform behavior are retained.

`telegram.NewAdapterWithClient` accepts an initialized Telegram Bot API client
for externally delivered updates. It starts no polling goroutine; deliver an
update using `ProcessUpdate`. The ordinary `NewAdapter`/`Start` path continues
to own long polling. A stream sender may optionally implement
`SetHTMLCard(string) error`; hosts should test that capability through an
interface instead of depending on the implementation type.

## Development and compatibility

```bash
go test ./...
```

The platform tests moved with their implementations. A boundary test rejects
imports from `github.com/yurika0211/luckyagent/`. Application command and runtime
tests remain in LuckyAgent under `internal/channelapp`.

The initial extraction preserves the existing public Go types, JSON names,
attachment representation and stream event order. LuckyAgent's old internal
imports are forwarding aliases; platform implementations have a single owner
in this module. Standard `log/slog` uses the host's configured default logger.

This project retains the original MIT license and attribution. Install the
current extraction with `go get github.com/yurika0211/luckyagent-gateway@v0.2.2`.
