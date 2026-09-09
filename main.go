//go:build linux

package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/alexflint/go-arg"
	"gopkg.in/yaml.v3"
)

type App struct {
	Depotcache       string
	ApiKey           string
	SLSconfigPath    string
	Name             string
	Config           *yaml.Node
	AdditionalApps   *yaml.Node // []appid
	AdditionalDepots *yaml.Node // []depotid
	DecryptionKeys   *yaml.Node // map[depotid]key
	ManifestIds      *yaml.Node // map[depotid]gid
}

func main() {
	var args struct {
		Appid         int    `arg:"positional,required"`
		Verbose       bool   `arg:"-v,--verbose" help:"Verbose output"`
		ApiKey        string `arg:"-k,--key,env:HUBCAP_KEY" help:"HubCap API key"`
		SLSconfigPath string `arg:"-c,--config" help:"Custom SLSsteam config (for testing)" placeholder:"SLSC_PATH"`
	}
	p := arg.MustParse(&args)
	if args.ApiKey == "" {
		p.Fail("error: APIKEY is required (or environment variable HUBCAP_KEY)")
	}
	NewLogger(args.Verbose)

	home, err := os.UserHomeDir()
	if err != nil {
		logger.Fatal("Failed to get home directory", "error", err)
	}
	depotcache := filepath.Join(home, ".steam", "steam", "depotcache")
	if f, err := os.Stat(depotcache); err != nil || !f.IsDir() {
		logger.Fatal("Ensure Steam is installed and has been run once")
	}
	if args.SLSconfigPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			logger.Error("Failed to locate user .config directory")
			os.Exit(1)
		}
		args.SLSconfigPath = filepath.Join(configDir, "SLSsteam", "config.yaml")
	}

	(&App{
		Depotcache:    depotcache,
		ApiKey:        args.ApiKey,
		SLSconfigPath: args.SLSconfigPath,
	}).Run(args.Appid)
}

func (a *App) Run(appid int) {
	start := time.Now()
	logger.Info("Loading existing config", "step", "1/4")
	root, err := LoadConfig(a.SLSconfigPath)
	if err != nil {
		logger.Fatal(err)
	}
	a.Config = root

	a.AdditionalApps, err = EnsureKey(root, "AdditionalApps", yaml.SequenceNode, "!!seq")
	if err != nil {
		logger.Fatal(err)
	}
	a.AdditionalDepots, err = EnsureKey(root, "AdditionalDepots", yaml.SequenceNode, "!!seq")
	if err != nil {
		logger.Fatal(err)
	}
	a.DecryptionKeys, err = EnsureKey(root, "DecryptionKeys", yaml.MappingNode, "!!map")
	if err != nil {
		logger.Fatal(err)
	}
	a.ManifestIds, err = EnsureKey(root, "ManifestIds", yaml.MappingNode, "!!map")
	if err != nil {
		logger.Fatal(err)
	}

	logger.Info("Fetching HubCap manifest", "appid", appid, "step", "2/4")
	luab, err := a.fetchHubcap(appid)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Info("Fetching game name", "appid", appid, "step", "3/4")
	if a.Name, err = getGameName(appid); err != nil {
		logger.Fatal(err)
	}

	logger.Info("Executing Lua configuration", "step", "4/4")
	if err := a.parseLua(luab, appid); err != nil {
		logger.Fatal(err)
	}

	AppendIntToSeq(a.AdditionalApps, appid, a.Name)

	logger.Info("Fetched app info", "duration", time.Since(start).Round(time.Millisecond))
	if err := a.SaveConfig(); err != nil {
		logger.Fatal("Failed to save config", "error", err)
	}
	logger.Info("✓ Config updated successfully")

	killSteam()

	logger.Info("✓ Done!")
}
