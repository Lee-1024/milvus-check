package checker

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"milvus-check/internal/domain"
)

type MilvusClient interface {
	ListDatabases(ctx context.Context) ([]string, error)
	ListCollections(ctx context.Context, database string) ([]string, error)
	InspectCollection(ctx context.Context, database, collection string) (domain.CollectionReport, error)
}

type PrometheusClient interface {
	RuntimeMetrics(ctx context.Context, collection domain.CollectionReport) (domain.RuntimeMetrics, []string)
}

type Checker struct {
	milvus        MilvusClient
	prometheus    PrometheusClient
	loadThreshold int
	logger        *slog.Logger
}

func New(milvus MilvusClient, prometheus PrometheusClient, loadThreshold int, logger *slog.Logger) *Checker {
	return &Checker{milvus: milvus, prometheus: prometheus, loadThreshold: loadThreshold, logger: logger}
}

func (c *Checker) Check(ctx context.Context, database, collection string) (domain.CheckReport, error) {
	started := time.Now()
	databases := []string{database}
	if database == "*" {
		var err error
		databases, err = c.milvus.ListDatabases(ctx)
		if err != nil {
			return domain.CheckReport{}, err
		}
		sort.Strings(databases)
	}

	report := domain.CheckReport{Healthy: true, CheckedAt: time.Now()}
	for _, databaseName := range databases {
		names := []string{collection}
		if collection == "" {
			var err error
			names, err = c.milvus.ListCollections(ctx, databaseName)
			if err != nil {
				report.Healthy = false
				report.Warnings = append(report.Warnings, fmt.Sprintf("列出数据库 %q 的集合失败: %v", databaseName, err))
				c.logger.Error("列出集合失败", "database", databaseName, "error", err)
				continue
			}
			sort.Strings(names)
		}
		for _, name := range names {
			itemStarted := time.Now()
			item, err := c.milvus.InspectCollection(ctx, databaseName, name)
			if err != nil {
				item.Database = databaseName
				item.Collection = name
				item.Error = err.Error()
				c.logger.Error("集合检查失败", "database", databaseName, "collection", name, "duration", time.Since(itemStarted), "error", err)
			} else {
				metrics, warnings := c.prometheus.RuntimeMetrics(ctx, item)
				item.Metrics = metrics
				item.Warnings = append(item.Warnings, warnings...)
				for _, warning := range item.Warnings {
					c.logger.Warn("集合检查出现降级项", "database", databaseName, "collection", name, "warning", warning)
				}
				c.logger.Info("集合检查完成", "database", databaseName, "collection", name, "load_state", item.LoadState, "load_progress", item.LoadProgress, "duration", time.Since(itemStarted))
			}
			if !collectionHealthy(item, c.loadThreshold) {
				report.Healthy = false
			}
			report.Collections = append(report.Collections, item)
		}
	}
	c.logger.Info("检查任务完成", "database", database, "collection_count", len(report.Collections), "healthy", report.Healthy, "duration", time.Since(started))
	return report, nil
}

func collectionHealthy(report domain.CollectionReport, threshold int) bool {
	return report.Error == "" && report.Exists && report.LoadState == domain.LoadStateLoaded && report.LoadProgress >= int64(threshold) && report.IndexHealthy
}

func FailureError(report domain.CheckReport) error {
	if report.Healthy {
		return nil
	}
	return fmt.Errorf("Milvus 集合健康检查未通过")
}
