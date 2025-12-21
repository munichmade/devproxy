package docker

import (
	"testing"
)

func TestLabelParser_ParseLabels(t *testing.T) {
	parser := NewLabelParser("devproxy")

	t.Run("returns nil when not enabled", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.host": "app.localhost",
		}

		configs, err := parser.ParseLabels(labels)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if configs != nil {
			t.Errorf("expected nil configs, got %v", configs)
		}
	})

	t.Run("parses simple single-service config", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable": "true",
			"devproxy.host":   "app.localhost",
		}

		configs, err := parser.ParseLabels(labels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(configs) != 1 {
			t.Fatalf("expected 1 config, got %d", len(configs))
		}

		if configs[0].Host != "app.localhost" {
			t.Errorf("expected host 'app.localhost', got %q", configs[0].Host)
		}
		if configs[0].Port != 80 {
			t.Errorf("expected default port 80, got %d", configs[0].Port)
		}
	})

	t.Run("parses single-service with custom port", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable": "true",
			"devproxy.host":   "app.localhost",
			"devproxy.port":   "3000",
		}

		configs, err := parser.ParseLabels(labels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if configs[0].Port != 3000 {
			t.Errorf("expected port 3000, got %d", configs[0].Port)
		}
	})

	t.Run("parses single-service with entrypoint", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable":     "true",
			"devproxy.host":       "db.localhost",
			"devproxy.port":       "5432",
			"devproxy.entrypoint": "postgres",
		}

		configs, err := parser.ParseLabels(labels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if configs[0].Entrypoint != "postgres" {
			t.Errorf("expected entrypoint 'postgres', got %q", configs[0].Entrypoint)
		}
	})

	t.Run("returns error for missing host", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable": "true",
		}

		_, err := parser.ParseLabels(labels)
		if err == nil {
			t.Error("expected error for missing host")
		}
	})

	t.Run("returns error for invalid port", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable": "true",
			"devproxy.host":   "app.localhost",
			"devproxy.port":   "invalid",
		}

		_, err := parser.ParseLabels(labels)
		if err == nil {
			t.Error("expected error for invalid port")
		}
	})

	t.Run("returns error for port out of range", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable": "true",
			"devproxy.host":   "app.localhost",
			"devproxy.port":   "70000",
		}

		_, err := parser.ParseLabels(labels)
		if err == nil {
			t.Error("expected error for port out of range")
		}
	})

	t.Run("parses multi-service config", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable":            "true",
			"devproxy.services.web.host": "app.localhost",
			"devproxy.services.web.port": "3000",
			"devproxy.services.api.host": "api.localhost",
			"devproxy.services.api.port": "4000",
		}

		configs, err := parser.ParseLabels(labels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(configs) != 2 {
			t.Fatalf("expected 2 configs, got %d", len(configs))
		}

		// Check that both services are present
		found := make(map[string]bool)
		for _, c := range configs {
			found[c.Name] = true
		}
		if !found["web"] || !found["api"] {
			t.Errorf("expected services 'web' and 'api', got %v", found)
		}
	})

	t.Run("returns error for multi-service missing host", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable":            "true",
			"devproxy.services.web.port": "3000",
		}

		_, err := parser.ParseLabels(labels)
		if err == nil {
			t.Error("expected error for missing host in multi-service")
		}
	})
}

func TestLabelParser_IsEnabled(t *testing.T) {
	parser := NewLabelParser("devproxy")

	t.Run("returns true when enabled", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable": "true",
		}

		if !parser.IsEnabled(labels) {
			t.Error("expected IsEnabled to return true")
		}
	})

	t.Run("returns false when not enabled", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable": "false",
		}

		if parser.IsEnabled(labels) {
			t.Error("expected IsEnabled to return false")
		}
	})

	t.Run("returns false when label missing", func(t *testing.T) {
		labels := map[string]string{}

		if parser.IsEnabled(labels) {
			t.Error("expected IsEnabled to return false")
		}
	})
}

func TestLabelParser_CustomPrefix(t *testing.T) {
	parser := NewLabelParser("myproxy")

	t.Run("uses custom prefix", func(t *testing.T) {
		labels := map[string]string{
			"myproxy.enable": "true",
			"myproxy.host":   "app.localhost",
		}

		configs, err := parser.ParseLabels(labels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(configs) != 1 {
			t.Fatalf("expected 1 config, got %d", len(configs))
		}
	})

	t.Run("ignores default prefix", func(t *testing.T) {
		labels := map[string]string{
			"devproxy.enable": "true",
			"devproxy.host":   "app.localhost",
		}

		configs, err := parser.ParseLabels(labels)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if configs != nil {
			t.Errorf("expected nil configs with wrong prefix, got %v", configs)
		}
	})
}
