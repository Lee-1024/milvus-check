package dashboard

import (
	"errors"
	"testing"
	"time"

	"milvus-check/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestStoreStartsUnavailable(t *testing.T) {
	store := NewStore(30 * time.Second)

	snapshot := store.Snapshot()

	require.False(t, snapshot.Ready)
	require.False(t, snapshot.Up)
	require.Equal(t, int64(30), snapshot.RefreshIntervalSeconds)
}

func TestStorePreservesLastSuccessAfterFailure(t *testing.T) {
	store := NewStore(30 * time.Second)
	report := domain.CheckReport{Healthy: true, CheckedAt: time.Unix(100, 0), Collections: []domain.CollectionReport{{Database: "default", Collection: "book"}}}
	store.SetSuccess(report)

	first := store.Snapshot()
	require.True(t, first.Ready)
	require.True(t, first.Up)
	require.Equal(t, "book", first.Report.Collections[0].Collection)

	store.SetFailure(errors.New("connection lost"))
	failed := store.Snapshot()
	require.True(t, failed.Ready)
	require.False(t, failed.Up)
	require.Equal(t, "connection lost", failed.LastError)
	require.Equal(t, "book", failed.Report.Collections[0].Collection)

	failed.Report.Collections[0].Collection = "changed"
	require.Equal(t, "book", store.Snapshot().Report.Collections[0].Collection)
}
