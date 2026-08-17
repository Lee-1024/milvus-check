package exporter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"milvus-check/internal/dashboard"
	"milvus-check/internal/domain"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestCollectorExportsCollectionMetrics(t *testing.T) {
	store := dashboard.NewStore(30 * time.Second)
	collector := NewCollector(store)
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(collector))
	store.SetSuccess(domain.CheckReport{CheckedAt: time.Unix(100, 0), Collections: []domain.CollectionReport{{
		Database: "default", Collection: "book", Exists: true, LoadState: domain.LoadStateLoaded, LoadProgress: 100, EntityCount: 20, IndexHealthy: true,
	}}})

	count, err := testutil.GatherAndCount(registry, "milvus_check_collection_loaded", "milvus_check_collection_load_progress_percent")
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, testutil.GatherAndCompare(registry, strings.NewReader("# HELP milvus_check_up Milvus SDK check can run successfully.\n# TYPE milvus_check_up gauge\nmilvus_check_up 1\n"), "milvus_check_up"))

	store.SetFailure(errors.New("unavailable"))
	require.NoError(t, testutil.GatherAndCompare(registry, strings.NewReader("# HELP milvus_check_up Milvus SDK check can run successfully.\n# TYPE milvus_check_up gauge\nmilvus_check_up 0\n"), "milvus_check_up"))
}
