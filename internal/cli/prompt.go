package cli

import (
	"context"
	"io"
)

// Prompter 实现 port.Prompter：提示用户完成登录/验证码并等待其确认。
// 运行阶段与提示文案按“准备期→人工期→自动期”切换（§7 审查意见 16）。
// 人工确认本身**不设超时**，这是全项目唯一的例外（§9.2）。
type Prompter struct {
	Out io.Writer // 提示出口；生产为 stderr
	In  io.Reader // 回车输入；生产为 stdin
}

// WaitForLogin 提示“请在浏览器中手动登录”，等待用户按回车后返回。
func (p *Prompter) WaitForLogin(ctx context.Context) error {
	panic("TODO(wave1-E): 见 spec 3.2")
}

// WaitForCaptcha 提示“请在浏览器窗口完成验证后按回车继续”，等待用户确认后返回。
func (p *Prompter) WaitForCaptcha(ctx context.Context) error {
	panic("TODO(wave1-E): 见 spec 3.2-2")
}
