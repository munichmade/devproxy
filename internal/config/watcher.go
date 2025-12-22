// Package config provides configuration management for devproxy.
package config

import (
	"os"
	"sync"
	"time"

	"github.com/munichmade/devproxy/internal/logging"
)

// Watcher watches the config file for changes and triggers reloads.
type Watcher struct {
	path     string
	onChange func(*Config)
	stop     chan struct{}
	wg       sync.WaitGroup
	lastMod  time.Time
	interval time.Duration
}

// NewWatcher creates a new config file watcher.
func NewWatcher(path string, onChange func(*Config)) *Watcher {
	return &Watcher{
		path:     path,
		onChange: onChange,
		stop:     make(chan struct{}),
		interval: 2 * time.Second,
	}
}

// Start begins watching the config file for changes.
func (w *Watcher) Start() error {
	// Get initial mod time
	info, err := os.Stat(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's okay
			w.lastMod = time.Time{}
		} else {
			return err
		}
	} else {
		w.lastMod = info.ModTime()
	}

	w.wg.Add(1)
	go w.watch()

	logging.Info("config watcher started", "path", w.path)
	return nil
}

// Stop stops watching the config file.
func (w *Watcher) Stop() {
	close(w.stop)
	w.wg.Wait()
	logging.Info("config watcher stopped")
}

// watch is the main loop that checks for file changes.
func (w *Watcher) watch() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.checkForChanges()
		}
	}
}

// checkForChanges checks if the config file has been modified.
func (w *Watcher) checkForChanges() {
	info, err := os.Stat(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted, reset lastMod
			if !w.lastMod.IsZero() {
				w.lastMod = time.Time{}
				logging.Debug("config file deleted", "path", w.path)
			}
		}
		return
	}

	modTime := info.ModTime()
	if modTime.After(w.lastMod) {
		w.lastMod = modTime
		logging.Info("config file changed, reloading", "path", w.path)

		// Load new config
		cfg, err := Load()
		if err != nil {
			logging.Error("failed to reload config", "error", err)
			return
		}

		// Trigger callback
		if w.onChange != nil {
			w.onChange(cfg)
		}
	}
}
