// Package article 负责单篇正文的获取，拆为两半：
//   - 纯解析（extract / accumulate / Plan / verify）：输入为页面 HTML，可在单测中按 fixture 复现（D1/D4）；
//   - 浏览器驱动的状态机（Fetcher.Fetch）：只做编排与步进，不解析 DOM（§6.2）。
//
// Wave 0 只冻结本包的跨包契约（ClickWaiter / Fetcher）；extract.go、accumulate.go、
// navigate.go、verify.go 的签名与实现由 Wave 1 的 A 路自行决定（先写测试，再写实现）。
package article

import (
	"context"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
	"caixin2kindle/internal/selector"
)

// ClickWaiter 提供篇内两次点击之间的随机等待（spec 3.4）；由 adapter/clock 实现。
// 接口定义在消费方包内（§5 判定规则）。
type ClickWaiter interface {
	// BetweenClicks 在两次点击之间等待 1–3 秒随机时长。
	BetweenClicks(ctx context.Context) error
}

// Fetcher 实现 app.ArticleFetcher：把一篇文章的所有分页/展开步骤走完，返回拼接后的正文。
type Fetcher struct {
	Set    selector.Set  // selector 表
	Cfg    config.Config // 点击上限、步数上限与各类等待超时
	Waiter ClickWaiter   // 篇内点击间等待
}

// NewFetcher 构造单篇获取器。
func NewFetcher(set selector.Set, cfg config.Config, waiter ClickWaiter) *Fetcher {
	return &Fetcher{Set: set, Cfg: cfg, Waiter: waiter}
}

// Fetch 按 §6.2 的状态机获取一篇正文：
// 导航 → 循环（完成判定 / Plan 取一个动作 / 执行 / 归档）→ 收尾归档 → 最终强制成功判定。
// 任一判据不通过返回包装 model.ErrFetch 的错误，调用方据此记 fail 并重试。
func (f *Fetcher) Fetch(ctx context.Context, nav port.Navigator, art model.Article) (model.ArticleText, error) {
	panic("TODO(wave1-A): 见 architecture.md §6.2")
}
