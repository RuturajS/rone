package adapters

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

// DiscordAdapter implements Adapter for Discord using WebSocket gateway.
type DiscordAdapter struct {
	token   string
	session *discordgo.Session
	handler MessageHandler
}

// NewDiscordAdapter creates a new Discord adapter.
func NewDiscordAdapter(token string) *DiscordAdapter {
	return &DiscordAdapter{token: token}
}

func (d *DiscordAdapter) Name() string { return "discord" }

func (d *DiscordAdapter) SetHandler(fn MessageHandler) {
	d.handler = fn
}

func (d *DiscordAdapter) Start(ctx context.Context) error {
	session, err := discordgo.New("Bot " + d.token)
	if err != nil {
		return fmt.Errorf("discord: create session: %w", err)
	}
	d.session = session

	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore own messages
		if m.Author.ID == s.State.User.ID {
			return
		}
		if d.handler != nil {
			d.handler(IncomingMessage{
				Platform:  "discord",
				ChannelID: m.ChannelID,
				Sender:    m.Author.Username,
				Content:   m.Content,
			})
		}
	})

	if err := session.Open(); err != nil {
		return fmt.Errorf("discord: open session: %w", err)
	}
	slog.Info("discord adapter started")

	// Block until context is cancelled
	<-ctx.Done()

	slog.Info("discord adapter stopping")
	return session.Close()
}

func (d *DiscordAdapter) SendTyping(channelID string) {
	if d.session != nil {
		_ = d.session.ChannelTyping(channelID)
	}
}

func (d *DiscordAdapter) Send(channelID string, message string) error {
	if d.session == nil {
		return fmt.Errorf("discord: session not initialized")
	}
	_, err := d.session.ChannelMessageSend(channelID, message)
	if err != nil {
		return fmt.Errorf("discord: send: %w", err)
	}
	return nil
}
