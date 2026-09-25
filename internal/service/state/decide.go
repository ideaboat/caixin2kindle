package state

import (
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
)

// NewState 为一期新建空状态，SchemaVersion 取当前支持值。
func NewState(issue model.Issue) model.State {
	panic("TODO(wave1-B): 见 architecture.md §4 state.json 结构")
}

// Decide 产出本次需要抓取的篇目：状态为 success、正文文件存在且本地哈希与记录一致者跳过，
// 其余（含 pending / fail / 哈希不符 / 文件缺失）都进待抓列表（spec 4.1、审查意见 7）。
//
// 语义边界：此判定只证明“本地正文文件未被改动”，**不检测财新网页是否更新**。
func Decide(issue model.Issue, current model.State, store port.TextStore) ([]model.Article, error) {
	panic("TODO(wave1-B): 见 spec 4.1")
}

// VerifyTexts 做构建前的正文复核：返回缺失、不可读或哈希与状态不符的篇目。
// 调用方据此回抓，绝不用损坏正文生成 EPUB（M2 + 审查意见 12）。
func VerifyTexts(issue model.Issue, current model.State, store port.TextStore) ([]model.Article, error) {
	panic("TODO(wave1-B): 见 architecture.md §7")
}

// NeedRebuild 判定是否可以跳过 EPUB/MOBI 重建：三个条件必须同时成立——
// ① 全部文章 success；② artifacts.built_issue_id == issue.ID；③ EPUB 与 MOBI 均存在。
// --no-kindle 不参与该判定：该模式仍须产出 EPUB + MOBI（审查意见 5）。
func NeedRebuild(issue model.Issue, current model.State, epubExists, mobiExists bool) bool {
	panic("TODO(wave1-B): 见 architecture.md §7 H6")
}

// WithArticleState 返回把单篇进度写入后的新状态（保持其余字段不变）。
// 调用方必须在写正文并 fsync **之后**才调用本函数并落盘（审查意见 16）。
func WithArticleState(current model.State, entry model.ArticleState) model.State {
	panic("TODO(wave1-B): 见 architecture.md §7 写入顺序")
}

// MarkBuilt 返回标记“产物已构建”后的新状态：写入 artifacts 与 built_issue_id。
// 只在 EPUB 与 MOBI 都成功产出后调用（§4 artifacts 不变式）。
func MarkBuilt(current model.State, issueID, epubName, mobiName string) model.State {
	panic("TODO(wave1-B): 见 architecture.md §4")
}
