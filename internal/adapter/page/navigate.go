package page

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"caixin2kindle/internal/model"
)

const (
	// defaultNetworkIdleWait 是配置缺失时网络空闲等待的兜底上限（默认值 60s）。
	defaultNetworkIdleWait = 60 * time.Second
	// defaultDOMStableTimeout 与 defaultDOMStablePollInterval 是配置缺失时的 DOM 稳定兜底参数。
	defaultDOMStableTimeout      = 25 * time.Second
	defaultDOMStablePollInterval = 500 * time.Millisecond
	// networkIdleQuietPeriod 是"无在途请求"需持续的静默期，避免把请求间隙当成空闲。
	networkIdleQuietPeriod = 500 * time.Millisecond
	// networkIdlePollInterval 是网络空闲的轮询间隔。
	networkIdlePollInterval = 250 * time.Millisecond
	// secondaryNetworkIdleTimeout 是导航后网络空闲二次等待的上限。
	// 财新页面存在长轮询与常驻连接，真正的"无在途请求"可能永远不出现（2026-09-25 实测每次空等 60s），
	// 故二次等待只给短上限，且失败仅告警：页面就绪的强判据是 DOM 指纹稳定。
	secondaryNetworkIdleTimeout = 8 * time.Second
	// postClickSettleTimeout 是点击后等待页面稳定的上限，同样以 DOM 指纹稳定为准（不复用 60s 网络空闲）。
	postClickSettleTimeout = 8 * time.Second
)

// Navigate 打开 url 并等待页面就绪（architecture.md §6.2 步骤 1）：
// WaitReady(body) → DOM 指纹稳定（主判据）→ 网络空闲（降级二次等待，失败仅告警）→ 登出探测。
func (c *Client) Navigate(ctx context.Context, url string) error {
	browserCtx, err := c.activeBrowserContext()
	if err != nil {
		return err
	}
	navCtx, cancel := context.WithTimeout(browserCtx, c.cfg.NavigateTimeout)
	defer cancel()

	err = chromedp.Run(navCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)
	if err != nil {
		if isTimeoutError(err, navCtx, ctx) {
			return fmt.Errorf("%w：导航超时（上限 %s）：%s：%w", model.ErrFetch, c.cfg.NavigateTimeout, url, err)
		}
		return fmt.Errorf("%w：导航失败：%s：%w", model.ErrFetch, url, err)
	}

	if err := c.WaitDOMStable(ctx); err != nil {
		return err
	}
	if err := c.waitNetworkIdleWithin(ctx, secondaryNetworkIdleTimeout); err != nil {
		c.warnf("网络空闲二次等待未达成（仅警告，DOM 稳定判据已通过）：%v", err)
	}
	return c.checkLogout(ctx)
}

// WaitDOMStable 轮询 DOM 指纹，要求连续 cfg.DOMStableChecks 次不变；整体上限 cfg.DOMStableTimeout。
// 超时只告警并返回 nil：页面照常快照，失败判定交给状态机与 verify.Complete。
func (c *Client) WaitDOMStable(ctx context.Context) error {
	if c.cfg.DOMStableChecks <= 0 {
		return nil
	}
	timeout := c.cfg.DOMStableTimeout
	if timeout <= 0 {
		timeout = defaultDOMStableTimeout
	}
	return c.waitDOMStableWithin(ctx, timeout)
}

// waitDOMStableWithin 是 WaitDOMStable 的带界版本：导航用配置上限，点击后用短上限。
func (c *Client) waitDOMStableWithin(ctx context.Context, timeout time.Duration) error {
	if c.cfg.DOMStableChecks <= 0 {
		return nil
	}
	browserCtx, err := c.activeBrowserContext()
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = defaultDOMStableTimeout
	}
	interval := c.cfg.DOMStablePollInterval
	if interval <= 0 {
		interval = defaultDOMStablePollInterval
	}
	stableCtx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()

	previous := ""
	checks := 0
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stableCtx.Done():
			c.warnf("DOM 稳定等待超时（上限 %s），按当前页面继续快照", timeout)
			return nil
		case <-ticker.C:
			var fingerprint string
			if err := chromedp.Run(stableCtx, chromedp.Evaluate(domFingerprintJS, &fingerprint)); err != nil {
				if stableCtx.Err() != nil {
					continue // 交给下一轮 select 走超时分支
				}
				c.warnf("读取 DOM 指纹失败（按当前页面继续快照）：%v", err)
				return nil
			}
			checks = stableChecksAfter(previous, fingerprint, checks)
			previous = fingerprint
			if checks >= c.cfg.DOMStableChecks {
				return nil
			}
		}
	}
}

