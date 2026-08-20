package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"milvus-check/internal/alert"
	"milvus-check/internal/checker"
	"milvus-check/internal/config"
	"milvus-check/internal/dashboard"
	"milvus-check/internal/domain"
	"milvus-check/internal/exporter"
	"milvus-check/internal/logging"
	"milvus-check/internal/metricdash"
	"milvus-check/internal/milvus"
	"milvus-check/internal/output"
	"milvus-check/internal/promquery"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

func Execute(ctx context.Context) error {
	var configPath string
	root := &cobra.Command{Use: "milvus-check", Short: "检查 Milvus 集合加载状态"}
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "config.yaml", "YAML 配置文件路径")
	root.AddCommand(newCheckCommand(ctx, &configPath), newServeCommand(ctx, &configPath), newAlertTestCommand(ctx, &configPath))
	return root.ExecuteContext(ctx)
}

func newAlertTestCommand(ctx context.Context, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "alert-test",
		Short: "发送一条飞书测试告警",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, logger, err := loadRuntime(*configPath)
			if err != nil {
				return err
			}
			notifier := alert.NewFeishuNotifier(cfg.Alert.FeishuWebhook, cfg.Alert.RequestTimeout)
			if err := runAlertTest(ctx, cfg, notifier); err != nil {
				logger.Error("飞书测试告警发送失败", "error", err)
				return err
			}
			logger.Info("飞书测试告警发送成功")
			return nil
		},
	}
}

func runAlertTest(ctx context.Context, cfg config.Config, notifier alert.Notifier) error {
	if !cfg.Alert.Enabled {
		return errors.New("发送测试告警前必须设置 alert.enabled: true")
	}
	return notifier.NotifyBatch(ctx, []alert.Notification{{Test: true, MilvusAddress: cfg.Milvus.Address, CheckedAt: time.Now()}})
}

func newCheckCommand(ctx context.Context, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "执行一次集合健康检查",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, logger, err := loadRuntime(*configPath)
			if err != nil {
				return err
			}
			logger.Info("启动一次性检查", "config", *configPath, "database", cfg.Milvus.Database, "collection", cfg.Milvus.Collection)
			service, closeFn, err := buildChecker(ctx, cfg, logger)
			if err != nil {
				return err
			}
			defer closeFn()

			report, err := service.Check(ctx, cfg.Milvus.Database, cfg.Milvus.Collection)
			if err != nil {
				return err
			}
			if err := output.Write(cmd.OutOrStdout(), cfg.Check.Output, report); err != nil {
				return err
			}
			return checker.FailureError(report)
		},
	}
}

func newServeCommand(ctx context.Context, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "启动 Prometheus exporter",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, logger, err := loadRuntime(*configPath)
			if err != nil {
				return err
			}
			service, closeFn, err := buildChecker(ctx, cfg, logger)
			if err != nil {
				return err
			}
			defer closeFn()

			store := dashboard.NewStore(cfg.Server.Interval)
			collector := exporter.NewCollector(store)
			registry := prometheus.NewRegistry()
			if err := registry.Register(collector); err != nil {
				return fmt.Errorf("注册 exporter 指标: %w", err)
			}
			var metricsClient metricdash.Prometheus
			if cfg.Prometheus.Enabled {
				metricsClient, err = promquery.New(cfg.Prometheus.Address, cfg.Prometheus.Timeout, logger)
				if err != nil {
					return err
				}
			}
			metricsService := metricdash.NewService(metricsClient, cfg.Prometheus.Enabled, cfg.Prometheus.Job, cfg.Metrics.QueryTimeout, logger)
			metricsService.Configure(cfg.Metrics.DefaultRange, cfg.Metrics.Thresholds)
			mux := newServeMux(store, registry, logger, metricsService)
			var alertService alertEvaluator = noopAlertEvaluator{}
			if cfg.Alert.Enabled {
				notifier := alert.NewFeishuNotifier(cfg.Alert.FeishuWebhook, cfg.Alert.RequestTimeout)
				alertService = alert.NewTracker(cfg.Milvus.Address, cfg.Alert.LoadingTimeout, cfg.Alert.RepeatInterval, notifier, logger)
				logger.Info("飞书持续加载告警已启用", "loading_timeout", cfg.Alert.LoadingTimeout, "repeat_interval", cfg.Alert.RepeatInterval)
			} else {
				logger.Info("飞书持续加载告警未启用")
			}
			go refreshLoop(ctx, service, store, alertService, cfg, logger)
			server := &http.Server{Addr: cfg.Server.Listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
				defer cancel()
				if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
					logger.Error("HTTP 服务关闭失败", "error", shutdownErr)
				}
			}()

			logger.Info("Exporter 服务启动", "listen", cfg.Server.Listen, "interval", cfg.Server.Interval)
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("启动 HTTP 服务: %w", err)
			}
			logger.Info("Exporter 服务已停止")
			return nil
		},
	}
}

