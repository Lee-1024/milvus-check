package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"milvus-check/internal/domain"
	"milvus-check/internal/metricdash"
)

//go:embed assets/*
var assets embed.FS

type StatusResponse struct {
	Ready                  bool                      `json:"ready"`
	Up                     bool                      `json:"up"`
	Healthy                bool                      `json:"healthy"`
	CheckedAt              time.Time                 `json:"checked_at"`
	RefreshIntervalSeconds int64                     `json:"refresh_interval_seconds"`
	LastError              string                    `json:"last_error,omitempty"`
	Warnings               []string                  `json:"warnings,omitempty"`
	Collections            []domain.CollectionReport `json:"collections"`
}

type Handler struct {
	store   *Store
	logger  *slog.Logger
	metrics MetricsService
}

type MetricsService interface {
	Catalog(context.Context) (metricdash.CatalogResponse, error)
	Summary(context.Context, string) (metricdash.SummaryResponse, error)
	Metric(context.Context, string, string) (metricdash.MetricResult, error)
}

func NewHandler(store *Store, logger *slog.Logger, services ...MetricsService) http.Handler {
	handler := &Handler{store: store, logger: logger}
	if len(services) > 0 {
		handler.metrics = services[0]
	}
	return handler
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method != http.MethodGet:
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
	case request.URL.Path == "/":
		h.serveAsset(writer, "assets/index.html")
	case request.URL.Path == "/api/status":
		h.serveStatus(writer)
	case request.URL.Path == "/api/metrics/catalog":
		h.serveMetricsCatalog(writer, request)
	case request.URL.Path == "/api/metrics/summary":
		h.serveMetricsSummary(writer, request)
	case strings.HasPrefix(request.URL.Path, "/api/metrics/series/"):
		h.serveMetricSeries(writer, request)
	case strings.HasPrefix(request.URL.Path, "/assets/"):
		h.serveAsset(writer, strings.TrimPrefix(request.URL.Path, "/"))
	default:
		http.NotFound(writer, request)
	}
}

func (h *Handler) serveMetricsCatalog(writer http.ResponseWriter, request *http.Request) {
	if h.metrics == nil {
		h.writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": "指标服务未配置"})
		return
	}
	response, err := h.metrics.Catalog(request.Context())
	if err != nil {
		h.logger.Error("获取指标目录失败", "error", err)
		h.writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": "Prometheus 指标目录暂不可用"})
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *Handler) serveMetricsSummary(writer http.ResponseWriter, request *http.Request) {
	rangeName := request.URL.Query().Get("range")
	if _, err := metricdash.ParseRange(rangeName); err != nil {
		h.writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if h.metrics == nil {
		h.writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": "指标服务未配置"})
		return
	}
	response, err := h.metrics.Summary(request.Context(), rangeName)
	if err != nil {
		h.writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "指标汇总请求无效"})
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *Handler) serveMetricSeries(writer http.ResponseWriter, request *http.Request) {
	rangeName := request.URL.Query().Get("range")
	if _, err := metricdash.ParseRange(rangeName); err != nil {
		h.writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	id := strings.TrimPrefix(request.URL.Path, "/api/metrics/series/")
	if id == "" || h.metrics == nil {
		h.writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "指标 ID 无效"})
		return
	}
	response, err := h.metrics.Metric(request.Context(), id, rangeName)
	if err != nil {
		h.writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "未知指标 ID"})
		return
	}
	h.writeJSON(writer, http.StatusOK, response)
}

func (h *Handler) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		h.logger.Error("输出 JSON 接口失败", "status", status, "error", err)
	}
}

func (h *Handler) serveAsset(writer http.ResponseWriter, name string) {
	content, err := assets.ReadFile(name)
	if err != nil {
		http.Error(writer, "not found", http.StatusNotFound)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Cache-Control", "public, max-age=300")
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write(content); err != nil {
		h.logger.Warn("写入界面资源失败", "asset", name, "error", err)
	}
}

func (h *Handler) serveStatus(writer http.ResponseWriter) {
	snapshot := h.store.Snapshot()
	response := StatusResponse{
		Ready:                  snapshot.Ready,
		Up:                     snapshot.Up,
		Healthy:                snapshot.Report.Healthy,
		CheckedAt:              snapshot.Report.CheckedAt,
		RefreshIntervalSeconds: snapshot.RefreshIntervalSeconds,
		LastError:              snapshot.LastError,
		Warnings:               snapshot.Report.Warnings,
		Collections:            snapshot.Report.Collections,
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	if !snapshot.Ready {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(writer).Encode(response); err != nil {
		h.logger.Error("输出状态接口失败", "path", "/api/status", "error", err)
	}
}
