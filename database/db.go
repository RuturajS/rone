package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
)

// DB wraps the sql.DB and holds prepared statements.
type DB struct {
	conn  *sql.DB
	stmts *Statements
}

// Statements holds all prepared statements for reuse.
type Statements struct {
	// Channels
	UpsertChannel *sql.Stmt
	GetChannel    *sql.Stmt

	// Messages
	InsertMessage    *sql.Stmt
	MarkResponded    *sql.Stmt

	// Tasks
	InsertTask       *sql.Stmt
	GetDueTasks      *sql.Stmt
	UpdateTaskStatus *sql.Stmt
	UpdateNextRun    *sql.Stmt

	// Execution logs
	InsertExecLog *sql.Stmt
}

// schema is the DDL for all tables.
const schema = `
CREATE TABLE IF NOT EXISTS channels (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    platform    TEXT    NOT NULL CHECK(platform IN ('telegram','discord','slack')),
    channel_id  TEXT    NOT NULL,
    name        TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    UNIQUE(platform, channel_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id  INTEGER NOT NULL REFERENCES channels(id),
    sender      TEXT    NOT NULL,
    content     TEXT    NOT NULL,
    intent      TEXT    NOT NULL CHECK(intent IN ('conversation','task')),
    responded   INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS tasks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id      INTEGER REFERENCES messages(id),
    channel_id      INTEGER NOT NULL REFERENCES channels(id),
    description     TEXT    NOT NULL,
    status          TEXT    NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','done','failed')),
    task_type       TEXT    NOT NULL DEFAULT 'once' CHECK(task_type IN ('once','recurring')),
    cron_expr       TEXT,
    scheduled_time  TEXT    NOT NULL,
    next_run        TEXT,
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_tasks_scheduled_time ON tasks(scheduled_time);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

CREATE TABLE IF NOT EXISTS execution_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id),
    started_at  TEXT    NOT NULL,
    finished_at TEXT,
    status      TEXT    NOT NULL CHECK(status IN ('success','failure')),
    result      TEXT,
    error       TEXT
);
`

// Open creates the in-memory SQLite database, runs migrations, and prepares statements.
func Open() (*DB, error) {
	conn, err := sql.Open("sqlite", "file:memdb1?mode=memory&cache=shared")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Single connection for in-memory — prevent pool from opening multiple
	conn.SetMaxOpenConns(1)

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	stmts, err := prepare(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("prepare statements: %w", err)
	}

	slog.Info("database initialized", "driver", "sqlite-memory")
	return &DB{conn: conn, stmts: stmts}, nil
}

// migrate executes the schema DDL.
func migrate(conn *sql.DB) error {
	_, err := conn.Exec(schema)
	return err
}

// Migrate is the public migration entry point (used by CLI migrate command).
func Migrate(conn *sql.DB) error {
	return migrate(conn)
}

// prepare creates all prepared statements.
func prepare(conn *sql.DB) (*Statements, error) {
	var s Statements
	var err error

	s.UpsertChannel, err = conn.Prepare(`
		INSERT INTO channels (platform, channel_id, name)
		VALUES (?, ?, ?)
		ON CONFLICT(platform, channel_id) DO UPDATE SET name=excluded.name
		RETURNING id
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare upsert_channel: %w", err)
	}

	s.GetChannel, err = conn.Prepare(`
		SELECT id, platform, channel_id, name, created_at
		FROM channels
		WHERE platform = ? AND channel_id = ?
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare get_channel: %w", err)
	}

	s.InsertMessage, err = conn.Prepare(`
		INSERT INTO messages (channel_id, sender, content, intent)
		VALUES (?, ?, ?, ?)
		RETURNING id
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare insert_message: %w", err)
	}

	s.MarkResponded, err = conn.Prepare(`
		UPDATE messages SET responded = 1 WHERE id = ?
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare mark_responded: %w", err)
	}

	s.InsertTask, err = conn.Prepare(`
		INSERT INTO tasks (message_id, channel_id, description, task_type, cron_expr, scheduled_time, next_run)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare insert_task: %w", err)
	}

	s.GetDueTasks, err = conn.Prepare(`
		SELECT id, message_id, channel_id, description, status, task_type, cron_expr, scheduled_time, next_run
		FROM tasks
		WHERE status = 'pending' AND scheduled_time <= ?
		ORDER BY scheduled_time ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare get_due_tasks: %w", err)
	}

	s.UpdateTaskStatus, err = conn.Prepare(`
		UPDATE tasks SET status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id = ?
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare update_task_status: %w", err)
	}

	s.UpdateNextRun, err = conn.Prepare(`
		UPDATE tasks
		SET next_run = ?, scheduled_time = ?, status = 'pending',
		    updated_at = strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id = ?
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare update_next_run: %w", err)
	}

	s.InsertExecLog, err = conn.Prepare(`
		INSERT INTO execution_logs (task_id, started_at, finished_at, status, result, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare insert_exec_log: %w", err)
	}

	return &s, nil
}

// Close closes all prepared statements and the database connection.
func (db *DB) Close() error {
	stmts := []*sql.Stmt{
		db.stmts.UpsertChannel,
		db.stmts.GetChannel,
		db.stmts.InsertMessage,
		db.stmts.MarkResponded,
		db.stmts.InsertTask,
		db.stmts.GetDueTasks,
		db.stmts.UpdateTaskStatus,
		db.stmts.UpdateNextRun,
		db.stmts.InsertExecLog,
	}
	for _, s := range stmts {
		if s != nil {
			s.Close()
		}
	}
	return db.conn.Close()
}

// Conn returns the underlying sql.DB for direct access if needed.
func (db *DB) Conn() *sql.DB {
	return db.conn
}
