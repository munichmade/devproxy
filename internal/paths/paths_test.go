package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setEnv sets an environment variable and returns a cleanup function to restore the original value.
func setEnv(t *testing.T, key, value string) func() {
	t.Helper()
	oldVal := os.Getenv(key)
	os.Setenv(key, value)
	return func() {
		if oldVal == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, oldVal)
		}
	}
}

// clearXDGEnv clears all XDG environment variables and returns a cleanup function.
func clearXDGEnv(t *testing.T) func() {
	t.Helper()
	cleanupConfig := setEnv(t, "XDG_CONFIG_HOME", "")
	cleanupData := setEnv(t, "XDG_DATA_HOME", "")
	cleanupRuntime := setEnv(t, "XDG_RUNTIME_DIR", "")

	os.Unsetenv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_DATA_HOME")
	os.Unsetenv("XDG_RUNTIME_DIR")

	return func() {
		cleanupConfig()
		cleanupData()
		cleanupRuntime()
	}
}

func TestDefault(t *testing.T) {
	Reset()
	defer Reset()

	p := Default()

	// Basic sanity checks - paths should not be empty
	if p.ConfigDir == "" {
		t.Error("ConfigDir is empty")
	}
	if p.DataDir == "" {
		t.Error("DataDir is empty")
	}
	if p.RuntimeDir == "" {
		t.Error("RuntimeDir is empty")
	}
	if p.CADir == "" {
		t.Error("CADir is empty")
	}
	if p.CertsDir == "" {
		t.Error("CertsDir is empty")
	}
	if p.ConfigFile == "" {
		t.Error("ConfigFile is empty")
	}
	if p.PIDFile == "" {
		t.Error("PIDFile is empty")
	}
	if p.LogFile == "" {
		t.Error("LogFile is empty")
	}

	// All paths should contain "devproxy"
	if !strings.Contains(p.ConfigDir, "devproxy") {
		t.Errorf("ConfigDir %q does not contain 'devproxy'", p.ConfigDir)
	}
	if !strings.Contains(p.DataDir, "devproxy") {
		t.Errorf("DataDir %q does not contain 'devproxy'", p.DataDir)
	}
}

func TestDefaultCaching(t *testing.T) {
	Reset()
	defer Reset()

	p1 := Default()
	p2 := Default()

	// Should return same instance
	if p1 != p2 {
		t.Error("Default() should return cached instance")
	}
}

func TestXDGConfigHome(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	cleanup := setEnv(t, "XDG_CONFIG_HOME", tmpDir)
	defer cleanup()

	p := Default()

	expected := filepath.Join(tmpDir, "devproxy")
	if p.ConfigDir != expected {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, expected)
	}
}

func TestXDGDataHome(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	cleanup := setEnv(t, "XDG_DATA_HOME", tmpDir)
	defer cleanup()

	p := Default()

	expected := filepath.Join(tmpDir, "devproxy")
	if p.DataDir != expected {
		t.Errorf("DataDir = %q, want %q", p.DataDir, expected)
	}

	// CADir and CertsDir should be under DataDir
	expectedCA := filepath.Join(expected, "ca")
	if p.CADir != expectedCA {
		t.Errorf("CADir = %q, want %q", p.CADir, expectedCA)
	}

	expectedCerts := filepath.Join(expected, "certs")
	if p.CertsDir != expectedCerts {
		t.Errorf("CertsDir = %q, want %q", p.CertsDir, expectedCerts)
	}
}

func TestXDGRuntimeDir(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	cleanup := setEnv(t, "XDG_RUNTIME_DIR", tmpDir)
	defer cleanup()

	p := Default()

	expected := filepath.Join(tmpDir, "devproxy")
	if p.RuntimeDir != expected {
		t.Errorf("RuntimeDir = %q, want %q", p.RuntimeDir, expected)
	}

	// PIDFile should be under RuntimeDir
	expectedPID := filepath.Join(expected, "devproxy.pid")
	if p.PIDFile != expectedPID {
		t.Errorf("PIDFile = %q, want %q", p.PIDFile, expectedPID)
	}
}

func TestDefaultPaths_NoXDG(t *testing.T) {
	Reset()
	defer Reset()

	cleanup := clearXDGEnv(t)
	defer cleanup()

	p := Default()
	home := os.Getenv("HOME")

	// ConfigDir should be ~/.config/devproxy
	expectedConfig := filepath.Join(home, ".config", "devproxy")
	if p.ConfigDir != expectedConfig {
		t.Errorf("ConfigDir = %q, want %q", p.ConfigDir, expectedConfig)
	}

	// DataDir depends on platform
	if runtime.GOOS == "darwin" {
		expectedData := filepath.Join(home, "Library", "Application Support", "devproxy")
		if p.DataDir != expectedData {
			t.Errorf("DataDir = %q, want %q (darwin)", p.DataDir, expectedData)
		}
	} else {
		expectedData := filepath.Join(home, ".local", "share", "devproxy")
		if p.DataDir != expectedData {
			t.Errorf("DataDir = %q, want %q (linux)", p.DataDir, expectedData)
		}
	}

	// RuntimeDir should fall back to DataDir when XDG_RUNTIME_DIR is not set
	if p.RuntimeDir != p.DataDir {
		t.Errorf("RuntimeDir = %q, want %q (fallback to DataDir)", p.RuntimeDir, p.DataDir)
	}
}

