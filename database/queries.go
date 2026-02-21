package database

import (
	"fmt"
	"time"
)

// UpsertChannel inserts or updates a channel record and returns its ID.
func (db *DB) UpsertChannel(platform, channelID, name string) (int64, error) {
	var id int64
	err := db.stmts.UpsertChannel.QueryRow(platform, channelID, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert channel: %w", err)
	}
	return id, nil
}

// GetChannel retrieves a channel by platform and channel ID.
func (db *DB) GetChannel(platform, channelID string) (*Channel, error) {
	var ch Channel
	err := db.stmts.GetChannel.QueryRow(platform, channelID).Scan(
		&ch.ID, &ch.Platform, &ch.ChannelID, &ch.Name, &ch.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	return &ch, nil
}

// InsertMessage stores a new message and returns its ID.
func (db *DB) InsertMessage(channelID int64, sender, content, intent string) (int64, error) {
	var id int64
	err := db.stmts.InsertMessage.QueryRow(channelID, sender, content, intent).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert message: %w", err)
	}
	return id, nil
}

// MarkResponded sets the responded flag on a message.
func (db *DB) MarkResponded(messageID int64) error {
	_, err := db.stmts.MarkResponded.Exec(messageID)
	return err
}

// InsertTask stores a new task and returns its ID.
func (db *DB) InsertTask(messageID *int64, channelID int64, description, taskType string, cronExpr *string, scheduledTime string, nextRun *string) (int64, error) {
	var id int64
	err := db.stmts.InsertTask.QueryRow(messageID, channelID, description, taskType, cronExpr, scheduledTime, nextRun).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert task: %w", err)
	}
	return id, nil
}

// GetDueTasks returns all pending tasks whose scheduled_time <= now.
func (db *DB) GetDueTasks(now string) ([]Task, error) {
	rows, err := db.stmts.GetDueTasks.Query(now)
	if err != nil {
		return nil, fmt.Errorf("get due tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.MessageID, &t.ChannelID, &t.Description,
			&t.Status, &t.TaskType, &t.CronExpr, &t.ScheduledTime, &t.NextRun,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// UpdateTaskStatus sets the status of a task.
func (db *DB) UpdateTaskStatus(taskID int64, status string) error {
	_, err := db.stmts.UpdateTaskStatus.Exec(status, taskID)
	return err
}

// UpdateNextRun sets the next scheduled time for a recurring task.
func (db *DB) UpdateNextRun(taskID int64, nextRun string) error {
	_, err := db.stmts.UpdateNextRun.Exec(nextRun, nextRun, taskID)
	return err
}

// GetChannelByID retrieves a channel by its internal DB ID.
func (db *DB) GetChannelByID(id int64) (*Channel, error) {
	var ch Channel
	err := db.conn.QueryRow(
		`SELECT id, platform, channel_id, name, created_at FROM channels WHERE id = ?`, id,
	).Scan(&ch.ID, &ch.Platform, &ch.ChannelID, &ch.Name, &ch.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get channel by id: %w", err)
	}
	return &ch, nil
}

// InsertExecLog records the result of a task execution.
func (db *DB) InsertExecLog(taskID int64, startedAt time.Time, finishedAt *time.Time, status string, result, errMsg *string) error {
	var finStr *string
	if finishedAt != nil {
		s := finishedAt.UTC().Format(time.RFC3339)
		finStr = &s
	}
	_, err := db.stmts.InsertExecLog.Exec(
		taskID,
		startedAt.UTC().Format(time.RFC3339),
		finStr,
		status,
		result,
		errMsg,
	)
	return err
}
