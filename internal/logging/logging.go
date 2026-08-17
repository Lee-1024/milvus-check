package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"milvus-check/internal/config"
)

// New 创建统一的结构化日志器，日志固定写入调用方提供的 writer。
func New(cfg config.LogConfig, writer io.Writer) (*slog.Logger, error) {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("不支持的日志级别 %q", cfg.Level)
	}

	options := &slog.HandlerOptions{Level: level, AddSource: cfg.AddSource}
	if cfg.Format == "text" {
		return slog.New(slog.NewTextHandler(writer, options)), nil
	}
	return slog.New(slog.NewJSONHandler(writer, options)), nil
}
