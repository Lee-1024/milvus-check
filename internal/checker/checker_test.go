package checker

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"milvus-check/internal/domain"

	"github.com/stretchr/testify/require"
)

type fakeMilvus struct {
	databases []string
	names     map[string][]string
	reports   map[string]domain.CollectionReport
}

func (f fakeMilvus) ListDatabases(context.Context) ([]string, error) { return f.databases, nil }
func (f fakeMilvus) ListCollections(_ context.Context, database string) ([]string, error) {
	return f.names[database], nil
}
func (f fakeMilvus) InspectCollection(_ context.Context, database, collection string) (domain.CollectionReport, error) {
	report := f.reports[collection]
	report.Database, report.Collection = database, collection
	return report, nil
}

type fakePrometheus struct{}

func (fakePrometheus) RuntimeMetrics(context.Context, domain.CollectionReport) (domain.RuntimeMetrics, []string) {
	value := 3.5
	return domain.RuntimeMetrics{SearchQPS: &value}, nil
}

func TestCheckAllCollections(t *testing.T) {
	client := fakeMilvus{names: map[string][]string{"default": {"b", "a"}}, reports: map[string]domain.CollectionReport{
		"a": {Exists: true, LoadState: domain.LoadStateLoaded, LoadProgress: 100, IndexHealthy: true},
		"b": {Exists: true, LoadState: domain.LoadStateLoading, LoadProgress: 80, IndexHealthy: true},
	}}
	checker := New(client, fakePrometheus{}, 100, slog.New(slog.NewTextHandler(io.Discard, nil)))

	report, err := checker.Check(context.Background(), "default", "")

	require.NoError(t, err)
	require.False(t, report.Healthy)
	require.Equal(t, "a", report.Collections[0].Collection)
	require.NotNil(t, report.Collections[0].Metrics.SearchQPS)
}

func TestCheckAllDatabases(t *testing.T) {
	client := fakeMilvus{
		databases: []string{"db2", "db1"},
		names:     map[string][]string{"db1": {"a"}, "db2": {"b"}},
		reports: map[string]domain.CollectionReport{
			"a": {Exists: true, LoadState: domain.LoadStateLoaded, LoadProgress: 100, IndexHealthy: true},
			"b": {Exists: true, LoadState: domain.LoadStateNotLoad, LoadProgress: 0, IndexHealthy: true},
		},
	}
	checker := New(client, fakePrometheus{}, 100, slog.New(slog.NewTextHandler(io.Discard, nil)))

	report, err := checker.Check(context.Background(), "*", "")

	require.NoError(t, err)
	require.Len(t, report.Collections, 2)
	require.Equal(t, "db1", report.Collections[0].Database)
	require.Equal(t, "db2", report.Collections[1].Database)
	require.False(t, report.Healthy)
}
