# Chart Tooltip Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 区分指标兼容状态文案，并为 uPlot 曲线增加悬停数值提示。

**Architecture:** 指标字典使用独立兼容状态映射。图表通过 uPlot `setCursor` hook 读取当前索引，在图表内部更新自定义 tooltip。

**Tech Stack:** 原生 JavaScript、CSS、uPlot、Go embed 测试。

---

### Task 1: 状态文案

- [ ] 增加静态资源失败测试。
- [ ] 新增目录兼容状态映射。
- [ ] 指标字典和详情兼容信息使用新映射。

### Task 2: 曲线悬停提示

- [ ] 增加 tooltip 结构标识失败测试。
- [ ] 通过 `setCursor` 显示时间和最多六条序列值。
- [ ] 增加边界翻转、溢出限制和隐藏样式。

### Task 3: 验证构建

- [ ] 运行 `go test ./...`、`go vet ./...` 和 `git diff --check`。
- [ ] 重新编译根目录和预览二进制并重启服务。
