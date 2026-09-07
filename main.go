//go:build linux

package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	"github.com/alexflint/go-arg"
	"gopkg.in/yaml.v3"
)

var Client = http.Client{Timeout: 10 * time.Second}

type App struct {
	Depotcache       string
	ApiKey           string
	SLSconfigPath    string
	Name             string
	Config           *yaml.Node
	AdditionalApps   *yaml.Node
	AdditionalDepots *yaml.Node
	DecryptionKeys   *yaml.Node
}

func main() {
	var args struct {
		Appid   int  `arg:"positional,required"`
		Verbose bool `arg:"-v,--verbose" help:"verbosity level"`
	}
	arg.MustParse(&args)
	// lmittmann/tint-like styling
	s := log.DefaultStyles()
	s.Levels[log.DebugLevel] = s.Levels[log.DebugLevel].UnsetForeground().Bold(false)
	s.Levels[log.InfoLevel] = s.Levels[log.InfoLevel].Foreground(lipgloss.Color("2"))
	s.Levels[log.WarnLevel] = s.Levels[log.WarnLevel].Foreground(lipgloss.Color("3"))
	s.Levels[log.ErrorLevel] = s.Levels[log.ErrorLevel].Foreground(lipgloss.Color("1"))
	s.Timestamp = lipgloss.NewStyle().Faint(true)
	for level, style := range s.Levels {
		s.Levels[level] = style.Transform(func(str string) string {
			if len(str) > 2 {
				return str[:3]
			}
			return str
		}).Width(0)
	}
	logger = log.NewWithOptions(os.Stdout, log.Options{
		TimeFormat:      time.Kitchen,
		ReportTimestamp: true,
	})
	logger.SetStyles(s)
	if args.Verbose {
		logger.SetLevel(log.DebugLevel)
	}

	key := os.Getenv("HUBCAB_KEY")
	if key == "" {
		logger.Error("HUBCAB_KEY environment variable not set")
		logger.Info("Please set HUBCAB_KEY to your HubCap API key")
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		logger.Error("Failed to get home directory", "error", err)
		os.Exit(1)
	}
	depotcache := filepath.Join(home, ".steam", "steam")
	if f, err := os.Stat(depotcache); err != nil || !f.IsDir() {
		logger.Error("Steam depotcache not found", "path", depotcache)
		logger.Info("Ensure Steam is installed and has been run once")
		os.Exit(1)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		logger.Error("Failed to locate user .config directory")
		os.Exit(1)
	}
	slsc := filepath.Join(configDir, "SLSsteam", "config.yaml")
	if f, err := os.Stat(slsc); err != nil || f.IsDir() {
		logger.Error("Config file not found", "path", slsc)
		logger.Info("Install SLSsteam and launch once")
		os.Exit(1)
	}

	app := &App{
		Depotcache:    depotcache,
		ApiKey:        key,
		SLSconfigPath: slsc,
	}
	if err := app.Run(args.Appid); err != nil {
		logger.Error("Failed to run", "error", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func (a *App) Run(appid int) error {
	start := time.Now()
	logger.Info("Loading existing config", "step", "1/4")
	root, err := LoadConfig(a.SLSconfigPath)
	if err != nil {
		return err
	}
	a.Config = root

	a.AdditionalApps, err = EnsureKey(root, "AdditionalApps", yaml.SequenceNode, "!!seq")
	if err != nil {
		return err
	}
	a.AdditionalDepots, err = EnsureKey(root, "AdditionalDepots", yaml.SequenceNode, "!!seq")
	if err != nil {
		return err
	}
	a.DecryptionKeys, err = EnsureKey(root, "DecryptionKeys", yaml.MappingNode, "!!map")
	if err != nil {
		return err
	}

	logger.Info("Fetching HubCap manifest", "appid", appid, "step", "2/4")
	luab, err := a.fetchHubcap(appid)
	if err != nil {
		return fmt.Errorf("failed to fetch hubcap: %w", err)
	}

	logger.Info("Fetching game name", "appid", appid, "step", "3/4")
	if a.Name, err = getGameName(appid); err != nil {
		return fmt.Errorf("failed to fetch game name: %w", err)
	}

	logger.Info("Parsing Lua configuration", "step", "4/4")
	if err := a.parseLua(luab, appid); err != nil {
		return fmt.Errorf("failed to parse lua: %w", err)
	}

	AppendIntToSeq(a.AdditionalApps, appid, a.Name)

	logger.Info("Fetched app info", "duration", time.Since(start).Round(time.Millisecond))
	if err := a.SaveConfig(); err != nil {
		logger.Error("Failed to save config", "error", err)
		os.Exit(1)
	}
	logger.Info("✓ Config updated successfully")

	killSteam()

	logger.Info("✓ Done!")
	return nil
}