// WaitNetworkIdle 等待网络空闲（有界，上限 cfg.NetworkIdleTimeout）：
// 监听 Network 域事件统计在途请求，无在途且静默超过 networkIdleQuietPeriod 即返回。
// 超时返回包装 model.ErrFetch 的错误，由调用方决定是否致命。
func (c *Client) WaitNetworkIdle(ctx context.Context) error {
	timeout := c.cfg.NetworkIdleTimeout
	if timeout <= 0 {
		timeout = defaultNetworkIdleWait
	}
	return c.waitNetworkIdleWithin(ctx, timeout)
}

// waitNetworkIdleWithin 是 WaitNetworkIdle 的带界版本，供导航后的二次等待使用短上限。
func (c *Client) waitNetworkIdleWithin(ctx context.Context, timeout time.Duration) error {
	browserCtx, err := c.activeBrowserContext()
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = defaultNetworkIdleWait
	}
	idleCtx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()

	var mu sync.Mutex
	pending := 0
	lastChange := time.Now()
	chromedp.ListenTarget(idleCtx, func(event any) {
		switch event.(type) {
		case *network.EventRequestWillBeSent:
			mu.Lock()
			pending++
			lastChange = time.Now()
			mu.Unlock()
		case *network.EventLoadingFinished, *network.EventLoadingFailed:
			mu.Lock()
			if pending > 0 {
				pending--
			}
			lastChange = time.Now()
			mu.Unlock()
		}
	})
	if err := chromedp.Run(idleCtx, network.Enable()); err != nil {
		return fmt.Errorf("%w：启用网络监听失败：%w", model.ErrFetch, err)
	}

	ticker := time.NewTicker(networkIdlePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-idleCtx.Done():
			return fmt.Errorf("%w：等待网络空闲超时（上限 %s）", model.ErrFetch, timeout)
		case now := <-ticker.C:
			mu.Lock()
			idle := isNetworkIdle(pending, lastChange, now, networkIdleQuietPeriod)
			mu.Unlock()
			if idle {
				return nil
			}
		}
	}
}

// HTML 返回当前页面的完整 HTML 快照；失败包装 model.ErrFetch。
func (c *Client) HTML(ctx context.Context) (string, error) {
	var snapshot string
	if err := c.runAction(ctx, c.cfg.NetworkIdleTimeout, chromedp.OuterHTML("html", &snapshot, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("%w：读取页面 HTML 失败：%w", model.ErrFetch, err)
	}
	return snapshot, nil
}

// ScrollToBottom 滚动到页面底部，用于期号页懒加载。
func (c *Client) ScrollToBottom(ctx context.Context) error {
	if err := c.runAction(ctx, c.cfg.ElementVisibleTimeout, chromedp.Evaluate(scrollToBottomJS, nil)); err != nil {
		return fmt.Errorf("%w：滚动到页面底部失败：%w", model.ErrFetch, err)
	}
	return nil
}

// WaitVisible 等待 selector 对应元素可见，上限 cfg.ElementVisibleTimeout。
func (c *Client) WaitVisible(ctx context.Context, selector string) error {
	if selector == "" {
		return fmt.Errorf("%w：等待可见的 selector 为空", model.ErrFetch)
	}
	if err := c.runAction(ctx, c.cfg.ElementVisibleTimeout, chromedp.WaitVisible(selector, chromedp.ByQuery)); err != nil {
		return fmt.Errorf("%w：等待元素可见失败：%s：%w", model.ErrFetch, selector, err)
	}
	return nil
}

// runAction 在浏览器上下文上执行 chromedp 动作并施加显式超时；禁止无超时等待。
func (c *Client) runAction(ctx context.Context, timeout time.Duration, actions ...chromedp.Action) error {
	browserCtx, err := c.activeBrowserContext()
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = defaultElementTimeout
	}
	actionCtx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()
	return chromedp.Run(actionCtx, actions...)
}

// isTimeoutError 判断错误是否由 context 超时引起（chromedp 可能只回传底层错误）。
func isTimeoutError(err error, contexts ...context.Context) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	for _, ctx := range contexts {
		if ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return true
		}
	}
	return false
}
