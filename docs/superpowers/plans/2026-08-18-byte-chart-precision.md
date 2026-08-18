# Byte Chart Precision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让 Binlog 和内存曲线的坐标、提示值能区分接近的容量数据。

**Architecture:** bytes 坐标轴根据全部刻度选择共享二进制单位和动态精度；卡片保持简洁，tooltip 使用三位精度。

---

- [ ] 增加 bytes 坐标格式静态回归测试。
- [ ] 实现共享单位与跨度精度计算。
- [ ] 增加 tooltip 精度和坐标轴宽度。
- [ ] 完整验证、编译并重启预览服务。
