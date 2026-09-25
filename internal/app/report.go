package app

// Reporter 是 app 的进度与告警出口，由 cli.Reporter 实现（§5 审查意见 5：日志与输出分离）。
// 接口定义在消费方包内，唯一消费方是 app。
type Reporter interface {
	// Progress 报告抓取进度；total 必须是去重后的文章总数（L2 日志断言口径）。
	Progress(done, total int, title string)
	// Warn 报告可继续运行的异常（如 webdriver 未抑制、清理陈旧锁）。
	Warn(msg string)
	// Hint 报告需要用户行动的排查提示。
	Hint(msg string)
}
