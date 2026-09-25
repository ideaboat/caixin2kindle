// Package state 是进度判定（纯函数）：待抓列表、构建前复核、重建判定与状态更新。
// 唯一进度依据是 state.json；正文文件只经 port.TextStore 读取（§7 审查意见 8/9）。
//
// Wave 0 只冻结本包的跨包契约（Hash 与 decide.go 中的判定/更新函数）。
package state

// Hash 返回正文纯文本的内容哈希，形如 "sha256:<hex>"。
// 哈希对象必须是**提取后的正文纯文本**，不得对原始 HTML 计算（含动态注入会导致增量失效，spec 4.1）。
func Hash(body string) string {
	panic("TODO(wave1-B): 见 spec 4.1")
}
