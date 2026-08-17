package promquery

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"milvus-check/internal/domain"

	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Client struct {
	api     v1.API
	timeout time.Duration
	logger  *slog.Logger
}

type Noop struct{}

type Sample struct {
	Labels    map[string]string `json:"labels"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
}

type Point struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

type Series struct {
	Labels map[string]string `json:"labels"`
	Points []Point           `json:"points"`
}

func New(address string, timeout time.Duration, logger *slog.Logger) (*Client, error) {
	client, err := promapi.NewClient(promapi.Config{Address: address})
	if err != nil {
		return nil, fmt.Errorf("创建 Prometheus 客户端: %w", err)
	}
	return &Client{api: v1.NewAPI(client), timeout: timeout, logger: logger}, nil
}

func (Noop) RuntimeMetrics(context.Context, domain.CollectionReport) (domain.RuntimeMetrics, []string) {
	return domain.RuntimeMetrics{}, nil
}

// MetricNames 返回 Prometheus 当前保存的指标名，用于选择兼容的 PromQL 变体。
func (c *Client) MetricNames(ctx context.Context) (map[string]struct{}, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	values, warnings, err := c.api.LabelValues(requestCtx, "__name__", nil, time.Time{}, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("发现 Prometheus 指标: %w", err)
	}
	if len(warnings) > 0 {
		c.logger.Warn("Prometheus 指标发现返回警告", "warning", warnings[0])
	}
	names := make(map[string]struct{}, len(values))
	for _, value := range values {
		names[string(value)] = struct{}{}
	}
	return names, nil
}

func (c *Client) Instant(ctx context.Context, query string, at time.Time) ([]Sample, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	value, warnings, err := c.api.Query(requestCtx, query, at)
	if err != nil {
		return nil, fmt.Errorf("执行 Prometheus 即时查询: %w", err)
	}
	if len(warnings) > 0 {
		c.logger.Warn("Prometheus 即时查询返回警告", "warning", warnings[0])
	}
	vector, ok := value.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("Prometheus 即时查询返回类型 %T，不是 vector", value)
	}
	result := make([]Sample, 0, len(vector))
	for _, item := range vector {
		result = append(result, Sample{Labels: labels(item.Metric), Value: float64(item.Value), Timestamp: item.Timestamp.Time().Unix()})
	}
	return result, nil
}

func (c *Client) Range(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	value, warnings, err := c.api.QueryRange(requestCtx, query, v1.Range{Start: start, End: end, Step: step})
	if err != nil {
		return nil, fmt.Errorf("执行 Prometheus 范围查询: %w", err)
	}
	if len(warnings) > 0 {
		c.logger.Warn("Prometheus 范围查询返回警告", "warning", warnings[0])
	}
	matrix, ok := value.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("Prometheus 范围查询返回类型 %T，不是 matrix", value)
	}
	result := make([]Series, 0, len(matrix))
	for _, stream := range matrix {
		points := make([]Point, 0, len(stream.Values))
		for _, pair := range stream.Values {
			points = append(points, Point{Timestamp: pair.Timestamp.Time().Unix(), Value: float64(pair.Value)})
		}
		result = append(result, Series{Labels: labels(stream.Metric), Points: points})
	}
	return result, nil
}

func labels(metric model.Metric) map[string]string {
	result := make(map[string]string, len(metric))
	for key, value := range metric {
		result[string(key)] = string(value)
	}
	return result
}

// RuntimeMetrics 对不存在的时间序列保持容错，避免 patch 版本的指标差异影响核心检查。
func (c *Client) RuntimeMetrics(ctx context.Context, collection domain.CollectionReport) (domain.RuntimeMetrics, []string) {
	database := strconv.Quote(collection.Database)
	name := strconv.Quote(collection.Collection)
	queries := []struct {
		name  string
		query string
		set   func(*domain.RuntimeMetrics, *float64)
	}{
		{"search_qps", `sum(rate(milvus_proxy_search_vectors_count[5m]))`, func(m *domain.RuntimeMetrics, v *float64) { m.SearchQPS = v }},
		{"query_qps", `sum(rate(milvus_proxy_query_vectors_count[5m]))`, func(m *domain.RuntimeMetrics, v *float64) { m.QueryQPS = v }},
		{"failed_request_ps", `sum(rate(milvus_proxy_function_call_count{status!="success"}[5m]))`, func(m *domain.RuntimeMetrics, v *float64) { m.FailedRequestPS = v }},
		{"loaded_entities", `sum(milvus_rootcoord_entity_num{status="loaded",database_name=` + database + `,collection_name=` + name + `})`, func(m *domain.RuntimeMetrics, v *float64) { m.LoadedEntities = v }},
		{"segment_count", `sum(milvus_querynode_segment_num{collection_id=` + strconv.Quote(strconv.FormatInt(collection.CollectionID, 10)) + `})`, func(m *domain.RuntimeMetrics, v *float64) { m.SegmentCount = v }},
	}

	var metrics domain.RuntimeMetrics
	var warnings []string
	for _, item := range queries {
		value, found, err := c.queryScalar(ctx, item.query)
		if err != nil {
			warning := fmt.Sprintf("Prometheus 查询 %s 失败: %v", item.name, err)
			warnings = append(warnings, warning)
			c.logger.Warn("Prometheus 查询失败", "metric", item.name, "database", collection.Database, "collection", collection.Collection, "error", err)
			continue
		}
		if found {
			item.set(&metrics, &value)
		}
	}
	return metrics, warnings
}

func (c *Client) queryScalar(ctx context.Context, query string) (float64, bool, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	value, warnings, err := c.api.Query(requestCtx, query, time.Now())
	if err != nil {
		return 0, false, err
	}
	if len(warnings) > 0 {
		return 0, false, fmt.Errorf("%s", warnings[0])
	}
	vector, ok := value.(model.Vector)
	if !ok || len(vector) == 0 {
		return 0, false, nil
	}
	return float64(vector[0].Value), true, nil
}
