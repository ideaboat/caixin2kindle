// Package clock 提供可注入随机源与睡眠函数的拟人化等待实现。
//
// 它结构性地实现 app.Waiter 与 article.ClickWaiter（spec 3.4、architecture.md §7）：
// 随机源与睡眠函数均可注入，使核心逻辑在测试中完全确定、不真实等待。
package clock

import (
	"context"
	"math/rand"
	"time"

	"caixin2kindle/internal/config"
)

// Waiter 按注入的随机源与睡眠函数做拟人化等待：篇间用 Article 区间，篇内点击用 Click 区间。
type Waiter struct {
	ArticleMin time.Duration                                    // 篇间等待下限
	ArticleMax time.Duration                                    // 篇间等待上限
	ClickMin   time.Duration                                    // 篇内点击间等待下限
	ClickMax   time.Duration                                    // 篇内点击间等待上限
	Rand       func() float64                                   // 随机源，取值 [0,1)；nil 时用 math/rand
	Sleep      func(ctx context.Context, d time.Duration) error // 睡眠函数；nil 时用可被 ctx 取消的定时器
}

// NewWaiter 从配置的 DelayMin/Max 与 ClickDelayMin/Max 构造 Waiter；Rand 与 Sleep 保持默认。
func NewWaiter(cfg config.Config) *Waiter {
	return &Waiter{
		ArticleMin: cfg.DelayMin,
		ArticleMax: cfg.DelayMax,
		ClickMin:   cfg.ClickDelayMin,
		ClickMax:   cfg.ClickDelayMax,
	}
}

// BetweenArticles 在相邻两篇文章之间等待 ArticleMin–ArticleMax 的随机时长（默认 3–8s）。
// ctx 取消时返回 ctx.Err()。
func (w *Waiter) BetweenArticles(ctx context.Context) error {
	return w.wait(ctx, w.ArticleMin, w.ArticleMax)
}

// BetweenClicks 在同一篇文章的两次点击之间等待 ClickMin–ClickMax 的随机时长（默认 1–3s）。
// ctx 取消时返回 ctx.Err()。
func (w *Waiter) BetweenClicks(ctx context.Context) error {
	return w.wait(ctx, w.ClickMin, w.ClickMax)
}

// wait 计算区间内的随机时长并交给注入（或默认）的睡眠函数；零/负结果仍调用 Sleep(0)。
func (w *Waiter) wait(ctx context.Context, minimum, maximum time.Duration) error {
	duration := randomDuration(minimum, maximum, w.random())
	return w.resolveSleep()(ctx, duration)
}

// random 返回一次 [0,1) 随机取值；未注入时退回 math/rand。
func (w *Waiter) random() float64 {
	if w.Rand != nil {
		return w.Rand()
	}
	return rand.Float64()
}

// resolveSleep 返回注入的睡眠函数；未注入时退回响应 ctx 取消的定时器实现。
func (w *Waiter) resolveSleep() func(context.Context, time.Duration) error {
	if w.Sleep != nil {
		return w.Sleep
	}
	return sleepContext
}

// randomDuration 在 [minimum, maximum] 内按 sample 取时长：maximum <= minimum 时精确取 minimum；
// 结果为负时收敛到 0（避免负时长）。
func randomDuration(minimum, maximum time.Duration, sample float64) time.Duration {
	if maximum <= minimum {
		return clampNonNegative(minimum)
	}
	return clampNonNegative(minimum + time.Duration(sample*float64(maximum-minimum)))
}

// clampNonNegative 把负时长收敛为 0。
func clampNonNegative(duration time.Duration) time.Duration {
	if duration < 0 {
		return 0
	}
	return duration
}

// sleepContext 等待 d，并在等待期间响应 ctx 取消（禁止裸 time.Sleep，architecture.md §7）。
// ctx 已取消时立即返回 ctx.Err()。
func sleepContext(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
