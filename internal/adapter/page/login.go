package page

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"

	"caixin2kindle/internal/model"
)

// maxLoginPrompts 限制"提示 → 用户确认 → 再探测"的循环次数。
// 人工确认本身无超时（architecture.md §9.2），但自动循环必须有界（§6.3 审查意见 6）。
const maxLoginPrompts = 3

// LogoutDetected 探测是否已登出：付费墙可见、登录页/登录表单或验证码任一命中即为 true。
// 探测本身失败包装 model.ErrFetch（无法判断登录态）。
func (c *Client) LogoutDetected(ctx context.Context) (bool, error) {
	signal, err := c.detectLoginState(ctx)
	if err != nil {
		return false, err
	}
	return signal != loginSignalNone, nil
}

// detectLoginState 返回登录态信号："" / loginSignalPaywall / loginSignalForm / loginSignalCaptcha。
func (c *Client) detectLoginState(ctx context.Context) (string, error) {
	var signal string
	if err := c.runAction(ctx, c.cfg.ElementVisibleTimeout, chromedp.Evaluate(logoutSignalJS(c.set), &signal)); err != nil {
		return loginSignalNone, fmt.Errorf("%w：探测登录状态失败：%w", model.ErrFetch, err)
	}
	return signal, nil
}

// checkLogout 把登出信号统一转成 model.ErrLogin（退出码 2，§7「中途登出」）。
func (c *Client) checkLogout(ctx context.Context) error {
	logout, err := c.LogoutDetected(ctx)
	if err != nil {
		return err
	}
	if logout {
		return fmt.Errorf("%w：页面出现付费墙、登录页或验证码", model.ErrLogin)
	}
	return nil
}

// EnsureReady 确保登录态就绪：探测 → 已就绪即返回 nil；否则经 port.Prompter 提示人工介入后重探。
// --headless 下无法人工介入，直接返回 model.ErrLogin；prompter 为 nil 同样返回 model.ErrLogin。
// 每次提示前检查 ctx，取消即返回；提示上限 maxLoginPrompts 次。
func (c *Client) EnsureReady(ctx context.Context) error {
	for attempt := 0; attempt < maxLoginPrompts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		signal, err := c.detectLoginState(ctx)
		if err != nil {
			return err
		}
		if signal == loginSignalNone {
			return nil
		}
		if c.cfg.Headless {
			return fmt.Errorf("%w：无头模式无法人工介入，请去掉 --headless 后重试", model.ErrLogin)
		}
		if c.prompter == nil {
			return fmt.Errorf("%w：缺少人工交互提示组件，无法完成登录/验证码", model.ErrLogin)
		}
		if err := c.promptFor(ctx, signal); err != nil {
			return err
		}
	}
	return fmt.Errorf("%w：人工介入后仍未就绪（已提示 %d 次）", model.ErrLogin, maxLoginPrompts)
}

// promptFor 按信号类型提示用户并等待确认；等待人工回车本身不设超时（§9.2）。
func (c *Client) promptFor(ctx context.Context, signal string) error {
	var err error
	if signal == loginSignalCaptcha {
		err = c.prompter.WaitForCaptcha(ctx)
	} else {
		err = c.prompter.WaitForLogin(ctx)
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("%w：等待人工确认失败：%w", model.ErrLogin, err)
	}
	return nil
}
