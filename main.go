package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"splunk-tui/internal/cli"
	"splunk-tui/internal/config"
	"splunk-tui/internal/secrets"
	"splunk-tui/internal/ui"
)

var (
	flagInstance       string
	flagConfig         string
	flagKeyringBackend string
	flagSPL            string
	flagJSON           bool
	flagEarliest       string
	flagLatest         string
	flagCount          int
)

func init() {
	flag.StringVar(&flagInstance, "instance", "", "Auto-connect to bookmark by name")
	flag.StringVar(&flagConfig, "config", "", "Path to custom config file (default ~/.config/splunk-tui/config.yaml)")
	flag.StringVar(&flagKeyringBackend, "keyring-backend", "", "Keyring backend override (auto, file, secret-service, keychain, wincred, pass)")
	flag.StringVar(&flagSPL, "spl", "", "Run SPL search non-interactively from CLI and print results")
	flag.BoolVar(&flagJSON, "json", false, "Output search results in JSON format")
	flag.StringVar(&flagEarliest, "earliest", "", "Earliest time for search (default: from config or -24h)")
	flag.StringVar(&flagLatest, "latest", "", "Latest time for search (default: now)")
	flag.IntVar(&flagCount, "count", 0, "Maximum number of events/results to return (0 for all)")
}

func main() {
	flag.Parse()

	// 1. Resolve config path
	configPath := flagConfig
	if configPath == "" {
		defaultPath, err := config.DefaultPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error locating config directory: %v\n", err)
			os.Exit(1)
		}
		configPath = defaultPath
	}

	// 2. Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration from %s: %v\n", configPath, err)
		os.Exit(1)
	}

	if flagKeyringBackend != "" {
		cfg.KeyringBackend = flagKeyringBackend
	}

	// 3. Validate --instance if provided
	if flagInstance != "" {
		if _, ok := cfg.Find(flagInstance); !ok {
			known := cfg.Names()
			if len(known) == 0 {
				fmt.Fprintf(os.Stderr, "Instance %q not found: no instances configured in %s.\n", flagInstance, configPath)
			} else {
				fmt.Fprintf(os.Stderr, "Instance %q not found. Available instances: %s\n", flagInstance, strings.Join(known, ", "))
			}
			os.Exit(1)
		}
	}

	// 4. Initialize secrets store
	keyringDir := filepath.Join(cfg.Dir(), "keyring")
	secStore, err := secrets.Open(secrets.Options{
		Backend: cfg.KeyringBackend,
		Dir:     keyringDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open keyring backend (%v), continuing with in-memory secrets.\n", err)
	}

	// 5. Check if non-interactive SPL CLI search was requested
	splPassed := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "spl" {
			splPassed = true
		}
	})

	if splPassed {
		if strings.TrimSpace(flagSPL) == "" {
			fmt.Fprintf(os.Stderr, "Error: --spl query cannot be empty\n")
			os.Exit(1)
		}

		if strings.TrimSpace(flagInstance) == "" {
			known := cfg.Names()
			if len(known) == 0 {
				fmt.Fprintf(os.Stderr, "Error: --instance is required for CLI search, but no instances are configured in %s\n", configPath)
			} else {
				fmt.Fprintf(os.Stderr, "Error: --instance is required for CLI search. Available instances: %s\n", strings.Join(known, ", "))
			}
			os.Exit(1)
		}

		opts := cli.SearchOptions{
			Config:       cfg,
			Secrets:      secStore,
			InstanceName: flagInstance,
			SPL:          flagSPL,
			JSON:         flagJSON,
			Earliest:     flagEarliest,
			Latest:       flagLatest,
			Count:        flagCount,
		}

		if err := cli.Run(context.Background(), opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 6. Create Bubble Tea root application
	app := ui.NewApp(cfg, secStore, flagInstance)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running splunk-tui: %v\n", err)
		os.Exit(1)
	}
}
