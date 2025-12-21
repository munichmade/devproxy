package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestNew(t *testing.T) {
	d := New()
	if d.pidFile == "" {
		t.Error("PID file path should not be empty")
	}
}

func TestNewWithPIDFile(t *testing.T) {
	pidFile := "/tmp/test.pid"
	d := NewWithPIDFile(pidFile)
	if d.pidFile != pidFile {
		t.Errorf("PID file = %q, want %q", d.pidFile, pidFile)
	}
}

func TestWriteAndGetPID(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")
	d := NewWithPIDFile(pidFile)

	// Write current PID
	if err := d.WritePID(); err != nil {
		t.Fatalf("WritePID() error = %v", err)
	}

	// Read it back
	pid, err := d.GetPID()
	if err != nil {
		t.Fatalf("GetPID() error = %v", err)
	}

	if pid != os.Getpid() {
		t.Errorf("GetPID() = %d, want %d", pid, os.Getpid())
	}
}

func TestGetPID_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")
	d := NewWithPIDFile(pidFile)

	_, err := d.GetPID()
	if err == nil {
		t.Error("GetPID() should return error when file doesn't exist")
	}
}

func TestGetPID_InvalidContent(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")
	d := NewWithPIDFile(pidFile)

	// Write invalid content
	if err := os.WriteFile(pidFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := d.GetPID()
	if err == nil {
		t.Error("GetPID() should return error for invalid PID content")
	}
}

func TestRemovePID(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")
	d := NewWithPIDFile(pidFile)

	// Write PID
	if err := d.WritePID(); err != nil {
		t.Fatalf("WritePID() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		t.Fatal("PID file should exist after WritePID()")
	}

	// Remove PID
	if err := d.RemovePID(); err != nil {
		t.Fatalf("RemovePID() error = %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("PID file should not exist after RemovePID()")
	}
}

func TestRemovePID_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")
	d := NewWithPIDFile(pidFile)

	// Should not error when file doesn't exist
	if err := d.RemovePID(); err != nil {
		t.Errorf("RemovePID() error = %v, want nil", err)
	}
}

func TestIsRunning_CurrentProcess(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")
	d := NewWithPIDFile(pidFile)

	// Write current process PID
	if err := d.WritePID(); err != nil {
		t.Fatalf("WritePID() error = %v", err)
	}

	// Current process should be running
	if !d.IsRunning() {
		t.Error("IsRunning() = false, want true for current process")
	}
}

func TestIsRunning_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")
	d := NewWithPIDFile(pidFile)

	if d.IsRunning() {
		t.Error("IsRunning() = true, want false when no PID file")
	}
}

func TestIsRunning_DeadProcess(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")
	d := NewWithPIDFile(pidFile)

	// Write a PID that definitely doesn't exist (very high number)
	// Note: This test might be flaky on systems with very high PIDs
	deadPID := 99999999
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(deadPID)), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if d.IsRunning() {
		t.Error("IsRunning() = true, want false for dead process")
	}
}

func TestCleanStalePIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")
	d := NewWithPIDFile(pidFile)

	// Write a stale PID
	deadPID := 99999999
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(deadPID)), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// cleanStalePIDFile should remove it
	if err := d.cleanStalePIDFile(); err != nil {
		t.Fatalf("cleanStalePIDFile() error = %v", err)
	}

	// File should be gone
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("stale PID file should be removed")
	}
}

func TestCleanStalePIDFile_RunningProcess(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")
	d := NewWithPIDFile(pidFile)

	// Write current process PID (which is running)
	if err := d.WritePID(); err != nil {
		t.Fatalf("WritePID() error = %v", err)
	}

	// cleanStalePIDFile should NOT remove it
	if err := d.cleanStalePIDFile(); err != nil {
		t.Fatalf("cleanStalePIDFile() error = %v", err)
	}

	// File should still exist
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		t.Error("PID file for running process should not be removed")
	}
}

func TestStop_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")
	d := NewWithPIDFile(pidFile)

	err := d.Stop()
	if err != ErrNotRunning {
		t.Errorf("Stop() error = %v, want %v", err, ErrNotRunning)
	}
}

func TestReload_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "nonexistent.pid")
	d := NewWithPIDFile(pidFile)

	err := d.Reload()
	if err != ErrNotRunning {
		t.Errorf("Reload() error = %v, want %v", err, ErrNotRunning)
	}
}

func TestPIDFile(t *testing.T) {
	pidFile := "/custom/path/test.pid"
	d := NewWithPIDFile(pidFile)

	if d.PIDFile() != pidFile {
		t.Errorf("PIDFile() = %q, want %q", d.PIDFile(), pidFile)
	}
}

func TestIsProcessRunning(t *testing.T) {
	// Current process should be running
	if !isProcessRunning(os.Getpid()) {
		t.Error("isProcessRunning(current PID) = false, want true")
	}

	// Non-existent process should not be running
	if isProcessRunning(99999999) {
		t.Error("isProcessRunning(99999999) = true, want false")
	}
}
