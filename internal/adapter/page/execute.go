package page

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"

	"caixin2kindle/internal/model"
)

// Execute 执行一个页面动作（architecture.md §6.2 步骤 3e、§7「中途登出」、§9.1 W9）：
//  1. 等待目标元素可见（上限 ElementVisibleTimeout）；
//  2. 滚动到元素并模拟真实点击；
//  3. 真实点击失败才降级为 DOM el.click()，元素缺失则返回 model.ErrFetch；
//  4. 点击后等待**目标就绪 = DOM 指纹稳定**（有界短上限），失败仅告警，由状态机重读 DOM 决定；
//     ⚠ 不复用 NetworkIdleTimeout：财新页面存在长轮询，真正的网络空闲可能永远不出现（实测 60s 空等）；
//  5. 探测登出，命中返回 model.ErrLogin。
func (c *Client) Execute(ctx context.Context, action model.PageAction) error {
	if action.Kind == model.ActionScrollToBottom {
		if err := c.ScrollToBottom(ctx); err != nil {
			return err
		}
	} else if err := c.performClick(ctx, action); err != nil {
		return err
	}

	if err := c.waitDOMStableWithin(ctx, postClickSettleTimeout); err != nil {
		c.warnf("动作后等待页面稳定未达成（仅警告，由状态机重读 DOM 决定）：%v", err)
	}
	return c.checkLogout(ctx)
}

// performClick 以"真实点击优先、DOM 点击兜底"的方式点击 action.Selector 指向的元素。
func (c *Client) performClick(ctx context.Context, action model.PageAction) error {
	if action.Selector == "" {
		return fmt.Errorf("%w：页面动作缺少 selector", model.ErrFetch)
	}
	if err := c.WaitVisible(ctx, action.Selector); err != nil {
		return err
	}
	clickTimeout := action.MaxWait
	if clickTimeout <= 0 {
		clickTimeout = c.cfg.ElementVisibleTimeout
	}

	realClickErr := c.runAction(ctx, clickTimeout,
		chromedp.ScrollIntoView(action.Selector, chromedp.ByQuery),
		chromedp.Click(action.Selector, chromedp.ByQuery),
	)
	if realClickErr == nil {
		return nil
	}
	c.warnf("[WARN] 真实点击失败，降级为 DOM el.click()：%s：%v", action.Selector, realClickErr)

	var clicked bool
	if err := c.runAction(ctx, clickTimeout, chromedp.Evaluate(domClickJS(action.Selector), &clicked)); err != nil {
		return fmt.Errorf("%w：点击元素失败：%s：%w", model.ErrFetch, action.Selector, err)
	}
	if !clicked {
		return fmt.Errorf("%w：点击目标不存在：%s", model.ErrFetch, action.Selector)
	}
	return nil
}
