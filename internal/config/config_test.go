package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadUsesDefaultsAndOverridesValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("milvus:\n  address: milvus:19530\n  database: app\ncheck:\n  output: json\n"), 0o600))

	cfg, err := Load(path)

	require.NoError(t, err)
	require.Equal(t, "milvus:19530", cfg.Milvus.Address)
	require.Equal(t, "app", cfg.Milvus.Database)
	require.Equal(t, "json", cfg.Check.Output)
	require.Equal(t, 100, cfg.Check.LoadThreshold)
	require.Equal(t, "milvus", cfg.Prometheus.Job)
	require.Equal(t, "1h", cfg.Metrics.DefaultRange)
	require.Equal(t, 15*time.Second, cfg.Metrics.QueryTimeout)
	require.False(t, cfg.Alert.Enabled)
	require.Equal(t, 30*time.Minute, cfg.Alert.LoadingTimeout)
	require.Equal(t, time.Hour, cfg.Alert.RepeatInterval)
}

func TestValidateAlertConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		content string
		message string
	}{
		{"missing webhook", "alert:\n  enabled: true\n", "alert.feishu_webhook"},
		{"http webhook", "alert:\n  enabled: true\n  feishu_webhook: http://example.com/hook\n", "HTTPS"},
		{"invalid timeout", "alert:\n  enabled: true\n  feishu_webhook: https://example.com/hook\n  loading_timeout: 0s\n", "alert.loading_timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(test.content), 0o600))
			_, err := Load(path)
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestLoadRejectsUnsupportedMetricsRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("metrics:\n  default_range: 2h\n"), 0o600))

	_, err := Load(path)

	require.ErrorContains(t, err, "metrics.default_range")
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("milvus:\n  adress: typo:19530\n"), 0o600))

	_, err := Load(path)

	require.ErrorContains(t, err, "field adress not found")
}
