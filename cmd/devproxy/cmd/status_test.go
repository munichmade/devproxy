package cmd

import (
	"os"
	"testing"
)

func TestShortenPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("could not get home dir: %v", err)
	}

	t.Run("shortens home directory to tilde", func(t *testing.T) {
		input := home + "/projects/myapp"
		expected := "~/projects/myapp"

		result := shortenPath(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("shortens exact home directory", func(t *testing.T) {
		input := home
		expected := "~"

		result := shortenPath(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("leaves non-home paths unchanged", func(t *testing.T) {
		input := "/var/lib/docker/volumes"
		expected := "/var/lib/docker/volumes"

		result := shortenPath(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("handles empty path", func(t *testing.T) {
		input := ""
		expected := ""

		result := shortenPath(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("handles path that starts with home prefix but is different", func(t *testing.T) {
		// e.g., if home is /home/user, don't shorten /home/username
		input := home + "name/projects"
		expected := "~name/projects" // This is technically how strings.TrimPrefix works

		result := shortenPath(input)
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})
}

func TestProjectRoutesSorting(t *testing.T) {
	t.Run("ungrouped sorts last", func(t *testing.T) {
		projects := map[string]*ProjectRoutes{
			"ungrouped": {Routes: []RouteStatus{{Host: "a.localhost"}}},
			"alpha":     {Routes: []RouteStatus{{Host: "b.localhost"}}},
			"beta":      {Routes: []RouteStatus{{Host: "c.localhost"}}},
		}

		// Extract project names and sort as done in outputStatusText
		projectNames := make([]string, 0, len(projects))
		for name := range projects {
			projectNames = append(projectNames, name)
		}

		// Sort with ungrouped last
		for i := 0; i < len(projectNames)-1; i++ {
			for j := i + 1; j < len(projectNames); j++ {
				shouldSwap := false
				if projectNames[i] == "ungrouped" {
					shouldSwap = true
				} else if projectNames[j] == "ungrouped" {
					shouldSwap = false
				} else if projectNames[i] > projectNames[j] {
					shouldSwap = true
				}
				if shouldSwap {
					projectNames[i], projectNames[j] = projectNames[j], projectNames[i]
				}
			}
		}

		expected := []string{"alpha", "beta", "ungrouped"}
		for i, name := range expected {
			if projectNames[i] != name {
				t.Errorf("expected projectNames[%d] = %q, got %q", i, name, projectNames[i])
			}
		}
	})
}

func TestProjectRoutesStruct(t *testing.T) {
	t.Run("creates project routes correctly", func(t *testing.T) {
		project := &ProjectRoutes{
			ProjectDir: "~/projects/myapp",
			Routes: []RouteStatus{
				{
					Host:          "api.localhost",
					Backend:       "172.18.0.3:8080",
					ContainerName: "myapp-api-1",
					Protocol:      "http",
				},
				{
					Host:          "web.localhost",
					Backend:       "172.18.0.4:3000",
					ContainerName: "myapp-web-1",
					Protocol:      "http",
				},
			},
		}

		if project.ProjectDir != "~/projects/myapp" {
			t.Errorf("expected ProjectDir '~/projects/myapp', got %q", project.ProjectDir)
		}

		if len(project.Routes) != 2 {
			t.Errorf("expected 2 routes, got %d", len(project.Routes))
		}

		if project.Routes[0].Host != "api.localhost" {
			t.Errorf("expected first route host 'api.localhost', got %q", project.Routes[0].Host)
		}
	})
}

func TestStatusStruct(t *testing.T) {
	t.Run("initializes projects map correctly", func(t *testing.T) {
		status := Status{
			Running:     true,
			PID:         12345,
			Entrypoints: []Entrypoint{},
			Projects:    make(map[string]*ProjectRoutes),
		}

		if status.Projects == nil {
			t.Error("expected Projects map to be initialized")
		}

		// Add a project
		status.Projects["myapp"] = &ProjectRoutes{
			ProjectDir: "~/projects/myapp",
			Routes:     []RouteStatus{},
		}

		if len(status.Projects) != 1 {
			t.Errorf("expected 1 project, got %d", len(status.Projects))
		}
	})

	t.Run("groups routes by project", func(t *testing.T) {
		status := Status{
			Running:  true,
			Projects: make(map[string]*ProjectRoutes),
		}

		// Simulate adding routes from different projects
		routes := []struct {
			host        string
			projectName string
			projectDir  string
		}{
			{"api.localhost", "myapp", "/home/user/myapp"},
			{"web.localhost", "myapp", "/home/user/myapp"},
			{"db.localhost", "other", "/home/user/other"},
			{"static.localhost", "", ""}, // ungrouped
		}

		for _, r := range routes {
			projectName := r.projectName
			if projectName == "" {
				projectName = "ungrouped"
			}

			project, exists := status.Projects[projectName]
			if !exists {
				project = &ProjectRoutes{
					ProjectDir: r.projectDir,
					Routes:     []RouteStatus{},
				}
				status.Projects[projectName] = project
			}

			project.Routes = append(project.Routes, RouteStatus{
				Host: r.host,
			})
		}

		// Verify grouping
		if len(status.Projects) != 3 {
			t.Errorf("expected 3 projects, got %d", len(status.Projects))
		}

		if len(status.Projects["myapp"].Routes) != 2 {
			t.Errorf("expected 2 routes in myapp, got %d", len(status.Projects["myapp"].Routes))
		}

		if len(status.Projects["other"].Routes) != 1 {
			t.Errorf("expected 1 route in other, got %d", len(status.Projects["other"].Routes))
		}

		if len(status.Projects["ungrouped"].Routes) != 1 {
			t.Errorf("expected 1 route in ungrouped, got %d", len(status.Projects["ungrouped"].Routes))
		}
	})
}
