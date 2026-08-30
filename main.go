//go:build linux

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

var Client = http.Client{Timeout: 10 * time.Second}

type App struct {
	Depotcache    string
	ApiKey        string
	SLSconfigPath string
	Config        *SLSconfig
}

type SLSconfig struct {
	AdditionalApps   []int          `yaml:"AdditionalApps,omitempty"`
	AdditionalDepots []int          `yaml:"AdditionalDepots,omitempty"`
	DecryptionKeys   map[int]string `yaml:"DecryptionKeys,omitempty"`
	// This will make Marshal keep unknown fields (has to be a public field, no _)
	A map[string]any `yaml:",inline"`
}

func main() {
	key := os.Getenv("HUBCAB_KEY")
	if key == "" {
		fmt.Println("HUBCAB_KEY environment variable not set")
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	depotcache := filepath.Join(home, ".steam/steam")
	if f, err := os.Stat(depotcache); err != nil || !f.IsDir() {
		fmt.Println("depotcache not found")
		os.Exit(1)
	}

	slsc := filepath.Join(home, ".config/SLSsteam/config.yaml")
	if f, err := os.Stat(slsc); err != nil || f.IsDir() {
		fmt.Println("config.yaml not found")
		os.Exit(1)
	}

	a := &App{
		Depotcache:    depotcache,
		ApiKey:        key,
		SLSconfigPath: slsc,
	}
	if len(os.Args) < 2 {
		fmt.Println("appid required")
		os.Exit(1)
	} else if len(os.Args) > 2 {
		fmt.Println("too many arguments")
		os.Exit(1)
	}
	app, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("Running appid %d\n", app)
	if err := a.Run(app); err != nil {
		fmt.Println(err)
	}
}

func (a *App) Run(appid int) error {
	cf, err := os.ReadFile(a.SLSconfigPath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(cf, &a.Config); err != nil {
		return err
	}
	if a.Config.DecryptionKeys == nil {
		a.Config.DecryptionKeys = make(map[int]string)
	}

	luab, err := a.fetchHubcap(appid)
	if err != nil {
		return err
	}

	if err := a.parseLua(luab); err != nil {
		return err
	}

	if err := a.getPackageIDs(appid); err != nil {
		return err
	}

	b, err := yaml.Marshal(a.Config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.SLSconfigPath, b, 0644); err != nil {
		return err
	}
	return nil
}

func (a *App) copyManifest(file *zip.File) error {
	zf, err := file.Open()
	if err != nil {
		return err
	}
	defer zf.Close()
	dst := filepath.Join(a.Depotcache, file.Name)
	df, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer df.Close()
	_, err = io.Copy(df, zf)
	return err
}
