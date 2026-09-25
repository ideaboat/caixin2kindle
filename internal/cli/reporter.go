package cli

import "io"

// Reporter 实现 app.Reporter：进度行、警告与排查提示统一写 stderr 并脱敏（架构 §5、§7 凭据安全）。
// 分母必须是去重后的文章总数（L2 日志断言口径）。
type Reporter struct {
	Log *Logger // 日志出口，测试注入 bytes.Buffer；生产为 stderr
}

// NewReporter 用给定 writer 构造 Reporter，供 main.go 与测试注入日志出口。
func NewReporter(out io.Writer) *Reporter {
	return &Reporter{Log: &Logger{Out: out}}
}

// Progress 输出进度行，格式固定为：第 3/32 篇：<标题>。
func (r *Reporter) Progress(done, total int, title string) {
	r.logger().Infof("第 %d/%d 篇：%s", done, total, title)
}

// Warn 输出可继续运行的异常警告，固定以“警告：”开头。
func (r *Reporter) Warn(msg string) {
	r.logger().Warnf("%s", msg)
}

// Hint 输出需要用户行动的排查提示，固定以“提示：”开头。
func (r *Reporter) Hint(msg string) {
	r.logger().Hintf("%s", msg)
}

// logger 容忍未注入 Log 的 Reporter，保证日志路径不 panic。
func (r *Reporter) logger() *Logger {
	if r == nil || r.Log == nil {
		return &Logger{}
	}
	return r.Log
}
