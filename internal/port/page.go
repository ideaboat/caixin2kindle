// Package port 是中立的端口包：只放被两个及以上包消费的接口。
// 单一消费方的接口仍定义在其消费方包内（architecture.md §5 判定规则、§9.1 W8）。
package port

import (
	"context"

	"caixin2kindle/internal/model"
)

// PageSource 是页面读取能力：导航、取 HTML、滚动与各类等待探测。
// 消费方为 app 与 service/*，实现方为 adapter/page。
type PageSource interface {
	// Navigate 打开 url 并等待页面就绪；就绪判据见 architecture.md §6.2 步骤 1。
	Navigate(ctx context.Context, url string) error
	// HTML 返回当前页面的完整 HTML 快照。
	HTML(ctx context.Context) (string, error)
	// ScrollToBottom 滚动到页面底部，用于列表懒加载。
	ScrollToBottom(ctx context.Context) error
	// WaitDOMStable 轮询 DOM 指纹直至连续多次不变；超时告警但不返回错误。
	WaitDOMStable(ctx context.Context) error
	// WaitNetworkIdle 等待网络空闲；已降级为可选二次等待，不再作导航后的唯一判据。
	WaitNetworkIdle(ctx context.Context) error
	// WaitVisible 等待 selector 对应元素可见。
	WaitVisible(ctx context.Context, selector string) error
	// LogoutDetected 探测是否已登出（付费墙可见、登录页或验证码）。
	LogoutDetected(ctx context.Context) (bool, error)
}

// Navigator 在 PageSource 之上增加动作执行能力，保证两个消费方看到的子集一致。
type Navigator interface {
	PageSource
	// Execute 执行一个页面动作（展开全文/翻页/滚动）。
	Execute(ctx context.Context, action model.PageAction) error
}
