package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	a := &App{
		Depotcache:    "/home/cinnamon/.local/share/Steam/depotcache",
		ApiKey:        "smm_03e8bebeef4e96f9f53fd5c2abbfa00d408ead236a5077978ae9b73f0b7e7ce5338d68fa3280304b6804a43cf53b74c7",
		SLSconfigPath: "/home/cinnamon/.config/SLSsteam/config.yaml",
	}
	err := a.Run(1030300)
	if err != nil {
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

	applua, err := a.fetchHubcap(appid)
	if err != nil {
		return err
	}

	if err := a.parseLua(applua); err != nil {
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
