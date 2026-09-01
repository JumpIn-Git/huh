//go:build linux

package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
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
	logger.Info("SLSsteam CLI started")
	logger.Info("Checking environment")

	key := os.Getenv("HUBCAB_KEY")
	if key == "" {
		logger.Error("HUBCAB_KEY environment variable not set")
		logger.Info("Please set HUBCAB_KEY to your HubCap API key")
		os.Exit(1)
	}
	logger.Info("✓ HUBCAB_KEY is set")

	home, err := os.UserHomeDir()
	if err != nil {
		logger.Error("Failed to get home directory", "error", err)
		os.Exit(1)
	}

	depotcache := filepath.Join(home, ".steam", "steam")
	if f, err := os.Stat(depotcache); err != nil || !f.IsDir() {
		logger.Error("Steam depotcache not found", "path", depotcache)
		logger.Info("Ensure Steam is installed and has been run at least once")
		os.Exit(1)
	}
	logger.Info("✓ Depot cache found", "path", depotcache)

	slsc := filepath.Join(home, ".config", "SLSsteam", "config.yaml")
	if f, err := os.Stat(slsc); err != nil || f.IsDir() {
		logger.Error("Config file not found", "path", slsc)
		logger.Info("Create the directory and config.yaml if needed: mkdir -p ~/.config/SLSsteam")
		os.Exit(1)
	}
	logger.Info("✓ Config file found", "path", slsc)

	if len(os.Args) < 2 {
		logger.Error("App ID required")
		logger.Info("Usage: SLSsteam <appid>")
		os.Exit(1)
	} else if len(os.Args) > 2 {
		logger.Error("Too many arguments")
		logger.Info("Usage: SLSsteam <appid>")
		os.Exit(1)
	}

	id, err := strconv.Atoi(os.Args[1])
	if err != nil {
		logger.Error("Invalid app ID", "appid", os.Args[1])
		logger.Info("App ID must be a number (e.g., 1493710)")
		os.Exit(1)
	}

	logger.Info("Target app ID", "appid", id)

	startTime := time.Now()
	app := &App{
		Depotcache:    depotcache,
		ApiKey:        key,
		SLSconfigPath: slsc,
	}

	logSection("Fetching HubCap Manifests")
	if err := app.Run(id); err != nil {
		logger.Error("Failed to fetch manifests", "error", err)
		os.Exit(1)
	}

	logDuration(startTime, "manifest fetch")
	logger.Info("✓ Added to config",
		"apps", len(app.Config.AdditionalApps),
		"depots", len(app.Config.AdditionalDepots),
		"packages", len(app.Config.AdditionalPackages))

	if err := app.SaveConfig(); err != nil {
		logger.Error("Failed to save config", "error", err)
		os.Exit(1)
	}
	logger.Info("✓ Config updated successfully")

	logSection("Restarting Steam")
	processes, err := process.Processes()
	if err != nil {
		logger.Warn("Failed to list processes", "error", err)
	} else {
		for _, p := range processes {
			name, err := p.Name()
			if err != nil {
				continue
			}
			if name == "steam" {
				logger.Info("Steam process found, requesting restart")
				if err := p.Kill(); err != nil {
					logger.Error("Failed to kill steam", "error", err)
					os.Exit(1)
				}
				logger.Info("✓ Steam terminated")
				time.Sleep(1 * time.Second)
				if err := exec.Command("steam").Run(); err != nil {
					logger.Error("Failed to restart steam", "error", err)
					os.Exit(1)
				}
				logger.Info("✓ Steam restarted")
				break
			}
		}
	}

	logSummary(
		fmt.Sprintf("App ID: %d", id),
		fmt.Sprintf("Apps configured: %d", len(app.Config.AdditionalApps)),
		fmt.Sprintf("Depots configured: %d", len(app.Config.AdditionalDepots)),
		fmt.Sprintf("Packages configured: %d", len(app.Config.AdditionalPackages)),
	)

	logger.Info("✓ All done! Steam should now have access to the game content.")
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
	logger.Info("Loaded from config",
		"apps", len(a.Config.AdditionalApps),
		"depots", len(a.Config.AdditionalDepots),
		"packages", len(a.Config.AdditionalPackages))

	logger.Info("Fetching HubCap manifest", "appid", appid, "step", "2/4")
	luab, err := a.fetchHubcap(appid)
	if err != nil {
		return fmt.Errorf("failed to fetch hubcap: %w", err)
	}
	logger.Info("Downloaded manifest data", "bytes", len(luab))

	logger.Info("Parsing Lua configuration", "step", "3/4")
	if err := a.parseLua(luab); err != nil {
		return fmt.Errorf("failed to parse lua: %w", err)
	}
	logger.Info("Parsed from Lua", "apps", len(a.Config.AdditionalApps), "depots", len(a.Config.AdditionalDepots))

	logger.Info("Fetching Steam Store package information", "step", "4/4")
	if err := a.getPackageIDs(appid); err != nil {
		return fmt.Errorf("failed to get packages: %w", err)
	}
	logger.Info("Found packages on Steam Store", "count", len(a.Config.AdditionalPackages))

	if !slices.Contains(a.Config.AdditionalApps, appid) {
		a.Config.AdditionalApps = append(a.Config.AdditionalApps, appid)
	}

	return nil
}

func (a *App) SaveConfig() error {
	b, err := yaml.Marshal(a.Config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.SLSconfigPath, b, 0666); err != nil {
		return err
	}
	return nil
}

func (a *App) copyManifest(file *zip.File) error {
	n := strings.TrimSuffix(filepath.Base(file.Name), ".manifest")
	logger.Debug("Processing manifest", "manifest", n)
	zf, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open manifest: %w", err)
	}
	defer zf.Close()
	dst := filepath.Join(a.Depotcache, file.Name)
	df, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if errors.Is(err, os.ErrExist) {
		logger.Debug("↷ Skipping existing manifest", "manifest", n)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to open manifest: %w", err)
	}
	defer df.Close()
	_, err = io.Copy(df, zf)
	if err != nil {
		return fmt.Errorf("failed to copy manifest: %w", err)
	}
	logger.Info("+ Copied manifest", "manifest", n)
	return nil
}
