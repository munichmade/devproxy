package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/munichmade/devproxy/internal/paths"
	"github.com/munichmade/devproxy/internal/privilege"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "init-config" {
		rootCmd.SetArgs([]string{"init-config"})
		if err := rootCmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestChownRecursive(t *testing.T) {
	t.Run("returns error for missing path", func(t *testing.T) {
		missingPath := filepath.Join(t.TempDir(), "missing")

		if err := chownRecursive(missingPath, os.Getuid(), os.Getgid()); err == nil {
			t.Error("chownRecursive() error = nil, want error for missing path")
		}
	})

	t.Run("does not follow symlinks", func(t *testing.T) {
		gid := alternateGroupID(t)
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		link := filepath.Join(dir, "link")
		if err := os.WriteFile(target, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}

		before := fileGroupID(t, target)
		if err := chownRecursive(link, os.Getuid(), gid); err != nil {
			t.Fatal(err)
		}
		if after := fileGroupID(t, target); after != before {
			t.Errorf("target group = %d, want %d", after, before)
		}
	})
}

func TestLoadConfigCreatesUserOwnedFiles(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	originalUser, err := privilege.GetOriginalUser()
	if err != nil || originalUser == nil {
		t.Skip("requires SUDO_UID and SUDO_GID")
	}

	configHome := t.TempDir()
	if err := os.Chown(configHome, originalUser.UID, originalUser.GID); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	paths.Reset()
	defer paths.Reset()

	if _, err := loadConfig(originalUser); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.ConfigDir(), paths.ConfigFile()} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		stat := info.Sys().(*syscall.Stat_t)
		if int(stat.Uid) != originalUser.UID || int(stat.Gid) != originalUser.GID {
			t.Errorf("%s owner = %d:%d, want %d:%d", path, stat.Uid, stat.Gid, originalUser.UID, originalUser.GID)
		}
	}
}

func alternateGroupID(t *testing.T) int {
	t.Helper()
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, gid := range groups {
		if gid != os.Getgid() {
			return gid
		}
	}
	t.Skip("no supplementary group available")
	return 0
}

func fileGroupID(t *testing.T, path string) uint32 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("unexpected file info type %T", info.Sys())
	}
	return stat.Gid
}
