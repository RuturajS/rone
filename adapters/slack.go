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

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
	"github.com/slack-go/slack/slackevents"
)

// SlackAdapter implements Adapter for Slack using Socket Mode.
type SlackAdapter struct {
	token    string
	appToken string
	client   *slack.Client
	handler  MessageHandler
}

// NewSlackAdapter creates a new Slack adapter.
func NewSlackAdapter(token, appToken string) *SlackAdapter {
	return &SlackAdapter{token: token, appToken: appToken}
}

func (s *SlackAdapter) Name() string { return "slack" }

func (s *SlackAdapter) SetHandler(fn MessageHandler) {
	s.handler = fn
}

func (s *SlackAdapter) Start(ctx context.Context) error {
	api := slack.New(
		s.token,
		slack.OptionAppLevelToken(s.appToken),
	)
	s.client = api

	sm := socketmode.New(api)

	go func() {
		for evt := range sm.Events {
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				evtAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				sm.Ack(*evt.Request)

				switch ev := evtAPI.InnerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					// Ignore bot messages
					if ev.BotID != "" {
						continue
					}
					if s.handler != nil {
						s.handler(IncomingMessage{
							Platform:  "slack",
							ChannelID: ev.Channel,
							Sender:    ev.User,
							Content:   ev.Text,
						})
					}
				}
			}
		}
	}()

	slog.Info("slack adapter started")

	errCh := make(chan error, 1)
	go func() {
		errCh <- sm.RunContext(ctx)
	}()

	select {
	case <-ctx.Done():
		slog.Info("slack adapter stopping")
		return nil
	case err := <-errCh:
		return fmt.Errorf("slack: socket mode: %w", err)
	}
}

func (s *SlackAdapter) SendTyping(_ string) {
	// Slack doesn't support bot typing indicators in socket mode
}

func (s *SlackAdapter) Send(channelID string, message string) error {
	if s.client == nil {
		return fmt.Errorf("slack: client not initialized")
	}
	_, _, err := s.client.PostMessage(channelID, slack.MsgOptionText(message, false))
	if err != nil {
		return fmt.Errorf("slack: send: %w", err)
	}
	return nil
}

