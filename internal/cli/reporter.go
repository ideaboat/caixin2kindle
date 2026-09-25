package cli

import "io"

// Reporter 实现 app.Reporter：进度行、告警与排查提示统一写 stderr 并脱敏（§5 审查意见 5、§7 凭据安全）。
// 分母必须是去重后的文章总数（L2 日志断言口径）。
type Reporter struct {
	Out io.Writer // 日志出口，测试注入 buffer；生产为 stderr
}

// Progress 输出进度行，格式：第 3/32 篇：<标题>。
func (r *Reporter) Progress(done, total int, title string) {
	panic("TODO(wave1-E): 见 spec 4.5")
}

// Warn 输出可继续运行的异常告警。
func (r *Reporter) Warn(msg string) {
	panic("TODO(wave1-E): 见 architecture.md §7")
}

// Hint 输出需要用户行动的排查提示。
func (r *Reporter) Hint(msg string) {
	panic("TODO(wave1-E): 见 spec 4.3")
}
