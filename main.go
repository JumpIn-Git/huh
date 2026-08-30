package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
	"gopkg.in/yaml.v3"
)

var Client = http.Client{Timeout: 10 * time.Second}

type App struct {
	Depotcache    string
	ApiKey        string
	SLSconfigPath string
}

type SLSconfig struct {
	AdditionalApps   []int          `yaml:"AdditionalApps,omitempty"`
	AdditionalDepots []int          `yaml:"AdditionalDepots,omitempty"`
	DecryptionKeys   map[int]string `yaml:"DecryptionKeys,omitempty"`
	ExtraFields      map[string]any `yaml:",inline"`
}

type LuaEntry struct {
	ID  int
	Key string // can be empty if appid (not depot)
}

func main() {
	a := &App{
		Depotcache:    "/home/cinnamon/.local/share/Steam/depotcache",
		ApiKey:        "smm_03e8bebeef4e96f9f53fd5c2abbfa00d408ead236a5077978ae9b73f0b7e7ce5338d68fa3280304b6804a43cf53b74c7",
		SLSconfigPath: "/home/cinnamon/.config/SLSsteam/config.yaml",
	}
	err := a.setupGame(1030300)
	if err != nil {
		fmt.Println(err)
	}
}

func (a *App) setupGame(appid int) error {
	// TODO most can just be streamed from ram, ask user cuz ie train sim has ton of depots
	tmp, err := os.CreateTemp("", fmt.Sprintf("%d-hubcap-*.zip", appid))
	if err != nil {
		return err
	}
	defer tmp.Close()
	defer os.Remove(tmp.Name())

	url := fmt.Sprintf("https://hubcapmanifest.com/api/v1/manifest/%d", appid)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+a.ApiKey)
	resp, err := Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		return err
	}
	zip, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return err
	}
	defer zip.Close()

	var applua string
	for _, file := range zip.File {
		if strings.HasSuffix(file.Name, ".manifest") {
			if err := a.copyManifest(file); err != nil {
				return err
			}
		} else if strings.HasSuffix(file.Name, ".lua") && applua == "" {
			zf, err := file.Open()
			if err != nil {
				return err
			}
			defer zf.Close()
			b, err := io.ReadAll(zf)
			if err != nil {
				return err
			}
			applua = string(b)
		}
	}
	if applua == "" {
		return fmt.Errorf("no lua file found")
	}

	entries := make([]LuaEntry, 0)
	L := lua.NewState()
	defer L.Close()
	L.SetGlobal("addappid", L.NewFunction(func(l *lua.LState) int {
		id := l.CheckInt(1)
		_ = l.OptInt(2, 0)
		key := l.OptString(3, "")
		entries = append(entries, LuaEntry{ID: id, Key: key})
		return 0
	}))
	mt := L.NewTable()
	L.SetField(mt, "__index", L.NewFunction(func(L *lua.LState) int {
		varName := L.CheckString(2)
		L.Push(L.NewFunction(func(L *lua.LState) int {
			top := L.GetTop()
			args := make([]string, 0, top)
			for i := 1; i <= top; i++ {
				args = append(args, L.Get(i).String())
			}
			fmt.Printf("[CAPTURED] %s call ignored with args: %v\n", varName, args)
			return 0
		}))
		return 1
	}))
	L.SetMetatable(L.GetGlobal("_G"), mt)
	if err := L.DoString(applua); err != nil {
		return err
	}

	cf, err := os.ReadFile(a.SLSconfigPath)
	if err != nil {
		return err
	}
	var config SLSconfig
	if err := yaml.Unmarshal(cf, &config); err != nil {
		return err
	}
	if config.DecryptionKeys == nil {
		config.DecryptionKeys = make(map[int]string)
	}
	for _, entry := range entries {
		if entry.Key != "" {
			config.AdditionalApps = append(config.AdditionalApps, entry.ID)
		} else {
			config.AdditionalDepots = append(config.AdditionalDepots, entry.ID)
			config.DecryptionKeys[entry.ID] = entry.Key
		}
	}

	resp, err = Client.Get(fmt.Sprintf(
		"https://store.steampowered.com/api/appdetails?appids=%d", appid,
	))
	if err != nil {
		return err
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var body steamResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	for _, pkg := range body[fmt.Sprintf("%d", appid)].Data.Packages {
		config.AdditionalApps = append(config.AdditionalApps, pkg)
	}

	b, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.SLSconfigPath, b, 0644); err != nil {
		return err
	}
	return nil
}

type steamResponse map[string]struct {
	// {appid}.data.packages
	Data struct {
		Packages []int `json:"packages"`
	} `json:"data"`
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
