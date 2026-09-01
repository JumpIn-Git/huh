package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

var logger = slog.New(tint.NewTextHandler(os.Stdout, &tint.Options{
	Level:      slog.LevelDebug,
	TimeFormat: time.Kitchen,
}))

func logSection(title string) {
	logger.Info("")
	logger.Info("══════════════════════════════════════")
	logger.Info(title, "section", "")
	logger.Info("══════════════════════════════════════")
}

func logSummary(items ...string) {
	logger.Info("")
	logger.Info("─── Summary ───")
	for _, item := range items {
		logger.Info(item, "summary", "")
	}
}

func logDuration(start time.Time, operation string) {
	logger.Info(operation, "duration", time.Since(start).Round(time.Millisecond))
}
