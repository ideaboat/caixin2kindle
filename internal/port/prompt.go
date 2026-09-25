package port

import "context"

// Prompter 是人机交互能力：提示用户完成登录或验证码，并等待其确认。
// 消费方为 adapter/page，实现方为 cli/prompt。（人工确认本身无超时，见 §9.2。）
type Prompter interface {
	// WaitForLogin 提示用户在浏览器中完成登录，并等待确认。
	WaitForLogin(ctx context.Context) error
	// WaitForCaptcha 提示用户在浏览器中完成验证码，并等待确认。
	WaitForCaptcha(ctx context.Context) error
}