func TestFileNames(t *testing.T) {
	Reset()
	defer Reset()

	p := Default()

	// ConfigFile should be config.yaml
	if filepath.Base(p.ConfigFile) != "config.yaml" {
		t.Errorf("ConfigFile base = %q, want %q", filepath.Base(p.ConfigFile), "config.yaml")
	}

	// PIDFile should be devproxy.pid
	if filepath.Base(p.PIDFile) != "devproxy.pid" {
		t.Errorf("PIDFile base = %q, want %q", filepath.Base(p.PIDFile), "devproxy.pid")
	}

	// LogFile should be devproxy.log
	if filepath.Base(p.LogFile) != "devproxy.log" {
		t.Errorf("LogFile base = %q, want %q", filepath.Base(p.LogFile), "devproxy.log")
	}
}

func TestEnsureDirectories(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	cleanupConfig := setEnv(t, "XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	cleanupData := setEnv(t, "XDG_DATA_HOME", filepath.Join(tmpDir, "data"))
	cleanupRuntime := setEnv(t, "XDG_RUNTIME_DIR", filepath.Join(tmpDir, "runtime"))
	defer cleanupConfig()
	defer cleanupData()
	defer cleanupRuntime()

	p := Default()

	// Directories should not exist yet
	if _, err := os.Stat(p.ConfigDir); !os.IsNotExist(err) {
		t.Error("ConfigDir should not exist before EnsureDirectories")
	}

	// Create directories
	if err := p.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}

	// All directories should now exist
	dirs := []struct {
		name string
		path string
	}{
		{"ConfigDir", p.ConfigDir},
		{"DataDir", p.DataDir},
		{"RuntimeDir", p.RuntimeDir},
		{"CADir", p.CADir},
		{"CertsDir", p.CertsDir},
	}

	for _, d := range dirs {
		info, err := os.Stat(d.path)
		if os.IsNotExist(err) {
			t.Errorf("%s was not created", d.name)
			continue
		}
		if err != nil {
			t.Errorf("%s stat error: %v", d.name, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d.name)
		}
		// Check permissions (0700)
		if perm := info.Mode().Perm(); perm != 0700 {
			t.Errorf("%s permissions = %o, want %o", d.name, perm, 0700)
		}
	}
}

func TestConvenienceFunctions(t *testing.T) {
	Reset()
	defer Reset()

	p := Default()

	// Test all convenience functions match Default() values
	if ConfigDir() != p.ConfigDir {
		t.Errorf("ConfigDir() = %q, want %q", ConfigDir(), p.ConfigDir)
	}
	if DataDir() != p.DataDir {
		t.Errorf("DataDir() = %q, want %q", DataDir(), p.DataDir)
	}
	if RuntimeDir() != p.RuntimeDir {
		t.Errorf("RuntimeDir() = %q, want %q", RuntimeDir(), p.RuntimeDir)
	}
	if CADir() != p.CADir {
		t.Errorf("CADir() = %q, want %q", CADir(), p.CADir)
	}
	if CertsDir() != p.CertsDir {
		t.Errorf("CertsDir() = %q, want %q", CertsDir(), p.CertsDir)
	}
	if ConfigFile() != p.ConfigFile {
		t.Errorf("ConfigFile() = %q, want %q", ConfigFile(), p.ConfigFile)
	}
	if PIDFile() != p.PIDFile {
		t.Errorf("PIDFile() = %q, want %q", PIDFile(), p.PIDFile)
	}
	if LogFile() != p.LogFile {
		t.Errorf("LogFile() = %q, want %q", LogFile(), p.LogFile)
	}
}

func TestReset(t *testing.T) {
	Reset()

	tmpDir1 := t.TempDir()
	cleanup1 := setEnv(t, "XDG_CONFIG_HOME", tmpDir1)
	p1 := Default()
	cleanup1()

	// Reset and change env
	Reset()
	tmpDir2 := t.TempDir()
	cleanup2 := setEnv(t, "XDG_CONFIG_HOME", tmpDir2)
	p2 := Default()
	cleanup2()

	// Should have different ConfigDir
	if p1.ConfigDir == p2.ConfigDir {
		t.Error("Reset() should allow paths to be recalculated")
	}

	Reset()
}

func TestPackageLevelEnsureDirectories(t *testing.T) {
	Reset()
	defer Reset()

	tmpDir := t.TempDir()
	cleanupConfig := setEnv(t, "XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	cleanupData := setEnv(t, "XDG_DATA_HOME", filepath.Join(tmpDir, "data"))
	defer cleanupConfig()
	defer cleanupData()

	// Use package-level function
	if err := EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}

	// Verify ConfigDir was created
	if _, err := os.Stat(ConfigDir()); os.IsNotExist(err) {
		t.Error("ConfigDir was not created by package-level EnsureDirectories()")
	}
}
