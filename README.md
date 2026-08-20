# milvus-check

`milvus-check` 用于检查 Milvus 2.6.x 集合加载状态，并将 SDK 获取的集合健康状态暴露为 Prometheus 指标。

## 配置

复制 `config.example.yaml` 为 `config.yaml`，按环境修改 Milvus、Prometheus、检查阈值、服务和日志配置。`config.yaml` 已加入 `.gitignore`，密码与 token 不会写入日志。

当前 Milvus Go client `v2.6.5` 声明需要 Go `1.25.8` 或更高版本。

Docker Compose 环境中，Prometheus 应分别抓取 Milvus 的 `:9091/metrics` 和本程序的 `:2112/metrics`。

## 一次性检查

检查配置中指定的集合：

```powershell
go run ./cmd/milvus-check check --config config.yaml
```

当 `milvus.collection` 为空时，程序检查 `milvus.database` 下的全部集合。将 `milvus.database` 设置为 `"*"` 时，程序会先列出所有数据库，再检查每个数据库下的全部集合；全库模式不能同时指定集合。`check.output` 支持 `table` 和 `json`。

## Exporter

```powershell
go run ./cmd/milvus-check serve --config config.yaml
```

HTTP 端点：

- `/`：只读集合加载监控界面
- `/api/status`：界面使用的最新检查快照 JSON
- `/metrics`：自定义集合健康指标
- `/healthz`：进程存活检查
- `/readyz`：至少成功完成一次 Milvus 检查后返回 200

界面按 `server.interval` 自动刷新，也可以使用右上角刷新按钮立即读取最新快照。浏览器刷新不会直接请求 Milvus，而是读取后台检查循环保存的结果，因此不会随着查看人数增加 Milvus 查询压力。

界面只提供查看能力，不包含集合加载、释放、创建、删除等写操作。历史趋势仍由 Prometheus 或 Grafana 展示。

## 飞书持续加载告警

服务模式可以对持续处于 `loading` 的集合发送飞书机器人告警。`loaded` 和 `not_load` 不发送，也不发送恢复通知。

```yaml
alert:
  enabled: true
  feishu_webhook: "https://open.feishu.cn/open-apis/bot/v2/hook/..."
  loading_timeout: 30m
  repeat_interval: 1h
  request_timeout: 10s
```

集合首次进入 `loading` 后开始计时，达到 `loading_timeout` 时发送首次告警；仍未完成时按 `repeat_interval` 重复。发送失败会在下一次后台检查时重试。告警状态保存在进程内，服务重启后重新计时。Webhook 地址不会输出到日志、页面或 API。

同一次后台检查中触发的多个集合会聚合到一条飞书消息中，避免大量集合同时超时造成消息风暴。

配置完成后可以直接测试飞书通知，不需要连接 Milvus 或创建加载中的集合：

```powershell
.\milvus-check.exe alert-test --config config.yaml
```

命令发送成功时退出码为 0；Webhook 配置、网络或飞书业务响应异常时返回非零退出码。

## 自定义指标

```text
milvus_check_up
milvus_check_collection_exists
milvus_check_collection_loaded
milvus_check_collection_load_progress_percent
milvus_check_collection_entities
milvus_check_collection_index_healthy
milvus_check_collection_check_errors_total
milvus_check_last_success_timestamp_seconds
```

Milvus 原生 metrics 可提供加载耗时、Segment 数和已加载实体数等上下文；集合准确加载状态与进度以 Go SDK 返回结果为准。
