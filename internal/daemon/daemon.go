// Package daemon provides daemon lifecycle management including
// PID file handling, process forking, and signal handling.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/munichmade/devproxy/internal/paths"
)

// Common errors
var (
	ErrAlreadyRunning = errors.New("daemon is already running")
	ErrNotRunning     = errors.New("daemon is not running")
	ErrStalePIDFile   = errors.New("stale PID file found")
)

// Daemon manages the daemon lifecycle.
type Daemon struct {
	pidFile string
}

// New creates a new Daemon instance using default paths.
func New() *Daemon {
	return &Daemon{
		pidFile: paths.PIDFile(),
	}
}

// NewWithPIDFile creates a Daemon with a custom PID file path.
func NewWithPIDFile(pidFile string) *Daemon {
	return &Daemon{
		pidFile: pidFile,
	}
}

// Start forks the current process and starts it in the background.
// The parent process returns nil after the child is started.
// Returns ErrAlreadyRunning if daemon is already running.
func (d *Daemon) Start() error {
	// Check if already running
	if d.IsRunning() {
		return ErrAlreadyRunning
	}

	// Clean up stale PID file if exists
	if err := d.cleanStalePIDFile(); err != nil {
		return err
	}

	// Ensure PID file directory exists
	if err := os.MkdirAll(filepath.Dir(d.pidFile), 0700); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}

	// Get the path to the current executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Start the daemon process with 'run' command
	cmd := exec.Command(executable, "run")
	cmd.Env = os.Environ()

	// Detach from parent
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Write PID file
	if err := d.writePIDFile(cmd.Process.Pid); err != nil {
		// Try to kill the process if we can't write PID file
		_ = cmd.Process.Kill()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	return nil
}

// Stop sends SIGTERM to the running daemon.
// Returns ErrNotRunning if daemon is not running.
func (d *Daemon) Stop() error {
	pid, err := d.GetPID()
	if err != nil {
		return ErrNotRunning
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		d.removePIDFile()
		return ErrNotRunning
	}

	// Send SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		if errors.Is(err, os.ErrProcessDone) {
			d.removePIDFile()
			return ErrNotRunning
		}
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	return nil
}

// Reload sends SIGHUP to the running daemon to reload configuration.
// Returns ErrNotRunning if daemon is not running.
func (d *Daemon) Reload() error {
	pid, err := d.GetPID()
	if err != nil {
		return ErrNotRunning
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		d.removePIDFile()
		return ErrNotRunning
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			d.removePIDFile()
			return ErrNotRunning
		}
		return fmt.Errorf("failed to send SIGHUP: %w", err)
	}

	return nil
}

// IsRunning checks if the daemon is currently running.
func (d *Daemon) IsRunning() bool {
	pid, err := d.GetPID()
	if err != nil {
		return false
	}
	return isProcessRunning(pid)
}

// GetPID reads and returns the PID from the PID file.
func (d *Daemon) GetPID() (int, error) {
	data, err := os.ReadFile(d.pidFile)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}

	return pid, nil
}

// WritePID writes the current process PID to the PID file.
// This is called by the daemon process itself after it starts.
func (d *Daemon) WritePID() error {
	return d.writePIDFile(os.Getpid())
}

// RemovePID removes the PID file.
// This is called by the daemon process when it shuts down.
func (d *Daemon) RemovePID() error {
	return d.removePIDFile()
}

// PIDFile returns the path to the PID file.
func (d *Daemon) PIDFile() string {
	return d.pidFile
}

// writePIDFile writes a PID to the PID file.
func (d *Daemon) writePIDFile(pid int) error {
	return os.WriteFile(d.pidFile, []byte(strconv.Itoa(pid)), 0644)
}

// removePIDFile removes the PID file.
func (d *Daemon) removePIDFile() error {
	err := os.Remove(d.pidFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// cleanStalePIDFile removes PID file if the process is no longer running.
func (d *Daemon) cleanStalePIDFile() error {
	pid, err := d.GetPID()
	if err != nil {
		// No PID file or unreadable - that's fine
		return nil
	}

	if !isProcessRunning(pid) {
		// Process is dead, clean up stale PID file
		return d.removePIDFile()
	}

	return nil
}

// isProcessRunning checks if a process with the given PID is running.
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds. We need to send signal 0
	// to check if the process actually exists.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
