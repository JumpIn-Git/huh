package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lmittmann/tint"
	"github.com/shirou/gopsutil/v3/process"
	"gopkg.in/yaml.v3"
)

var logger = slog.New(tint.NewTextHandler(os.Stdout, &tint.Options{
	Level:      slog.LevelDebug,
	TimeFormat: time.Kitchen,
}))

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

func killSteam() {
	processes, err := process.Processes()
	if err != nil {
		logger.Warn("Failed to list processes", "error", err)
		return
	}

	pid := int32(os.Getpid())
	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			continue
		}
		if name == "steam" && p.Pid != pid {
			logger.Info("Steam found, restarting")
			if err := p.SendSignal(syscall.SIGTERM); err != nil {
				logger.Error("Failed to kill Steam", "error", err)
				os.Exit(1)
			}
			timeout := time.After(5 * time.Second)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()

		PollLoop:
			for {
				select {
				case <-timeout:
					_ = p.Kill() // Fallback to SIGKILL if it hangs
					logger.Info("SIGTERM timed out, forcing kill")
					return
				case <-ticker.C:
					running, _ := p.IsRunning()
					if !running {
						break PollLoop
					}
				}
			}

			logger.Info("✓ Steam terminated")
			time.Sleep(1 * time.Second)
			if err := exec.Command("steam").Start(); err != nil {
				logger.Error("Failed to start Steam", "error", err)
			}
			logger.Info("✓ Steam restarted")
			return
		}
	}
}
