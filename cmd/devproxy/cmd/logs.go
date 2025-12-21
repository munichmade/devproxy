package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/munichmade/devproxy/internal/paths"
)

var (
	logsFollow bool
	logsSince  string
	logsLines  int
	logsLevel  string
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View daemon logs",
	Long: `Display logs from the devproxy daemon.

Examples:
  devproxy logs              # Show recent logs
  devproxy logs -f           # Follow log output
  devproxy logs --lines 100  # Show last 100 lines
  devproxy logs --since 1h   # Show logs from last hour
  devproxy logs --level error # Filter by log level`,
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVar(&logsSince, "since", "", "Show logs since duration (e.g., 1h, 30m, 24h)")
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 50, "Number of lines to show")
	logsCmd.Flags().StringVar(&logsLevel, "level", "", "Filter by log level (debug, info, warn, error)")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	switch runtime.GOOS {
	case "darwin":
		return runLogsMacOS()
	case "linux":
		return runLogsLinux()
	default:
		return runLogsFromFile()
	}
}

// runLogsMacOS uses the unified log system on macOS
func runLogsMacOS() error {
	// First try to use log show for launchd-managed daemon
	args := []string{"show", "--predicate", "subsystem == 'com.devproxy.daemon'", "--style", "compact"}

	if logsSince != "" {
		duration, err := parseDuration(logsSince)
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}
		startTime := time.Now().Add(-duration).Format("2006-01-02 15:04:05")
		args = append(args, "--start", startTime)
	} else {
		args = append(args, "--last", fmt.Sprintf("%dm", max(logsLines/10, 5)))
	}

	if logsFollow {
		// Use log stream for following
		streamArgs := []string{"stream", "--predicate", "subsystem == 'com.devproxy.daemon'", "--style", "compact"}
		execCmd := exec.Command("log", streamArgs...)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
		return execCmd.Run()
	}

	execCmd := exec.Command("log", args...)
	output, err := execCmd.Output()
	if err != nil || len(output) == 0 {
		// Fall back to log file
		return runLogsFromFile()
	}

	lines := strings.Split(string(output), "\n")
	filtered := filterLogLines(lines)
	printLogLines(filtered)
	return nil
}

// runLogsLinux uses journalctl on Linux
func runLogsLinux() error {
	args := []string{"-u", "devproxy", "--no-pager"}

	if logsFollow {
		args = append(args, "-f")
	} else {
		args = append(args, "-n", fmt.Sprintf("%d", logsLines))
	}

	if logsSince != "" {
		args = append(args, "--since", logsSince+" ago")
	}

	execCmd := exec.Command("journalctl", args...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		// Fall back to log file if journalctl fails
		return runLogsFromFile()
	}
	return nil
}

// runLogsFromFile reads logs from the log file
func runLogsFromFile() error {
	logFile := filepath.Join(paths.DataDir(), "devproxy.log")

	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No log file found. The daemon may not have been started yet.")
			fmt.Printf("Expected log file: %s\n", logFile)
			return nil
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	if logsFollow {
		return followLogFile(file)
	}

	// Read all lines and get the last N
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading log file: %w", err)
	}

	// Apply since filter if specified
	if logsSince != "" {
		duration, err := parseDuration(logsSince)
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}
		lines = filterByTime(lines, duration)
	}

	// Get last N lines
	start := 0
	if len(lines) > logsLines {
		start = len(lines) - logsLines
	}
	lines = lines[start:]

	// Filter by level if specified
	filtered := filterLogLines(lines)
	printLogLines(filtered)

	return nil
}

// followLogFile tails the log file
func followLogFile(file *os.File) error {
	// Seek to end of file
	_, err := file.Seek(0, 2)
	if err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}

	fmt.Println("Following logs (Ctrl+C to stop)...")

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// No new data, wait a bit
			time.Sleep(100 * time.Millisecond)
			continue
		}

		line = strings.TrimSuffix(line, "\n")
		if logsLevel == "" || matchesLevel(line, logsLevel) {
			printLogLine(line)
		}
	}
}

// filterLogLines filters lines by level if specified
func filterLogLines(lines []string) []string {
	if logsLevel == "" {
		return lines
	}

	var filtered []string
	for _, line := range lines {
		if matchesLevel(line, logsLevel) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

// matchesLevel checks if a log line matches the specified level
func matchesLevel(line, level string) bool {
	level = strings.ToUpper(level)
	line = strings.ToUpper(line)

	switch level {
	case "DEBUG":
		return strings.Contains(line, "DEBUG")
	case "INFO":
		return strings.Contains(line, "INFO")
	case "WARN", "WARNING":
		return strings.Contains(line, "WARN")
	case "ERROR", "ERR":
		return strings.Contains(line, "ERROR") || strings.Contains(line, "ERR")
	default:
		return true
	}
}

// filterByTime filters log lines by time
func filterByTime(lines []string, duration time.Duration) []string {
	cutoff := time.Now().Add(-duration)
	var filtered []string

	for _, line := range lines {
		// Try to extract timestamp from log line
		// Common format: 2006-01-02T15:04:05 or 2006/01/02 15:04:05
		if ts := extractTimestamp(line); !ts.IsZero() {
			if ts.After(cutoff) {
				filtered = append(filtered, line)
			}
		} else {
			// Include lines without timestamps
			filtered = append(filtered, line)
		}
	}

	return filtered
}

// extractTimestamp tries to extract a timestamp from a log line
func extractTimestamp(line string) time.Time {
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		time.RFC3339,
	}

	// Try to find a timestamp at the beginning of the line
	for _, format := range formats {
		if len(line) >= len(format) {
			if t, err := time.Parse(format, line[:len(format)]); err == nil {
				return t
			}
		}
	}

	// Try to find time= in slog format
	if idx := strings.Index(line, "time="); idx >= 0 {
		rest := line[idx+5:]
		end := strings.IndexAny(rest, " \t")
		if end == -1 {
			end = len(rest)
		}
		timeStr := rest[:end]
		for _, format := range formats {
			if t, err := time.Parse(format, timeStr); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}

// printLogLines prints log lines with optional colorization
func printLogLines(lines []string) {
	for _, line := range lines {
		printLogLine(line)
	}
}

// printLogLine prints a single log line with colorization
func printLogLine(line string) {
	// Simple colorization based on level
	upper := strings.ToUpper(line)

	if strings.Contains(upper, "ERROR") || strings.Contains(upper, "ERR") {
		fmt.Printf("\033[31m%s\033[0m\n", line) // Red
	} else if strings.Contains(upper, "WARN") {
		fmt.Printf("\033[33m%s\033[0m\n", line) // Yellow
	} else if strings.Contains(upper, "DEBUG") {
		fmt.Printf("\033[36m%s\033[0m\n", line) // Cyan
	} else {
		fmt.Println(line)
	}
}

// parseDuration parses a duration string like "1h", "30m", "24h"
func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}
