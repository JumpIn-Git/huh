//go:build linux

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/alexflint/go-arg"
	"gopkg.in/yaml.v3"
)

var Client = http.Client{Timeout: 10 * time.Second}

type App struct {
	Depotcache    string
	ApiKey        string
	SLSconfigPath string
	Config        SLSconfig
}

type SLSconfig struct {
	AdditionalApps     []int          `yaml:"AdditionalApps,omitempty"`
	AdditionalDepots   []int          `yaml:"AdditionalDepots,omitempty"`
	AdditionalPackages []int          `yaml:"AdditionalPackages,omitempty"`
	DecryptionKeys     map[int]string `yaml:"DecryptionKeys,omitempty"`
	A                  map[string]any `yaml:",inline"`
}

func main() {
	var args struct {
		Appid   int  `arg:"positional,required"`
		Verbose bool `arg:"-v,--verbose" help:"verbosity level"`
	}
	arg.MustParse(&args)
	if args.Verbose {
		logLevel = slog.LevelDebug
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

	start := time.Now()
	app := &App{
		Depotcache:    depotcache,
		ApiKey:        key,
		SLSconfigPath: slsc,
	}
	if err := app.Run(args.Appid); err != nil {
		logger.Error("Failed to run", "error", err)
		os.Exit(1)
	}

	logger.Info("Fetched app info", "duration", time.Since(start).Round(time.Millisecond))
	if err := app.SaveConfig(); err != nil {
		logger.Error("Failed to save config", "error", err)
		os.Exit(1)
	}
	logger.Info("✓ Config updated successfully")

	logger.Info("Restarting Steam")
	killSteam()

	logger.Info("✓ Done!")
	os.Exit(0)
}

func (a *App) Run(appid int) error {
	logger.Info("Loading existing config", "step", "1/4")
	cf, err := os.ReadFile(a.SLSconfigPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}
	if err := yaml.Unmarshal(cf, &a.Config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	if a.Config.DecryptionKeys == nil {
		a.Config.DecryptionKeys = make(map[int]string)
	}

	logger.Info("Fetching HubCap manifest", "appid", appid, "step", "2/4")
	luab, err := a.fetchHubcap(appid)
	if err != nil {
		return fmt.Errorf("failed to fetch hubcap: %w", err)
	}

	logger.Info("Parsing Lua configuration", "step", "3/4")
	if err := a.parseLua(luab); err != nil {
		return fmt.Errorf("failed to parse lua: %w", err)
	}

	logger.Info("Fetching Steam Store package information", "step", "4/4")
	if err := a.getPackageIDs(appid); err != nil {
		return fmt.Errorf("failed to get packages: %w", err)
	}
	logger.Debug("Found packages on Steam Store", "count", len(a.Config.AdditionalPackages))

	if !slices.Contains(a.Config.AdditionalApps, appid) {
		a.Config.AdditionalApps = append(a.Config.AdditionalApps, appid)
	}
	return nil
}
