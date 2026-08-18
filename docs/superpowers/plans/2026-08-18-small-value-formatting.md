# Small Value Formatting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 防止小于显示精度的非零指标被错误展示为零。

**Architecture:** 新增单一自适应数值格式函数，卡片、坐标轴和 tooltip 共同调用；bytes 继续使用容量单位转换。

**Tech Stack:** 原生 JavaScript、Go embed 测试。

---

- [ ] 增加精确零值和科学计数法静态回归测试。
- [ ] 实现自适应小数位数与科学计数法。
- [ ] 卡片、Y 轴和 tooltip 统一调用。
- [ ] 运行全量验证，重新编译并重启预览服务。
