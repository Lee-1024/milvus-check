package cli

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"milvus-check/internal/alert"
	"milvus-check/internal/config"
	"milvus-check/internal/dashboard"
	"milvus-check/internal/domain"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestServeMuxReadinessFollowsDashboardStore(t *testing.T) {
	store := dashboard.NewStore(30 * time.Second)
	registry := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := newServeMux(store, registry, logger)

	indexResponse := httptest.NewRecorder()
	mux.ServeHTTP(indexResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, indexResponse.Code)

	notReady := httptest.NewRecorder()
	mux.ServeHTTP(notReady, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusServiceUnavailable, notReady.Code)

	store.SetSuccess(domain.CheckReport{Healthy: true, CheckedAt: time.Now()})
	ready := httptest.NewRecorder()
	mux.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusOK, ready.Code)

	store.SetFailure(io.EOF)
	failed := httptest.NewRecorder()
	mux.ServeHTTP(failed, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.Equal(t, http.StatusServiceUnavailable, failed.Code)
}

type fakeAlertEvaluator struct {
	reports []domain.CheckReport
}

type fakeAlertNotifier struct {
	notifications []alert.Notification
	err           error
}

func (f *fakeAlertNotifier) Notify(_ context.Context, notification alert.Notification) error {
	f.notifications = append(f.notifications, notification)
	return f.err
}

func TestRunAlertTestSendsTestNotification(t *testing.T) {
	notifier := &fakeAlertNotifier{}
	cfg := config.Default()
	cfg.Alert.Enabled = true
	cfg.Milvus.Address = "milvus:19530"

	err := runAlertTest(context.Background(), cfg, notifier)

	require.NoError(t, err)
	require.Len(t, notifier.notifications, 1)
	require.True(t, notifier.notifications[0].Test)
	require.Equal(t, "milvus:19530", notifier.notifications[0].MilvusAddress)
}

func TestRunAlertTestRequiresEnabledAlert(t *testing.T) {
	err := runAlertTest(context.Background(), config.Default(), &fakeAlertNotifier{})
	require.ErrorContains(t, err, "alert.enabled")
}

func (f *fakeAlertEvaluator) Evaluate(_ context.Context, report domain.CheckReport) {
	f.reports = append(f.reports, report)
}

func TestHandleRefreshResultEvaluatesAlertsOnlyAfterSuccessfulCheck(t *testing.T) {
	store := dashboard.NewStore(time.Second)
	evaluator := &fakeAlertEvaluator{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	report := domain.CheckReport{Healthy: false, CheckedAt: time.Now(), Collections: []domain.CollectionReport{{Database: "default", Collection: "books", LoadState: domain.LoadStateLoading}}}

	handleRefreshResult(context.Background(), store, evaluator, report, nil, logger)
	require.Len(t, evaluator.reports, 1)
	require.True(t, store.Snapshot().Up)

	handleRefreshResult(context.Background(), store, evaluator, domain.CheckReport{}, io.EOF, logger)
	require.Len(t, evaluator.reports, 1)
	require.False(t, store.Snapshot().Up)
}