func loadRuntime(path string) (config.Config, *slog.Logger, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, nil, err
	}
	logger, err := logging.New(cfg.Log, os.Stderr)
	if err != nil {
		return config.Config{}, nil, err
	}
	slog.SetDefault(logger)
	logger.Info("配置加载成功", "path", path, "log_level", cfg.Log.Level, "log_format", cfg.Log.Format)
	return cfg, logger, nil
}

func buildChecker(ctx context.Context, cfg config.Config, logger *slog.Logger) (*checker.Checker, func(), error) {
	milvusClient, err := milvus.New(ctx, cfg.Milvus, logger)
	if err != nil {
		return nil, nil, err
	}
	closeFn := func() {
		if err := milvusClient.Close(context.Background()); err != nil {
			logger.Error("关闭 Milvus 客户端失败", "error", err)
		} else {
			logger.Info("Milvus 客户端已关闭")
		}
	}

	var prometheusClient checker.PrometheusClient = promquery.Noop{}
	if cfg.Prometheus.Enabled {
		client, err := promquery.New(cfg.Prometheus.Address, cfg.Prometheus.Timeout, logger)
		if err != nil {
			closeFn()
			return nil, nil, err
		}
		prometheusClient = client
		logger.Info("Prometheus 查询已启用", "address", cfg.Prometheus.Address)
	} else {
		logger.Warn("Prometheus 查询已禁用")
	}
	return checker.New(milvusClient, prometheusClient, cfg.Check.LoadThreshold, logger), closeFn, nil
}

func newServeMux(store *dashboard.Store, registry *prometheus.Registry, logger *slog.Logger, metricsServices ...dashboard.MetricsService) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(writer http.ResponseWriter, _ *http.Request) {
		snapshot := store.Snapshot()
		if !snapshot.Ready || !snapshot.Up {
			http.Error(writer, "not ready", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
	})
	mux.Handle("/", dashboard.NewHandler(store, logger, metricsServices...))
	return mux
}

type alertEvaluator interface {
	Evaluate(context.Context, domain.CheckReport)
}

type noopAlertEvaluator struct{}

func (noopAlertEvaluator) Evaluate(context.Context, domain.CheckReport) {}

func refreshLoop(ctx context.Context, service *checker.Checker, store *dashboard.Store, alertService alertEvaluator, cfg config.Config, logger *slog.Logger) {
	ticker := time.NewTicker(cfg.Server.Interval)
	defer ticker.Stop()
	for {
		report, err := service.Check(ctx, cfg.Milvus.Database, cfg.Milvus.Collection)
		handleRefreshResult(ctx, store, alertService, report, err, logger)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func handleRefreshResult(ctx context.Context, store *dashboard.Store, alertService alertEvaluator, report domain.CheckReport, err error, logger *slog.Logger) {
	if err != nil {
		store.SetFailure(err)
		logger.Error("Exporter 刷新失败", "error", err)
		return
	}
	store.SetSuccess(report)
	logger.Info("Exporter 指标刷新完成", "collection_count", len(report.Collections), "healthy", report.Healthy)
	alertService.Evaluate(ctx, report)
}
