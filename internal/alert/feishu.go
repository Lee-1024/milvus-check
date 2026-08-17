package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FeishuNotifier struct {
	webhook string
	client  *http.Client
}

func NewFeishuNotifier(webhook string, timeout time.Duration) *FeishuNotifier {
	return &FeishuNotifier{webhook: webhook, client: &http.Client{Timeout: timeout}}
}

func (f *FeishuNotifier) Notify(ctx context.Context, notification Notification) error {
	title := "Milvus 集合持续加载告警"
	template := "orange"
	if notification.Test {
		title = "Milvus 告警通道测试"
		template = "blue"
	}
	payload := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"template": template,
				"title":    map[string]string{"tag": "plain_text", "content": title},
			},
			"elements": []any{
				map[string]any{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": notificationContent(notification)}},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("编码飞书告警消息: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, f.webhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建飞书告警请求: %w", err)
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := f.client.Do(request)
	if err != nil {
		return fmt.Errorf("发送飞书告警请求失败: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("飞书 Webhook 返回 HTTP %d", response.StatusCode)
	}
	var result struct {
		Code       int    `json:"code"`
		StatusCode int    `json:"StatusCode"`
		Msg        string `json:"msg"`
		StatusMsg  string `json:"StatusMessage"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return fmt.Errorf("解析飞书 Webhook 响应: %w", err)
	}
	code := result.Code
	message := result.Msg
	if result.StatusCode != 0 {
		code = result.StatusCode
		message = result.StatusMsg
	}
	if code != 0 {
		return fmt.Errorf("飞书 Webhook 返回业务错误 code=%d message=%s", code, message)
	}
	return nil
}

func notificationContent(notification Notification) string {
	if notification.Test {
		return fmt.Sprintf("**状态：** 飞书 Webhook 配置与网络连接正常\n**Milvus 地址：** %s\n**测试时间：** %s", notification.MilvusAddress, notification.CheckedAt.Format("2006-01-02 15:04:05"))
	}
	alertType := "首次告警"
	if notification.Repeated {
		alertType = "重复告警"
	}
	return fmt.Sprintf(
		"**告警类型：** %s\n**Milvus 地址：** %s\n**数据库：** %s\n**集合：** %s\n**加载进度：** %d%%\n**持续时间：** %s\n**首次观察：** %s\n**检查时间：** %s",
		alertType, notification.MilvusAddress, notification.Database, notification.Collection, notification.Progress,
		formatDuration(notification.Duration), notification.LoadingSince.Format("2006-01-02 15:04:05"), notification.CheckedAt.Format("2006-01-02 15:04:05"),
	)
}

func formatDuration(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%d 秒", int64(duration/time.Second))
	}
	hours := int64(duration / time.Hour)
	minutes := int64(duration%time.Hour) / int64(time.Minute)
	if hours == 0 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	return fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
}
