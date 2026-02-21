package adapters

import "context"

// IncomingMessage is the platform-agnostic message representation.
type IncomingMessage struct {
	Platform  string // "telegram" | "discord" | "slack"
	ChannelID string // platform-specific channel identifier
	Sender    string
	Content   string
}

// MessageHandler is the callback invoked by adapters when a message arrives.
type MessageHandler func(msg IncomingMessage)

// Adapter is the interface every messaging platform adapter must implement.
type Adapter interface {
	// Name returns the platform identifier.
	Name() string

	// Start begins listening for messages. Blocks until ctx is cancelled.
	Start(ctx context.Context) error

	// Send sends a message to a specific channel on this platform.
	Send(channelID string, message string) error

	// SetHandler registers the message handler callback.
	SetHandler(fn MessageHandler)

	// SendTyping sends a typing/processing indicator (optional — no-op if not supported).
	SendTyping(channelID string)
}
