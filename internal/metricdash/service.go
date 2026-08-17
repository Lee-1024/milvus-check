package metricdash

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"milvus-check/internal/config"
	"milvus-check/internal/promquery"
)

const (
	StateAvailable   = "available"
	StateZero        = "zero"
	StateNoData      = "no_data"
	StateUnsupported = "unsupported"
	StateError       = "error"
	StateDisabled    = "disabled"
)

type Prometheus interface {
	MetricNames(context.Context) (map[string]struct{}, error)
	Instant(context.Context, string, time.Time) ([]promquery.Sample, error)
	Range(context.Context, string, time.Time, time.Time, time.Duration) ([]promquery.Series, error)
}

type RangeSpec struct {
	Name       string
	Duration   time.Duration
	Step       time.Duration
	RateWindow string
}

type MetricResult struct {
	Definition
	State          string             `json:"state"`
	Current        *float64           `json:"current,omitempty"`
	Level          string             `json:"level"`
	PromQL         string             `json:"promql,omitempty"`
	MissingMetrics []string           `json:"missing_metrics,omitempty"`
	Message        string             `json:"message,omitempty"`
	Series         []promquery.Series `json:"series,omitempty"`
}

type CatalogItem struct {
	Definition
	State          string   `json:"state"`
	PromQL         string   `json:"promql,omitempty"`
	MissingMetrics []string `json:"missing_metrics,omitempty"`
}

type CatalogResponse struct {
	Enabled      bool          `json:"enabled"`
	DefaultRange string        `json:"default_range"`
	Ranges       []string      `json:"ranges"`
	Version      string        `json:"version,omitempty"`
	GitCommit    string        `json:"git_commit,omitempty"`
	BuildTime    string        `json:"build_time,omitempty"`
	Items        []CatalogItem `json:"items"`
}

type SummaryResponse struct {
	Range   string         `json:"range"`
	Metrics []MetricResult `json:"metrics"`
}

type Service struct {
	client       Prometheus
	enabled      bool
	job          string
	timeout      time.Duration
	defaultRange string
	thresholds   map[string]config.MetricThreshold
	logger       *slog.Logger
}

func NewService(client Prometheus, enabled bool, job string, timeout time.Duration, logger *slog.Logger) *Service {
	return &Service{client: client, enabled: enabled, job: job, timeout: timeout, defaultRange: "1h", thresholds: map[string]config.MetricThreshold{}, logger: logger}
}

func (s *Service) Configure(defaultRange string, thresholds map[string]config.MetricThreshold) {
	s.defaultRange = defaultRange
	s.thresholds = thresholds
}

func ParseRange(value string) (RangeSpec, error) {
	specs := map[string]RangeSpec{
		"5m":  {Name: "5m", Duration: 5 * time.Minute, Step: 15 * time.Second, RateWindow: "1m"},
		"1h":  {Name: "1h", Duration: time.Hour, Step: 30 * time.Second, RateWindow: "5m"},
		"6h":  {Name: "6h", Duration: 6 * time.Hour, Step: 2 * time.Minute, RateWindow: "10m"},
		"24h": {Name: "24h", Duration: 24 * time.Hour, Step: 5 * time.Minute, RateWindow: "15m"},
	}
	spec, ok := specs[value]
	if !ok {
		return RangeSpec{}, fmt.Errorf("不支持的时间范围 %q", value)
	}
	return spec, nil
}

func (s *Service) Catalog(ctx context.Context) (CatalogResponse, error) {
	response := CatalogResponse{Enabled: s.enabled, DefaultRange: s.defaultRange, Ranges: []string{"5m", "1h", "6h", "24h"}}
	if !s.enabled {
		for _, definition := range Definitions() {
			response.Items = append(response.Items, CatalogItem{Definition: definition, State: StateDisabled})
		}
		return response, nil
	}
	names, err := s.client.MetricNames(ctx)
	if err != nil {
		return response, err
	}
	for _, definition := range Definitions() {
		variant, missing, ok := SelectVariant(definition, names)
		item := CatalogItem{Definition: definition, State: StateUnsupported, MissingMetrics: missing}
		if ok {
			item.State = StateAvailable
			item.PromQL = RenderPromQL(variant.PromQL, s.job, "5m")
		}
		response.Items = append(response.Items, item)
	}
	build, err := s.client.Instant(ctx, `milvus_build_info{job=`+quote(s.job)+`}`, time.Now())
	if err == nil && len(build) > 0 {
		response.Version = build[0].Labels["version"]
		response.GitCommit = build[0].Labels["git_commit"]
		response.BuildTime = build[0].Labels["built"]
	}
	return response, nil
}

