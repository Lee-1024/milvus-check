package metricdash

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"milvus-check/internal/promquery"

	"github.com/stretchr/testify/require"
)

type fakePrometheus struct {
	names    map[string]struct{}
	samples  []promquery.Sample
	series   []promquery.Series
	namesErr error
	queryErr error
}

func TestServiceTreatsNaNAsNoData(t *testing.T) {
	service := NewService(fakePrometheus{names: map[string]struct{}{"up": {}}, samples: []promquery.Sample{{Value: math.NaN()}}}, true, "milvus", time.Second, nil)
	metric, err := service.Metric(context.Background(), "milvus_up", "1h")
	require.NoError(t, err)
	require.Equal(t, StateNoData, metric.State)
	require.Nil(t, metric.Current)
}

func (fake fakePrometheus) MetricNames(context.Context) (map[string]struct{}, error) {
	return fake.names, fake.namesErr
}
func (fake fakePrometheus) Instant(context.Context, string, time.Time) ([]promquery.Sample, error) {
	return fake.samples, fake.queryErr
}
func (fake fakePrometheus) Range(context.Context, string, time.Time, time.Time, time.Duration) ([]promquery.Series, error) {
	return fake.series, fake.queryErr
}

func TestServiceDistinguishesZeroNoDataAndUnsupported(t *testing.T) {
	zeroService := NewService(fakePrometheus{names: map[string]struct{}{"up": {}}, samples: []promquery.Sample{{Value: 0}}}, true, "milvus", time.Second, nil)
	zero, err := zeroService.Metric(context.Background(), "milvus_up", "1h")
	require.NoError(t, err)
	require.Equal(t, StateZero, zero.State)

	emptyService := NewService(fakePrometheus{names: map[string]struct{}{"up": {}}}, true, "milvus", time.Second, nil)
	empty, err := emptyService.Metric(context.Background(), "milvus_up", "1h")
	require.NoError(t, err)
	require.Equal(t, StateNoData, empty.State)

	unsupportedService := NewService(fakePrometheus{names: map[string]struct{}{}}, true, "milvus", time.Second, nil)
	unsupported, err := unsupportedService.Metric(context.Background(), "milvus_up", "1h")
	require.NoError(t, err)
	require.Equal(t, StateUnsupported, unsupported.State)
	require.Equal(t, []string{"up"}, unsupported.MissingMetrics)
}

func TestServiceReturnsMetricErrorWithoutFailingRequest(t *testing.T) {
	service := NewService(fakePrometheus{names: map[string]struct{}{"up": {}}, queryErr: errors.New("query failed")}, true, "milvus", time.Second, nil)
	metric, err := service.Metric(context.Background(), "milvus_up", "1h")
	require.NoError(t, err)
	require.Equal(t, StateError, metric.State)
	require.Contains(t, metric.Message, "查询失败")
}

func TestRangeSpecRejectsUnknownRange(t *testing.T) {
	_, err := ParseRange("2h")
	require.Error(t, err)
}
