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
)

// {appid}.data.packages
type storeResponse map[string]struct {
	Data struct {
		Packages []int `json:"packages"`
	} `json:"data"`
}

func (a *App) getPackageIDs(appid int) error {
	resp, err := Client.Get(fmt.Sprintf(
		"https://store.steampowered.com/api/appdetails?appids=%d", appid,
	))
	if err != nil {
		return err
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	var body storeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	for _, pkg := range body[fmt.Sprintf("%d", appid)].Data.Packages {
		if !slices.Contains(a.Config.AdditionalApps, pkg) {
			a.Config.AdditionalApps = append(a.Config.AdditionalApps, pkg)
		}
	}
	return nil
}

func (a *App) fetchHubcap(appid int) (string, error) {
	// TODO most can just be streamed from ram, ask user cuz ie train sim has ton of depots
	tmp, err := os.CreateTemp("", fmt.Sprintf("%d-hubcap-*.zip", appid))
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	defer os.Remove(tmp.Name())

	url := fmt.Sprintf("https://hubcapmanifest.com/api/v1/manifest/%d", appid)
	req, _ := http.NewRequest("GET", url, nil) // We can assume this wont error
	req.Header.Set("Authorization", "Bearer "+a.ApiKey)
	resp, err := Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		return "", err
	}
	zip, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return "", err
	}
	defer zip.Close()

	var applua string
	for _, file := range zip.File {
		if strings.HasSuffix(file.Name, ".manifest") {
			if err := a.copyManifest(file); err != nil {
				return "", err
			}
		} else if strings.HasSuffix(file.Name, ".lua") && applua == "" {
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
				applua = string(b)
				return nil
			}(); err != nil {
				return "", err
			}
		}
	}
	if applua == "" {
		return "", fmt.Errorf("no lua file found")
	}
	return applua, nil
}
