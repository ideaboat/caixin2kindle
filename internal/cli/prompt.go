package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"caixin2kindle/internal/model"
)

// Prompter 实现 port.Prompter：提示用户完成登录/验证码并等待其确认。
// 人工确认本身不设超时，是全项目唯一例外（架构 §9.2）；但仍尊重 ctx：已取消时立即返回 ErrLogin。
type Prompter struct {
	Out io.Writer // 提示出口；生产为 stderr
	In  io.Reader // 回车输入；生产为 stdin
}

// WaitForLogin 提示用户在浏览器中手动登录，并等待其按回车确认。
func (p *Prompter) WaitForLogin(ctx context.Context) error {
	return p.wait(ctx, "请在浏览器窗口中完成登录，完成后回到终端按回车继续")
}

// WaitForCaptcha 提示用户在浏览器窗口完成验证码，并等待其按回车确认。
func (p *Prompter) WaitForCaptcha(ctx context.Context) error {
	return p.wait(ctx, "请在浏览器窗口完成验证后按回车继续")
}

// wait 输出提示并从 In 读取一行；读到换行或 EOF 视为用户已确认，返回 nil。
// ctx 在读取前检查一次：已取消则返回包装 model.ErrLogin 的错误，避免在已取消的 ctx 上继续等待。
// 读取本身不设超时（唯一例外），只把非 EOF 的读取错误视为无法确认。
func (p *Prompter) wait(ctx context.Context, message string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w：%v", model.ErrLogin, err)
	}
	if p == nil || p.In == nil {
		return fmt.Errorf("%w：无可用的人工确认输入", model.ErrLogin)
	}
	if p.Out != nil {
		writeRedacted(p.Out, "%s", message)
	}
	if _, err := bufio.NewReader(p.In).ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w：读取人工确认失败：%v", model.ErrLogin, err)
	}
	return nil
}
