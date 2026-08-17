package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 汇总程序运行所需的全部配置。
type Config struct {
	Milvus     MilvusConfig     `yaml:"milvus"`
	Prometheus PrometheusConfig `yaml:"prometheus"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	Alert      AlertConfig      `yaml:"alert"`
	Check      CheckConfig      `yaml:"check"`
	Server     ServerConfig     `yaml:"server"`
	Log        LogConfig        `yaml:"log"`
}

type MilvusConfig struct {
	Address    string        `yaml:"address"`
	Database   string        `yaml:"database"`
	Collection string        `yaml:"collection"`
	Username   string        `yaml:"username"`
	Password   string        `yaml:"password"`
	Token      string        `yaml:"token"`
	Timeout    time.Duration `yaml:"timeout"`
}

type PrometheusConfig struct {
	Enabled bool          `yaml:"enabled"`
	Address string        `yaml:"address"`
	Timeout time.Duration `yaml:"timeout"`
	Job     string        `yaml:"job"`
}

// MetricsConfig 控制内置指标面板的查询范围、超时和可选阈值覆盖。
type MetricsConfig struct {
	DefaultRange string                     `yaml:"default_range"`
	QueryTimeout time.Duration              `yaml:"query_timeout"`
	Thresholds   map[string]MetricThreshold `yaml:"thresholds"`
}

type MetricThreshold struct {
	Warning       float64 `yaml:"warning"`
	Critical      float64 `yaml:"critical"`
	HigherIsWorse bool    `yaml:"higher_is_worse"`
}

// AlertConfig 控制持续加载集合的飞书机器人告警。
type AlertConfig struct {
	Enabled        bool          `yaml:"enabled"`
	FeishuWebhook  string        `yaml:"feishu_webhook"`
	LoadingTimeout time.Duration `yaml:"loading_timeout"`
	RepeatInterval time.Duration `yaml:"repeat_interval"`
	RequestTimeout time.Duration `yaml:"request_timeout"`
}

type CheckConfig struct {
	LoadThreshold int    `yaml:"load_threshold"`
	Output        string `yaml:"output"`
}

type ServerConfig struct {
	Listen          string        `yaml:"listen"`
	Interval        time.Duration `yaml:"interval"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type LogConfig struct {
	Level     string `yaml:"level"`
	Format    string `yaml:"format"`
	AddSource bool   `yaml:"add_source"`
}

func Default() Config {
	return Config{
		Milvus:     MilvusConfig{Address: "localhost:19530", Database: "default", Timeout: 10 * time.Second},
		Prometheus: PrometheusConfig{Enabled: true, Address: "http://localhost:9090", Timeout: 10 * time.Second, Job: "milvus"},
		Metrics:    MetricsConfig{DefaultRange: "1h", QueryTimeout: 15 * time.Second, Thresholds: map[string]MetricThreshold{}},
		Alert:      AlertConfig{LoadingTimeout: 30 * time.Minute, RepeatInterval: time.Hour, RequestTimeout: 10 * time.Second},
		Check:      CheckConfig{LoadThreshold: 100, Output: "table"},
		Server:     ServerConfig{Listen: ":2112", Interval: 30 * time.Second, ShutdownTimeout: 10 * time.Second},
		Log:        LogConfig{Level: "info", Format: "json", AddSource: true},
	}
}

// Load 严格解析 YAML，未知字段会直接报错，避免配置拼写错误被静默忽略。
func Load(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取配置文件 %q: %w", path, err)
	}

	cfg := Default()
	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置文件 %q: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("校验配置文件 %q: %w", path, err)
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	if strings.TrimSpace(cfg.Milvus.Address) == "" {
		return errors.New("milvus.address 不能为空")
	}
	if strings.TrimSpace(cfg.Milvus.Database) == "" {
		return errors.New("milvus.database 不能为空")
	}
	if cfg.Milvus.Database == "*" && cfg.Milvus.Collection != "" {
		return errors.New("milvus.database 为 * 时不能指定 milvus.collection")
	}
	if cfg.Milvus.Timeout <= 0 {
		return errors.New("milvus.timeout 必须大于 0")
	}
	if cfg.Prometheus.Enabled && strings.TrimSpace(cfg.Prometheus.Address) == "" {
		return errors.New("启用 Prometheus 时 prometheus.address 不能为空")
	}
	if cfg.Prometheus.Timeout <= 0 {
		return errors.New("prometheus.timeout 必须大于 0")
	}
	if strings.TrimSpace(cfg.Prometheus.Job) == "" {
		return errors.New("prometheus.job 不能为空")
	}
	switch cfg.Metrics.DefaultRange {
	case "5m", "1h", "6h", "24h":
	default:
		return errors.New("metrics.default_range 只能是 5m、1h、6h 或 24h")
	}
	if cfg.Metrics.QueryTimeout <= 0 {
		return errors.New("metrics.query_timeout 必须大于 0")
	}
	if cfg.Alert.Enabled {
		webhook, err := url.Parse(cfg.Alert.FeishuWebhook)
		if err != nil || webhook.Scheme != "https" || webhook.Host == "" || webhook.Path == "" {
			return errors.New("启用告警时 alert.feishu_webhook 必须是有效的 HTTPS URL")
		}
		if cfg.Alert.LoadingTimeout <= 0 {
			return errors.New("alert.loading_timeout 必须大于 0")
		}
		if cfg.Alert.RepeatInterval <= 0 {
			return errors.New("alert.repeat_interval 必须大于 0")
		}
		if cfg.Alert.RequestTimeout <= 0 {
			return errors.New("alert.request_timeout 必须大于 0")
		}
	}
	if cfg.Check.LoadThreshold < 0 || cfg.Check.LoadThreshold > 100 {
		return errors.New("check.load_threshold 必须在 0 到 100 之间")
	}
	if cfg.Check.Output != "table" && cfg.Check.Output != "json" {
		return errors.New("check.output 只能是 table 或 json")
	}
	if strings.TrimSpace(cfg.Server.Listen) == "" {
		return errors.New("server.listen 不能为空")
	}
	if cfg.Server.Interval <= 0 || cfg.Server.ShutdownTimeout <= 0 {
		return errors.New("server.interval 和 server.shutdown_timeout 必须大于 0")
	}
	if cfg.Log.Format != "json" && cfg.Log.Format != "text" {
		return errors.New("log.format 只能是 json 或 text")
	}
	switch strings.ToLower(cfg.Log.Level) {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("log.level 只能是 debug、info、warn 或 error")
	}
	return nil
}
