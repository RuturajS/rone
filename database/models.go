/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package database

// Channel represents a registered messaging source.
type Channel struct {
	ID        int64
	Platform  string
	ChannelID string
	Name      string
	CreatedAt string
}

// Message represents an ingested raw message.
type Message struct {
	ID        int64
	ChannelID int64
	Sender    string
	Content   string
	Intent    string // "conversation" | "task"
	Responded int
	CreatedAt string
}

// Task represents an extracted actionable item.
type Task struct {
	ID            int64
	MessageID     *int64 // nullable
	ChannelID     int64
	Description   string
	Status        string // "pending" | "running" | "done" | "failed"
	TaskType      string // "once" | "recurring"
	CronExpr      *string
	ScheduledTime string
	NextRun       *string
	CreatedAt     string
	UpdatedAt     string
}

// ExecutionLog is an audit record for task execution.
type ExecutionLog struct {
	ID         int64
	TaskID     int64
	StartedAt  string
	FinishedAt *string
	Status     string // "success" | "failure"
	Result     *string
	Error      *string
}

