# Feishu Loading Alert Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 对持续处于 `loading` 的 Milvus 集合按配置阈值和重复间隔发送飞书 Webhook 告警。

**Architecture:** 新增独立 `internal/alert` 包，将状态跟踪与飞书 HTTP 通知分离。`refreshLoop` 仅在检查成功后把报告交给 Tracker，通知失败记录日志但不影响 Dashboard Store 和 exporter。

**Tech Stack:** Go、`net/http`、YAML、`slog`、`httptest`、Testify。

---

### Task 1: 告警配置

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `config.example.yaml`

- [ ] **Step 1: 编写失败测试**

覆盖默认禁用、启用时 Webhook 必填、只允许 HTTPS、三个时长必须大于零，以及完整 YAML 加载。

- [ ] **Step 2: 运行配置测试确认失败**

Run: `go test ./internal/config`
Expected: FAIL，提示 `Alert` 或告警配置字段尚不存在。

- [ ] **Step 3: 实现配置模型与校验**

新增：

```go
type AlertConfig struct {
    Enabled        bool          `yaml:"enabled"`
    FeishuWebhook  string        `yaml:"feishu_webhook"`
    LoadingTimeout time.Duration `yaml:"loading_timeout"`
    RepeatInterval time.Duration `yaml:"repeat_interval"`
    RequestTimeout time.Duration `yaml:"request_timeout"`
}
```

默认值为禁用、`30m`、`1h`、`10s`。启用时解析 URL 并要求 `https`、主机和路径均有效。

- [ ] **Step 4: 更新示例配置并验证**

Run: `go test ./internal/config`
Expected: PASS。

### Task 2: 集合加载状态跟踪器

**Files:**
- Create: `internal/alert/tracker.go`
- Create: `internal/alert/tracker_test.go`

- [ ] **Step 1: 编写状态机失败测试**

使用可控时钟和 fake notifier 覆盖：首次 loading 仅记录、达到超时告警、成功后等待重复间隔、失败后下次检查重试、loaded/not_load/消失时清理、多个集合独立计时。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/alert -run Tracker`
Expected: FAIL，Tracker 尚未定义。

- [ ] **Step 3: 实现 Tracker**

定义：

```go
type Notifier interface {
    Notify(context.Context, Notification) error
}

type Notification struct {
    MilvusAddress string
    Database      string
    Collection    string
    Progress      int
    LoadingSince  time.Time
    CheckedAt     time.Time
    Duration      time.Duration
    Repeated      bool
}
```

Tracker 使用 `database + "\x00" + collection` 作为内部键，只在通知成功后更新 `lastSentAt`。

- [ ] **Step 4: 运行状态机测试**

Run: `go test ./internal/alert -run Tracker`
Expected: PASS。

### Task 3: 飞书 Webhook 客户端

**Files:**
- Create: `internal/alert/feishu.go`
- Create: `internal/alert/feishu_test.go`

- [ ] **Step 1: 编写 HTTP 协议失败测试**

通过 `httptest.Server` 校验 POST、`application/json`、交互卡片字段、数据库/集合/进度/持续时间；覆盖非 2xx、飞书 `code != 0` 和超时。

- [ ] **Step 2: 运行客户端测试确认失败**

Run: `go test ./internal/alert -run Feishu`
Expected: FAIL，FeishuNotifier 尚未定义。

- [ ] **Step 3: 实现飞书客户端**

使用注入的 `http.Client` 和 Webhook URL。响应先检查 HTTP 状态，再解析：

```go
var response struct {
    Code int    `json:"code"`
    Msg  string `json:"msg"`
}
```

错误信息不得包含完整 Webhook URL。

- [ ] **Step 4: 运行飞书客户端测试**

Run: `go test ./internal/alert -run Feishu`
Expected: PASS。

### Task 4: 刷新循环接入

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`

- [ ] **Step 1: 编写接入失败测试**

提取单次刷新函数，使用 fake checker/store/tracker 验证成功报告触发 Evaluate、检查失败不触发、告警失败不阻止 Store 成功更新。

- [ ] **Step 2: 运行 CLI 测试确认失败**

Run: `go test ./internal/cli`
Expected: FAIL，刷新循环尚未接受告警评估器。

- [ ] **Step 3: 构造并接入告警组件**

`serve` 启动时根据 `cfg.Alert.Enabled` 创建 `FeishuNotifier` 和 `Tracker`；禁用时使用 no-op evaluator。日志只输出启用状态、阈值和重复间隔，不输出 Webhook。

- [ ] **Step 4: 运行 CLI 与告警测试**

Run: `go test ./internal/cli ./internal/alert`
Expected: PASS。

### Task 5: 完整验证

**Files:**
- Verify: all modified files

- [ ] **Step 1: 格式化代码**

Run: `gofmt -w internal/config/config.go internal/config/config_test.go internal/alert/*.go internal/cli/root.go internal/cli/root_test.go`

- [ ] **Step 2: 运行全量测试和静态检查**

Run: `go test ./...`
Expected: PASS。

Run: `go vet ./...`
Expected: PASS。

- [ ] **Step 3: 构建和差异检查**

Run: `go build ./...`
Expected: PASS。

Run: `git diff --check`
Expected: PASS。

- [ ] **Step 4: 使用本地假 Webhook 验证**

用 `httptest` 或本地只读接收端验证超过阈值后只对 `loading` 集合发送，消息不包含密码、Token 和 Webhook。
