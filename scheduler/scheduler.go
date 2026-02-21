package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/RuturajS/rone/adapters"
	"github.com/RuturajS/rone/database"
	"github.com/RuturajS/rone/ollama"
)

// Scheduler polls the database for due tasks and executes them.
type Scheduler struct {
	interval time.Duration
	db       *database.DB
	adapters map[string]adapters.Adapter
	ollama   *ollama.Client
}

// New creates a new Scheduler.
func New(interval time.Duration, db *database.DB, adapterMap map[string]adapters.Adapter, ollamaClient *ollama.Client) *Scheduler {
	return &Scheduler{
		interval: interval,
		db:       db,
		adapters: adapterMap,
		ollama:   ollamaClient,
	}
}

// Run starts the scheduler loop, blocking until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	slog.Info("scheduler started", "interval", s.interval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll checks for due tasks and processes them.
func (s *Scheduler) poll(ctx context.Context) {
	now := time.Now().UTC().Format(time.RFC3339)

	tasks, err := s.db.GetDueTasks(now)
	if err != nil {
		slog.Error("scheduler: fetch due tasks", "error", err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	slog.Info("scheduler: found due tasks", "count", len(tasks))

	for _, task := range tasks {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.processTask(ctx, task)
	}
}

// processTask executes a single task via Ollama and updates the database.
func (s *Scheduler) processTask(ctx context.Context, task database.Task) {
	startedAt := time.Now().UTC()

	if err := s.db.UpdateTaskStatus(task.ID, "running"); err != nil {
		slog.Error("scheduler: update task status to running", "task_id", task.ID, "error", err)
		return
	}

	slog.Info("scheduler: executing task", "task_id", task.ID, "description", task.Description)

	// Send the task to Ollama for execution/processing
	result, err := s.ollama.Generate(ctx, "Execute this task and provide the result: "+task.Description)
	finishedAt := time.Now().UTC()

	if err != nil {
		slog.Error("scheduler: task execution failed", "task_id", task.ID, "error", err)
		errMsg := err.Error()
		_ = s.db.InsertExecLog(task.ID, startedAt, &finishedAt, "failure", nil, &errMsg)
		_ = s.db.UpdateTaskStatus(task.ID, "failed")

		// Notify channel about failure
		s.replyToChannel(task, "Task #"+itoa(task.ID)+" failed: "+errMsg)
		return
	}

	slog.Info("scheduler: task completed", "task_id", task.ID, "result_length", len(result))

	// Log success
	_ = s.db.InsertExecLog(task.ID, startedAt, &finishedAt, "success", &result, nil)

	// Handle recurring vs one-time
	if task.TaskType == "recurring" && task.CronExpr != nil {
		cron, err := ParseCron(*task.CronExpr)
		if err != nil {
			slog.Error("scheduler: parse cron", "task_id", task.ID, "cron", *task.CronExpr, "error", err)
			_ = s.db.UpdateTaskStatus(task.ID, "failed")
			return
		}
		next := cron.Next(finishedAt)
		nextStr := next.Format(time.RFC3339)
		if err := s.db.UpdateNextRun(task.ID, nextStr); err != nil {
			slog.Error("scheduler: update next run", "task_id", task.ID, "error", err)
		}
		slog.Info("scheduler: recurring task rescheduled", "task_id", task.ID, "next_run", nextStr)
	} else {
		// One-time task — mark done, never run again
		if err := s.db.UpdateTaskStatus(task.ID, "done"); err != nil {
			slog.Error("scheduler: update task status to done", "task_id", task.ID, "error", err)
		}
		slog.Info("scheduler: one-time task completed and closed", "task_id", task.ID)
	}

	// Send result back to originating channel
	s.replyToChannel(task, result)
}

// replyToChannel sends the task result back to the originating channel.
func (s *Scheduler) replyToChannel(task database.Task, result string) {
	channelRow, err := s.db.GetChannelByID(task.ChannelID)
	if err != nil {
		slog.Error("scheduler: lookup channel for reply", "channel_id", task.ChannelID, "error", err)
		return
	}

	adapter, ok := s.adapters[channelRow.Platform]
	if !ok {
		slog.Error("scheduler: no adapter for platform", "platform", channelRow.Platform)
		return
	}

	if err := adapter.Send(channelRow.ChannelID, result); err != nil {
		slog.Error("scheduler: send reply", "platform", channelRow.Platform, "channel", channelRow.ChannelID, "error", err)
	}
}

func itoa(id int64) string {
	return fmt.Sprintf("%d", id)
}

