package milvus

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"milvus-check/internal/config"
	"milvus-check/internal/domain"

	"github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

type Client struct {
	client  *milvusclient.Client
	timeout time.Duration
	logger  *slog.Logger
	mu      sync.Mutex
}

func New(ctx context.Context, cfg config.MilvusConfig, logger *slog.Logger) (*Client, error) {
	dialCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	database := cfg.Database
	if database == "*" {
		// Milvus 建连时必须使用真实数据库，全库扫描在连接成功后逐库切换。
		database = "default"
	}

	client, err := milvusclient.New(dialCtx, &milvusclient.ClientConfig{
		Address:  cfg.Address,
		Username: cfg.Username,
		Password: cfg.Password,
		APIKey:   cfg.Token,
		DBName:   database,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 Milvus: %w", err)
	}
	logger.Info("Milvus 连接成功", "address", cfg.Address, "database", database, "scan_all_databases", cfg.Database == "*")
	return &Client{client: client, timeout: cfg.Timeout, logger: logger}, nil
}

func (c *Client) Close(ctx context.Context) error {
	closeCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.client.Close(closeCtx); err != nil {
		return fmt.Errorf("关闭 Milvus 连接: %w", err)
	}
	return nil
}

func (c *Client) ListDatabases(ctx context.Context) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	names, err := c.client.ListDatabase(requestCtx, milvusclient.NewListDatabaseOption())
	if err != nil {
		return nil, fmt.Errorf("列出数据库: %w", err)
	}
	return names, nil
}

func (c *Client) ListCollections(ctx context.Context, database string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.client.UseDatabase(requestCtx, milvusclient.NewUseDatabaseOption(database)); err != nil {
		return nil, fmt.Errorf("切换数据库 %q: %w", database, err)
	}
	names, err := c.client.ListCollections(requestCtx, milvusclient.NewListCollectionOption())
	if err != nil {
		return nil, fmt.Errorf("列出集合: %w", err)
	}
	return names, nil
}

// InspectCollection 汇总 SDK 中与单个集合健康状态有关的信息。
func (c *Client) InspectCollection(ctx context.Context, database, collection string) (domain.CollectionReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	report := domain.CollectionReport{Database: database, Collection: collection, IndexHealthy: true}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.client.UseDatabase(requestCtx, milvusclient.NewUseDatabaseOption(database)); err != nil {
		return report, fmt.Errorf("切换数据库 %q: %w", database, err)
	}

	description, err := c.client.DescribeCollection(requestCtx, milvusclient.NewDescribeCollectionOption(collection))
	if err != nil {
		return report, fmt.Errorf("描述集合 %q: %w", collection, err)
	}
	report.Exists = true
	report.CollectionID = description.ID

	loadState, err := c.client.GetLoadState(requestCtx, milvusclient.NewGetLoadStateOption(collection))
	if err != nil {
		return report, fmt.Errorf("获取集合 %q 加载状态: %w", collection, err)
	}
	report.LoadState = convertLoadState(loadState.State)
	report.LoadProgress = loadState.Progress
	if report.LoadState == domain.LoadStateLoaded {
		report.LoadProgress = 100
	}

	stats, err := c.client.GetCollectionStats(requestCtx, milvusclient.NewGetCollectionStatsOption(collection))
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("获取实体数失败: %v", err))
	} else if rowCount, ok := stats["row_count"]; ok {
		value, parseErr := strconv.ParseInt(rowCount, 10, 64)
		if parseErr != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("解析实体数 %q 失败: %v", rowCount, parseErr))
		} else {
			report.EntityCount = value
		}
	}

	partitions, err := c.client.ListPartitions(requestCtx, milvusclient.NewListPartitionOption(collection))
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("获取分区失败: %v", err))
	} else {
		report.PartitionCount = len(partitions)
	}

	indexNames, err := c.client.ListIndexes(requestCtx, milvusclient.NewListIndexOption(collection))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "index not found") {
			report.Warnings = append(report.Warnings, "集合未创建索引")
		} else {
			report.IndexHealthy = false
			report.Warnings = append(report.Warnings, fmt.Sprintf("获取索引列表失败: %v", err))
		}
	} else {
		for _, indexName := range indexNames {
			description, describeErr := c.client.DescribeIndex(requestCtx, milvusclient.NewDescribeIndexOption(collection, indexName))
			if describeErr != nil || description.State != index.IndexState(commonpb.IndexState_Finished) {
				report.IndexHealthy = false
				if describeErr != nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("获取索引 %q 状态失败: %v", indexName, describeErr))
				}
			}
		}
	}
	return report, nil
}

func convertLoadState(state entity.LoadStateCode) domain.LoadState {
	switch state {
	case entity.LoadStateLoaded:
		return domain.LoadStateLoaded
	case entity.LoadStateLoading:
		return domain.LoadStateLoading
	case entity.LoadStateNotLoad, entity.LoadStateUnloading:
		return domain.LoadStateNotLoad
	default:
		return domain.LoadStateUnknown
	}
}
