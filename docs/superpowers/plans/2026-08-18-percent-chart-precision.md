# Percent Chart Precision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复成功率超过 100% 和百分比坐标刻度全部显示 100 的问题。

**Architecture:** 服务端 PromQL 限制百分比上界，前端根据可见刻度跨度选择小数精度，并为 tooltip 使用更高百分比精度。

---

- [ ] 增加 PromQL 上界回归测试。
- [ ] 增加百分比动态坐标格式静态测试。
- [ ] 修改成功率查询和前端格式器。
- [ ] 完整验证、编译并重启预览服务。