func (s *Service) Summary(ctx context.Context, rangeName string) (SummaryResponse, error) {
	if _, err := ParseRange(rangeName); err != nil {
		return SummaryResponse{}, err
	}
	definitions := Definitions()
	results := make([]MetricResult, len(definitions))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for index, definition := range definitions {
		index, definition := index, definition
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, err := s.metric(ctx, definition, rangeName, false)
			if err != nil {
				result = MetricResult{Definition: definition, State: StateError, Message: "指标查询失败"}
			}
			results[index] = result
		}()
	}
	wg.Wait()
	return SummaryResponse{Range: rangeName, Metrics: results}, nil
}

func (s *Service) Metric(ctx context.Context, id, rangeName string) (MetricResult, error) {
	for _, definition := range Definitions() {
		if definition.ID == id {
			return s.metric(ctx, definition, rangeName, true)
		}
	}
	return MetricResult{}, errors.New("未知指标 ID")
}

func (s *Service) metric(ctx context.Context, definition Definition, rangeName string, includeSeries bool) (MetricResult, error) {
	spec, err := ParseRange(rangeName)
	if err != nil {
		return MetricResult{}, err
	}
	result := MetricResult{Definition: definition, State: StateNoData, Level: "info"}
	if !s.enabled {
		result.State, result.Message = StateDisabled, "Prometheus 查询未启用"
		return result, nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	names, err := s.client.MetricNames(requestCtx)
	if err != nil {
		result.State, result.Message = StateError, "指标发现失败"
		return result, nil
	}
	variant, missing, ok := SelectVariant(definition, names)
	if !ok {
		result.State, result.MissingMetrics = StateUnsupported, missing
		result.Message = "当前 Milvus 指标中缺少所需时间序列"
		return result, nil
	}
	result.PromQL = RenderPromQL(variant.PromQL, s.job, spec.RateWindow)
	now := time.Now()
	samples, err := s.client.Instant(requestCtx, result.PromQL, now)
	if err != nil {
		result.State, result.Message = StateError, "Prometheus 指标查询失败"
		return result, nil
	}
	if len(samples) > 0 && isFinite(samples[0].Value) {
		current := samples[0].Value
		result.Current = &current
		result.State = StateAvailable
		if current == 0 {
			result.State = StateZero
		}
		result.Level = s.level(definition.ID, current)
	}
	if includeSeries {
		series, rangeErr := s.client.Range(requestCtx, result.PromQL, now.Add(-spec.Duration), now, spec.Step)
		if rangeErr != nil {
			result.State, result.Message = StateError, "Prometheus 趋势查询失败"
			return result, nil
		}
		result.Series = normalizeSeries(series)
	}
	return result, nil
}

func (s *Service) level(id string, value float64) string {
	threshold, ok := s.thresholds[id]
	if !ok {
		switch id {
		case "milvus_up":
			if value < 1 {
				return "critical"
			}
		case "request_success_rate":
			if value < 95 {
				return "critical"
			}
			if value < 99 {
				return "warning"
			}
		case "proxy_tt_lag":
			if value > 30000 {
				return "critical"
			}
			if value >= 5000 {
				return "warning"
			}
		}
		return "normal"
	}
	if threshold.HigherIsWorse {
		if value >= threshold.Critical {
			return "critical"
		}
		if value >= threshold.Warning {
			return "warning"
		}
	} else {
		if value <= threshold.Critical {
			return "critical"
		}
		if value <= threshold.Warning {
			return "warning"
		}
	}
	return "normal"
}

func normalizeSeries(series []promquery.Series) []promquery.Series {
	for seriesIndex := range series {
		points := series[seriesIndex].Points[:0]
		for _, point := range series[seriesIndex].Points {
			if isFinite(point.Value) {
				points = append(points, point)
			}
		}
		series[seriesIndex].Points = points
	}
	sort.Slice(series, func(i, j int) bool { return seriesName(series[i].Labels) < seriesName(series[j].Labels) })
	if len(series) > 6 {
		return series[:6]
	}
	return series
}

func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func seriesName(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, ",")
}

func quote(value string) string { return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"` }
