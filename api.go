package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"
)

func (a *App) fetchHubcap(appid int) ([]byte, error) {
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

	logger.Debug("Downloaded manifest zip", "duration", time.Since(start).Round(time.Millisecond))
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	bytesReader := bytes.NewReader(buf)
	zipReader, err := zip.NewReader(bytesReader, int64(len(buf)))
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}

	var luab []byte
	for _, file := range zipReader.File {
		ext := filepath.Ext(file.Name)
		if ext == ".manifest" {
			if err := a.copyManifest(file); err != nil {
				return nil, err
			}
		} else if ext == ".lua" && luab == nil {
			b, err := readZipFile(file)
			if err != nil {
				return nil, fmt.Errorf("failed reading lua file %s: %w", file.Name, err)
			}
			luab = b
			logger.Debug("Found Lua config file", "file", file.Name)
		}
	}
	if luab == nil {
		return nil, fmt.Errorf("no lua file found")
	}
	return luab, nil
}

func readZipFile(file *zip.File) ([]byte, error) {
	zf, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer zf.Close()
	return io.ReadAll(zf)
}
