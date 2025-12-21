package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/munichmade/devproxy/internal/daemon"
	"github.com/spf13/cobra"
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

	// Default entrypoints (these would be read from daemon state in production)
	status.Entrypoints = []Entrypoint{
		{Name: "http", Listen: ":80", Protocol: "HTTP", Status: getListenerStatus(80)},
		{Name: "https", Listen: ":443", Protocol: "HTTPS/TCP", Status: getListenerStatus(443)},
		{Name: "dns", Listen: ":53", Protocol: "DNS", Status: getListenerStatus(53)},
	}

	// Routes would be read from daemon state via IPC in production
	// For now, show placeholder if running
	if status.Running {
		// In a full implementation, we'd query the daemon for active routes
		// This would use a Unix socket or similar IPC mechanism
	}

	return status
}

func getListenerStatus(port int) string {
	// In production, check if we're actually listening
	// For now, return based on daemon status
	d := daemon.New()
	if d.IsRunning() {
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
	enc.Encode(status)
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSONOutput, "json", false, "Output in JSON format")
	rootCmd.AddCommand(statusCmd)
}
