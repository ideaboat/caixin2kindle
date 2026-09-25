// Package model 定义领域数据与哨兵错误：无行为、无依赖，供各层跨包传递（architecture.md §4）。
package model

// Issue 是《财新周刊》某一期：期号、目录名、来源 URL 与去重后的文章列表。
type Issue struct {
	ID       string    // 期号，如 “财新周刊第1234期”；解析失败时为 slug “2026-cw1224”
	DirName  string    // 已 sanitize 的目录名
	URL      string    // 期号页 URL
	Articles []Article // 已按 Normalized 去重，保持目录顺序
}

// Article 是期号页列表中的一篇：目录顺序、标题、署名与 URL。
type Article struct {
	Order      int    // 目录顺序，从 1 开始
	Title      string // 列表页标题
	Author     string // 列表页署名；仅用于日志，正文署名以文章页为准
	URL        string // 原文 URL
	Normalized string // 去尾 “/” 与 query，用于去重与状态匹配
}
