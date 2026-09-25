package model

import "strings"

// Status 是单篇文章的抓取状态，落盘于 state.json。
type Status string

const (
	// StatusPending 表示尚未抓取或需要重抓。
	StatusPending Status = "pending"
	// StatusSuccess 表示已抓取且通过完整性判定。
	StatusSuccess Status = "success"
	// StatusFail 表示重试后仍失败。
	StatusFail Status = "fail"
)

// ArticleText 是 extract/accumulate 的输出，也是 EPUBBuilder 的入参（M8）。
type ArticleText struct {
	Order      int      // 目录顺序
	Title      string   // 仅从第 1 页取一次
	Author     string   // 仅从第 1 页取一次
	Paragraphs []string // 段落级去重后的有序正文
}

// Body 把段落以空行连接，供哈希与落盘使用。
func (t ArticleText) Body() string {
	return strings.Join(t.Paragraphs, "\n\n")
}
