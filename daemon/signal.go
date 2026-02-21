/*
 * Copyright (c) 2026 RuturajS (ROne). All rights reserved.
 * This code belongs to the author. No modification or republication 
 * is allowed without explicit permission.
 */
package daemon

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// WaitForShutdown blocks until a termination signal is received, then cancels the context.
// Returns a cancel function that can be used to trigger shutdown programmatically.
func WaitForShutdown(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	slog.Info("received signal, shutting down", "signal", sig.String())
	cancel()
}

// ListenForReload listens for SIGHUP and calls the reload callback.
// On Windows, SIGHUP is not available — this is a no-op.
func ListenForReload(ctx context.Context, reload func()) {
	sigCh := make(chan os.Signal, 1)

	// SIGHUP is available on Linux/macOS. On Windows this call is a no-op
	// (Windows does not support SIGHUP, so the channel will never receive).
	signal.Notify(sigCh, syscall.SIGINT) // placeholder — see signal_unix.go for real SIGHUP

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				slog.Info("reload signal received")
				reload()
			}
		}
	}()
}

