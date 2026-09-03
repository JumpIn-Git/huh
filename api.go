package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

type storeResponse map[string]struct {
	Data struct {
		Packages []int `json:"packages"`
	} `json:"data"`
}

func (a *App) getPackageIDs(appid int) error {
	url := fmt.Sprintf("https://store.steampowered.com/api/appdetails?appids=%d", appid)

	resp, err := Client.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var body storeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if data, ok := body[fmt.Sprintf("%d", appid)]; ok && data.Data.Packages != nil {
		for _, pkg := range data.Data.Packages {
			if !slices.Contains(a.Config.AdditionalPackages, pkg) {
				a.Config.AdditionalPackages = append(a.Config.AdditionalPackages, pkg)
			}
		}
		return nil
	}
	return fmt.Errorf("invalid Steam response")
}

func (a *App) fetchHubcap(appid int) ([]byte, error) {
	tmp, err := os.CreateTemp("", fmt.Sprintf("%d-hubcap-*.zip", appid))
	if err != nil {
		return nil, err
	}
	defer tmp.Close()
	defer os.Remove(tmp.Name())

	url := fmt.Sprintf("https://hubcapmanifest.com/api/v1/manifest/%d", appid)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+a.ApiKey)
	start := time.Now()
	resp, err := Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	logger.Debug("Downloaded manifest zip", "duration", time.Since(start).Round(time.Millisecond))

	zip, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer zip.Close()

	var luab []byte
	for _, file := range zip.File {
		if strings.HasSuffix(file.Name, ".manifest") {
			if err := a.copyManifest(file); err != nil {
				return nil, err
			}
		} else if strings.HasSuffix(file.Name, ".lua") && luab == nil {
			if err := func() error {
				zf, err := file.Open()
				if err != nil {
					return err
				}
				defer zf.Close()
				b, err := io.ReadAll(zf)
				if err != nil {
					return err
				}
				luab = b
				return nil
			}(); err != nil {
				return nil, err
			}
			logger.Debug("Found Lua config file", "file", file.Name)
		}
	}
	if luab == nil {
		return nil, fmt.Errorf("no lua file found")
	}
	return luab, nil
}
