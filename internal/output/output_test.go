package output

import (
	"bytes"
	"strings"
	"testing"

	"milvus-check/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestWriteTable(t *testing.T) {
	var buffer bytes.Buffer
	report := domain.CheckReport{Collections: []domain.CollectionReport{{Database: "default", Collection: "book", Exists: true, LoadState: domain.LoadStateLoaded, LoadProgress: 100}}}

	require.NoError(t, Write(&buffer, "table", report))
	require.True(t, strings.Contains(buffer.String(), "book"))
	require.True(t, strings.Contains(buffer.String(), "100%"))
}

func TestWriteJSON(t *testing.T) {
	var buffer bytes.Buffer
	report := domain.CheckReport{Healthy: true}

	require.NoError(t, Write(&buffer, "json", report))
	require.JSONEq(t, `{"healthy":true,"checked_at":"0001-01-01T00:00:00Z","collections":null}`, buffer.String())
}
