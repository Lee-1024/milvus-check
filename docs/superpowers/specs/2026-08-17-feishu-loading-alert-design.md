# 飞书集合持续加载告警设计

## 目标

当集合持续处于 `loading` 状态并超过配置阈值时，通过飞书机器人 Webhook 发送告警。只告警加载中的集合，`loaded` 和 `not_load` 不发送通知，也不发送恢复通知。

## 配置

新增顶层配置：

```yaml
alert:
  enabled: true
  feishu_webhook: "https://open.feishu.cn/open-apis/bot/v2/hook/..."
  loading_timeout: 30m
  repeat_interval: 1h
  request_timeout: 10s
```

约束：

- `enabled` 默认为 `false`。
- 启用时 `feishu_webhook` 必须是 HTTPS URL。
- `loading_timeout`、`repeat_interval`、`request_timeout` 必须大于零。
- Webhook 地址属于敏感配置，不输出到日志、页面或 API。

## 状态跟踪

服务进程内按 `database/collection` 保存：

- 首次观察到 `loading` 的时间。
- 最近一次告警成功时间。
- 最近一次告警尝试时间和结果，仅用于日志。

每次 Milvus 检查成功后执行告警评估：

1. 新进入 `loading`：记录开始时间，不立即告警。
2. 持续时间小于 `loading_timeout`：不告警。
3. 首次达到超时：发送告警。
4. 已成功发送且距离上次成功时间达到 `repeat_interval`：重复发送。
5. Webhook 发送失败：记录错误；下一次检查刷新时重试，不等待完整重复间隔。
6. 集合变为 `loaded`、`not_load`、未知状态或从报告中消失：删除该集合状态。
7. 整次 Milvus 检查失败：不推进或清除已有状态，避免采集故障导致错误重置。

状态不落盘。程序重启后所有 `loading` 集合重新开始计时。

## 告警消息

同一次后台检查中所有到期集合聚合为一条飞书卡片消息。新首次告警和重复告警可以出现在同一批次中，卡片逐项列出数据库、集合、进度、持续时间和告警类型。只有整批发送成功后，批次内集合才更新各自的最近告警时间；失败时整批在下一次检查重试。

飞书使用交互卡片消息，内容包括：

- 标题：`Milvus 集合持续加载告警`
- Milvus 地址
- 数据库名称
- 集合名称
- 当前加载进度
- 持续加载时间
- 首次观察时间
- 当前检查时间

消息不包含 Milvus 密码、Token、Prometheus 地址或飞书 Webhook。

## 组件边界

新增 `internal/alert` 包：

- `Tracker`：维护集合加载状态并决定何时通知。
- `Notifier` 接口：发送结构化告警，便于测试和替换通知实现。
- `FeishuNotifier`：负责构造飞书请求、超时控制、HTTP 状态和飞书业务码校验。

`refreshLoop` 在检查成功并更新 Dashboard Store 后调用 `Tracker.Evaluate`。通知失败只写错误日志，不影响 Store、Exporter 或下一次检查。

## 日志

使用结构化中文日志：

- 首次跟踪加载中的集合：`database`、`collection`、`progress`。
- 集合退出加载状态：记录状态清理。
- 告警发送成功：记录集合、持续时间和是否重复告警。
- 告警失败：记录集合、HTTP 状态或飞书错误码，不记录 Webhook URL。

## 测试

- 配置测试：默认禁用、启用必填、URL 协议和所有时长校验。
- Tracker 测试：未超时、首次超时、重复间隔、失败重试、状态清理、多个集合独立计时。
- 飞书客户端测试：请求 JSON、成功业务码、非 2xx、飞书错误码、超时。
- 刷新循环测试：只在检查成功后评估告警，通知失败不影响快照。
- 完整验证：`go test ./...`、`go vet ./...`、`go build ./...`、`git diff --check`。

## 不包含范围

- 不告警 `not_load` 或 `loaded`。
- 不发送恢复通知。
- 不持久化告警状态。
- 不接入 Alertmanager。
- 不在 Web 页面展示或修改 Webhook。
