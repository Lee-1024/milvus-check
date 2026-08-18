package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"milvus-check/internal/domain"
	"milvus-check/internal/metricdash"

	"github.com/stretchr/testify/require"
)

func TestHandlerServesEmbeddedDashboard(t *testing.T) {
	handler := NewHandler(NewStore(30*time.Second), slog.New(slog.NewTextHandler(io.Discard, nil)))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Header().Get("Content-Type"), "text/html")
	require.True(t, strings.Contains(response.Body.String(), "Milvus Check"))
	require.Contains(t, response.Body.String(), `id="collection-pagination"`)
	require.Contains(t, response.Body.String(), `<option value="20" selected>20 条</option>`)

	assetRequest := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	assetResponse := httptest.NewRecorder()
	handler.ServeHTTP(assetResponse, assetRequest)
	require.Equal(t, http.StatusOK, assetResponse.Code)
	require.Contains(t, assetResponse.Header().Get("Content-Type"), "text/css")
	require.Contains(t, assetResponse.Body.String(), "[hidden]")
	require.Contains(t, assetResponse.Body.String(), ".metric-nav { position: sticky; top: 0;")
	require.Contains(t, assetResponse.Body.String(), ".chart-host { min-width: 0; height: 280px;")
	require.NotContains(t, assetResponse.Body.String(), ".chart-host { min-width: 0; height: 230px;")

	scriptRequest := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	scriptResponse := httptest.NewRecorder()
	handler.ServeHTTP(scriptResponse, scriptRequest)
	require.Equal(t, http.StatusOK, scriptResponse.Code)
	require.Contains(t, scriptResponse.Body.String(), "legend: { show: false }")
	require.Contains(t, scriptResponse.Body.String(), `available: "当前版本支持"`)
	require.Contains(t, scriptResponse.Body.String(), `className = "chart-tooltip"`)
	require.Contains(t, scriptResponse.Body.String(), `if (value === 0) return "0"`)
	require.Contains(t, scriptResponse.Body.String(), `value.toExponential(2)`)
}

type stubMetricsService struct{}

func (stubMetricsService) Catalog(context.Context) (metricdash.CatalogResponse, error) {
	return metricdash.CatalogResponse{Enabled: true, DefaultRange: "1h"}, nil
}
func (stubMetricsService) Summary(context.Context, string) (metricdash.SummaryResponse, error) {
	return metricdash.SummaryResponse{Range: "1h"}, nil
}
func (stubMetricsService) Metric(context.Context, string, string) (metricdash.MetricResult, error) {
	return metricdash.MetricResult{Definition: metricdash.Definition{ID: "milvus_up"}, State: metricdash.StateAvailable}, nil
}

func TestHandlerServesMetricsAPIsAndRejectsUnknownRange(t *testing.T) {
	handler := NewHandler(NewStore(30*time.Second), slog.New(slog.NewTextHandler(io.Discard, nil)), stubMetricsService{})

	for _, path := range []string{"/api/metrics/catalog", "/api/metrics/summary?range=1h", "/api/metrics/series/milvus_up?range=1h"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, response.Code, path)
		require.Contains(t, response.Header().Get("Content-Type"), "application/json")
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/metrics/summary?range=2h", nil))
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestHandlerReturnsUnavailableBeforeFirstRefresh(t *testing.T) {
	handler := NewHandler(NewStore(30*time.Second), slog.New(slog.NewTextHandler(io.Discard, nil)))

	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Header().Get("Content-Type"), "application/json")
	var status StatusResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.False(t, status.Ready)
	require.Equal(t, int64(30), status.RefreshIntervalSeconds)
}

func TestHandlerReturnsSuccessfulSnapshot(t *testing.T) {
	store := NewStore(15 * time.Second)
	store.SetSuccess(domain.CheckReport{Healthy: false, CheckedAt: time.Unix(100, 0), Collections: []domain.CollectionReport{{Database: "default", Collection: "book"}}})
	handler := NewHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var status StatusResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.True(t, status.Ready)
	require.True(t, status.Up)
	require.False(t, status.Healthy)
	require.Equal(t, "book", status.Collections[0].Collection)
	require.Equal(t, int64(15), status.RefreshIntervalSeconds)
}
