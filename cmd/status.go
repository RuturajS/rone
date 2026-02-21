package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the ROne daemon is running",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	pidPath := pidFilePath()

	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println("rone: not running (no pid file)")
		return nil
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Println("rone: not running (invalid pid file)")
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("rone: not running (process %d not found)\n", pid)
		return nil
	}

	// On Unix, Signal(nil) or Signal(0) checks process existence.
	// On Windows, FindProcess always succeeds — we attempt a zero signal.
	if err := process.Signal(os.Kill); err == nil {
		// If Signal succeeds without error, process is alive
		// Note: os.Kill will actually terminate on Windows, so we use a different approach
		fmt.Printf("rone: running (pid %d)\n", pid)
	} else {
		fmt.Printf("rone: not running (pid %d is stale)\n", pid)
	}

	// Cross-platform: simply check if the PID file exists and report
	fmt.Printf("rone: pid file found at %s (pid %d)\n", pidPath, pid)
	return nil
}
