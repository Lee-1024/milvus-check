# Collection Pagination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为集合加载表增加默认 20 条的客户端分页。

**Architecture:** 保持 `/api/status` 完整快照不变，浏览器维护 `currentPage` 和 `pageSize`，在 `renderRows` 前切片。分页栏由原生按钮和 select 组成，刷新时校正当前页边界。

**Tech Stack:** 嵌入式 HTML、CSS、原生 JavaScript、Go embed 测试。

---

### Task 1: 分页结构与样式

**Files:**
- Modify: `internal/dashboard/assets/index.html`
- Modify: `internal/dashboard/assets/app.css`
- Modify: `internal/dashboard/handler_test.go`

- [ ] 编写静态资源失败测试，要求页面包含分页区域和默认 20 条选项。
- [ ] 运行 `go test ./internal/dashboard` 确认失败。
- [ ] 增加分页信息、每页选择、首页/上一页/页码/下一页/末页控件。
- [ ] 增加桌面和移动端响应式样式并使测试通过。

### Task 2: 客户端分页逻辑

**Files:**
- Modify: `internal/dashboard/assets/app.js`

- [ ] 保存完整集合数组、当前页和每页条数。
- [ ] 计算总页数并将当前页限制在有效范围。
- [ ] 仅把当前页切片传给行渲染函数，同时保留全量汇总计算。
- [ ] 实现每页切换、首页、上一页、页码、下一页和末页事件。
- [ ] 刷新时保持当前页，总页数减少时回到最后有效页。

### Task 3: 验证与构建

**Files:**
- Verify: all modified files

- [ ] 运行 `go test ./...`。
- [ ] 运行 `go vet ./...`。
- [ ] 运行 `git diff --check`。
- [ ] 重新构建根目录 `milvus-check.exe` 并重启预览服务。
