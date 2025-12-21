package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/munichmade/devproxy/internal/config"
	"github.com/munichmade/devproxy/internal/paths"
)

var configFormat string

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `View and edit devproxy configuration.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current configuration with resolved defaults.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		switch configFormat {
		case "json":
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}
			fmt.Println(string(data))
		case "yaml":
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}
			fmt.Print(string(data))
		default:
			return fmt.Errorf("unknown format: %s (use 'json' or 'yaml')", configFormat)
		}

		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	Long:  `Display the path to the configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		configFile := paths.ConfigFile()
		fmt.Println(configFile)

		// Check if file exists
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "(file does not exist yet)")
		}
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration file",
	Long:  `Open the configuration file in your default editor ($EDITOR).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			// Try common editors
			for _, e := range []string{"vim", "vi", "nano", "code"} {
				if _, err := exec.LookPath(e); err == nil {
					editor = e
					break
				}
			}
		}
		if editor == "" {
			return fmt.Errorf("no editor found: set $EDITOR environment variable")
		}

		configFile := paths.ConfigFile()

		// Ensure config directory exists
		if err := paths.EnsureDirectories(); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		// Create default config if it doesn't exist
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			cfg := config.Default()
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to marshal default config: %w", err)
			}
			if err := os.WriteFile(configFile, data, 0600); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}
			fmt.Printf("Created default configuration at %s\n", configFile)
		}

		// Open editor
		editorCmd := exec.Command(editor, configFile)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		return editorCmd.Run()
	},
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	Long:  `Check the configuration file for syntax errors and invalid values.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile := paths.ConfigFile()

		// Check if file exists
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			fmt.Println("No configuration file found at", configFile)
			fmt.Println("Using default configuration (valid)")
			return nil
		}

		// Try to load the configuration
		cfg, err := config.LoadFromFile(configFile)
		if err != nil {
			return fmt.Errorf("configuration invalid: %w", err)
		}

		// Perform additional validation
		var warnings []string

		// Check DNS domains
		if len(cfg.DNS.Domains) == 0 {
			warnings = append(warnings, "No DNS domains configured")
		}

		// Check DNS listen address
		if cfg.DNS.Listen == "" {
			warnings = append(warnings, "DNS listen address is empty, DNS server will not start")
		}

		// Check for entrypoints
		if len(cfg.Entrypoints) == 0 {
			warnings = append(warnings, "No entrypoints configured")
		}

		fmt.Printf("Configuration file: %s\n", configFile)
		fmt.Println("Status: Valid")

		if len(warnings) > 0 {
			fmt.Println("\nWarnings:")
			for _, w := range warnings {
				fmt.Printf("  - %s\n", w)
			}
		}

		return nil
	},
}

func init() {
	configShowCmd.Flags().StringVarP(&configFormat, "format", "f", "yaml", "Output format (yaml, json)")

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(configCmd)
}
