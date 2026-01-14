package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/config"
	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/munichmade/devproxy/internal/proxy"
)

var statusJSONOutput bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status and proxied services",
	Long: `Display the current status of the devproxy daemon including:

  - Whether the daemon is running
  - Configured entrypoints (HTTP, HTTPS, TCP)
  - Currently proxied services
  - Certificate status`,
	Run: func(cmd *cobra.Command, args []string) {
		status := getStatus()

		if statusJSONOutput {
			outputStatusJSON(status)
		} else {
			outputStatusText(status)
		}
	},
}

// Status represents the current state of devproxy.
type Status struct {
	Running     bool          `json:"running"`
	PID         int           `json:"pid,omitempty"`
	Uptime      string        `json:"uptime,omitempty"`
	Entrypoints []Entrypoint  `json:"entrypoints"`
	Routes      []RouteStatus `json:"routes"`
}

// Entrypoint represents a listening endpoint.
type Entrypoint struct {
	Name     string `json:"name"`
	Listen   string `json:"listen"`
	Protocol string `json:"protocol"`
	Status   string `json:"status"`
}

// RouteStatus represents a proxied route.
type RouteStatus struct {
	Host          string `json:"host"`
	Backend       string `json:"backend"`
	ContainerName string `json:"container_name,omitempty"`
	ContainerID   string `json:"container_id,omitempty"`
	Protocol      string `json:"protocol"`
}

func getStatus() Status {
	d := daemon.New()

	status := Status{
		Running:     d.IsRunning(),
		Entrypoints: []Entrypoint{},
		Routes:      []RouteStatus{},
	}

	if status.Running {
		pid, _ := d.GetPID()
		status.PID = pid
	}

	// Load config to show actual configured ports
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}

	// Build entrypoints from config
	if cfg.DNS.Enabled {
		status.Entrypoints = append(status.Entrypoints,
			Entrypoint{Name: "dns", Listen: cfg.DNS.Listen, Protocol: "DNS", Status: getListenerStatus(status.Running)})
	}

	if ep, ok := cfg.Entrypoints["http"]; ok {
		status.Entrypoints = append(status.Entrypoints,
			Entrypoint{Name: "http", Listen: ep.Listen, Protocol: "HTTP", Status: getListenerStatus(status.Running)})
	}

	if ep, ok := cfg.Entrypoints["https"]; ok {
		status.Entrypoints = append(status.Entrypoints,
			Entrypoint{Name: "https", Listen: ep.Listen, Protocol: "HTTPS", Status: getListenerStatus(status.Running)})
	}

	// Add TCP entrypoints
	for name, ep := range cfg.Entrypoints {
		if name == "http" || name == "https" {
			continue
		}
		if ep.TargetPort > 0 {
			status.Entrypoints = append(status.Entrypoints,
				Entrypoint{Name: name, Listen: ep.Listen, Protocol: "TCP", Status: getListenerStatus(status.Running)})
		}
	}

	// Load routes from state file (written by daemon)
	if status.Running {
		routes, err := proxy.LoadState()
		if err == nil && len(routes) > 0 {
			for _, route := range routes {
				status.Routes = append(status.Routes, RouteStatus{
					Host:          route.Host,
					Backend:       route.Backend,
					ContainerName: route.ContainerName,
					ContainerID:   route.ContainerID,
					Protocol:      string(route.Protocol),
				})
			}
		}
	}

	return status
}

func getListenerStatus(running bool) string {
	if running {
		return "listening"
	}
	return "stopped"
}

func outputStatusText(status Status) {
	if status.Running {
		fmt.Printf("devproxy is running (pid %d)\n", status.PID)
	} else {
		fmt.Println("devproxy is not running")
		return
	}

	fmt.Println()

	// Entrypoints
	fmt.Println("Entrypoints:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  NAME\tLISTEN\tPROTOCOL\tSTATUS\n")
	for _, ep := range status.Entrypoints {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", ep.Name, ep.Listen, ep.Protocol, ep.Status)
	}
	w.Flush()

	fmt.Println()

	// Routes
	if len(status.Routes) == 0 {
		fmt.Println("Routes: none")
	} else {
		fmt.Println("Routes:")
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "  HOST\tBACKEND\tCONTAINER\n")
		for _, route := range status.Routes {
			container := route.ContainerName
			if container == "" {
				container = "-"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\n", route.Host, route.Backend, container)
		}
		w.Flush()
	}
}

func outputStatusJSON(status Status) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(status)
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSONOutput, "json", false, "Output in JSON format")
	rootCmd.AddCommand(statusCmd)
}
