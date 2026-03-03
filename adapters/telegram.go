/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramAdapter implements Adapter for Telegram using long polling.
type TelegramAdapter struct {
	token   string
	chatID  int64 // allowed chat/channel ID to listen to
	debug   bool  // log full message content
	bot     *tgbotapi.BotAPI
	handler MessageHandler
}

// NewTelegramAdapter creates a new Telegram adapter.
// chatID is the Telegram chat/channel ID the bot should listen to and respond in.
func NewTelegramAdapter(token string, chatID int64, debug bool) *TelegramAdapter {
	return &TelegramAdapter{token: token, chatID: chatID, debug: debug}
}

func (t *TelegramAdapter) Name() string { return "telegram" }

func (t *TelegramAdapter) SetHandler(fn MessageHandler) {
	t.handler = fn
}

// SendTyping sends a "typing..." indicator to the given chat.
func (t *TelegramAdapter) SendTyping(channelID string) {
	chatID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return
	}
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = t.bot.Send(action)
}

func (t *TelegramAdapter) Start(ctx context.Context) error {
	bot, err := tgbotapi.NewBotAPI(t.token)
	if err != nil {
		return fmt.Errorf("telegram: init bot: %w", err)
	}
	t.bot = bot

	// Enable telegram-bot-api debug logging if debug flag is set
	bot.Debug = t.debug

	slog.Info("telegram adapter connected",
		"username", bot.Self.UserName,
		"bot_id", bot.Self.ID,
		"chat_id", t.chatID,
		"debug", t.debug,
	)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := bot.GetUpdatesChan(u)

	slog.Info("telegram: listening for messages...")

	for {
		select {
		case <-ctx.Done():
			bot.StopReceivingUpdates()
			slog.Info("telegram adapter stopped")
			return nil
		case update := <-updates:
			// Handle both regular messages and channel posts
			var msg *tgbotapi.Message
			if update.Message != nil {
				msg = update.Message
			} else if update.ChannelPost != nil {
				msg = update.ChannelPost
			} else {
				continue
			}

			// Get sender name safely (From can be nil for channel posts)
			sender := "unknown"
			if msg.From != nil {
				sender = msg.From.UserName
				if sender == "" {
					sender = msg.From.FirstName
				}
			}

			// Get text content (could be caption for media messages)
			text := msg.Text
			if text == "" {
				text = msg.Caption
			}

			chatIDStr := strconv.FormatInt(msg.Chat.ID, 10)

			// Debug logging — log EVERY incoming message
			if t.debug {
				slog.Info("telegram: incoming message",
					"chat_id", msg.Chat.ID,
					"chat_type", msg.Chat.Type,
					"sender", sender,
					"text", text,
					"message_id", msg.MessageID,
				)
			}

			// Only process messages from the configured chat/channel
			if t.chatID != 0 && msg.Chat.ID != t.chatID {
				slog.Debug("telegram: ignoring message from unknown chat",
					"chat_id", msg.Chat.ID,
					"allowed", t.chatID,
				)
				continue
			}

			if text == "" {
				slog.Debug("telegram: skipping empty message", "chat_id", msg.Chat.ID)
				continue
			}

			slog.Info("telegram: processing message",
				"sender", sender,
				"chat_id", msg.Chat.ID,
				"length", len(text),
			)

			if t.handler != nil {
				t.handler(IncomingMessage{
					Platform:  "telegram",
					ChannelID: chatIDStr,
					Sender:    sender,
					Content:   text,
				})
			}
		}
	}
}

func (t *TelegramAdapter) Send(channelID string, message string) error {
	chatID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram: invalid channel id: %w", err)
	}

	// Telegram max message length is 4096. To be safe with markdown parsing,
	// we chunk the response at 4000 runes.
	const chunkSize = 4000
	runes := []rune(message)

	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		
		chunk := string(runes[i:end])
		msg := tgbotapi.NewMessage(chatID, chunk)
		
		// If using Markdown/HTML parsing, consider enabling it here like so:
		// msg.ParseMode = tgbotapi.ModeMarkdown
		
		_, err = t.bot.Send(msg)
		if err != nil {
			return fmt.Errorf("telegram: send chunk: %w", err)
		}
	}

	slog.Debug("telegram: message sent", "chat_id", chatID, "length", len(message))
	return nil
}

