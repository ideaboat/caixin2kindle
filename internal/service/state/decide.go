package state

import (
	"fmt"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
)

// NewState 为一期新建空状态，SchemaVersion 取当前支持值。
func NewState(issue model.Issue) model.State {
	return model.State{
		SchemaVersion: model.SchemaVersion,
		IssueID:       issue.ID,
	}
}

// Decide 产出本次需要抓取的篇目：状态为 success、正文文件存在且本地哈希与记录一致者跳过，
// 其余（含 pending / fail / 哈希不符 / 文件缺失）都进待抓列表（spec 4.1、审查意见 7）。
//
// 语义边界：此判定只证明“本地正文文件未被改动”，**不检测财新网页是否更新**——
// 重跑不联网比对；怀疑线上有更新时用 --full 重抓（spec 4.1）。
func Decide(issue model.Issue, current model.State, store port.TextStore) ([]model.Article, error) {
	var pending []model.Article
	for _, article := range issue.Articles {
		entry, found := findEntry(current, article)
		if found && entry.Status == model.StatusSkipped {
			continue // 有意跳过（图片型栏目）：既不抓取，也不算失败
		}
		if !found {
			pending = append(pending, article)
			continue
		}
		usable, err := isUsable(entry, store)
		if err != nil {
			return nil, err
		}
		if !usable {
			pending = append(pending, article)
		}
	}
	return pending, nil
}

// VerifyTexts 做构建前的正文复核：返回缺失、不可读或哈希与状态不符的篇目。
// 调用方据此回抓，绝不用损坏正文生成 EPUB（M2 + 审查意见 12）。
func VerifyTexts(issue model.Issue, current model.State, store port.TextStore) ([]model.Article, error) {
	var bad []model.Article
	for _, article := range issue.Articles {
		entry, found := findEntry(current, article)
		if found && entry.Status == model.StatusSkipped {
			continue // 有意跳过：不参与构建，也不算「正文不可用」
		}
		if !found {
			bad = append(bad, article)
			continue
		}
		usable, err := isUsable(entry, store)
		if err != nil {
			return nil, err
		}
		if !usable {
			bad = append(bad, article)
		}
	}
	return bad, nil
}

// NeedRebuild 判定是否需要重建 EPUB/MOBI：三个条件必须同时成立才可跳过——
// ① 全部文章 success；② artifacts.built_issue_id == issue.ID；③ EPUB 与 MOBI 均存在。
// --no-kindle 不参与该判定：该模式仍须产出 EPUB + MOBI（审查意见 5）。
func NeedRebuild(issue model.Issue, current model.State, epubExists, mobiExists bool) bool {
	if len(issue.Articles) == 0 {
		return true
	}
	if current.Artifacts.BuiltIssueID != issue.ID || !epubExists || !mobiExists {
		return true
	}
	for _, article := range issue.Articles {
		entry, found := findEntry(current, article)
		if !found || (entry.Status != model.StatusSuccess && entry.Status != model.StatusSkipped) {
			return true
		}
	}
	return false
}

// WithArticleState 返回把单篇进度写入后的新状态（保持其余字段不变）。
// 调用方必须在写正文并 fsync **之后**才调用本函数并落盘（审查意见 16）。
func WithArticleState(current model.State, entry model.ArticleState) model.State {
	updated := current
	updated.Articles = append([]model.ArticleState(nil), current.Articles...)
	for index, existing := range updated.Articles {
		if sameEntry(existing, entry) {
			updated.Articles[index] = entry
			return updated
		}
	}
	updated.Articles = append(updated.Articles, entry)
	return updated
}

// MarkBuilt 返回标记“产物已构建”后的新状态：写入 artifacts 与 built_issue_id。
// 只在 EPUB 与 MOBI 都成功产出后调用（§4 artifacts 不变式）。
func MarkBuilt(current model.State, issueID, epubName, mobiName string) model.State {
	updated := current
	updated.Artifacts = model.Artifact{
		EPUB:         epubName,
		MOBI:         mobiName,
		BuiltIssueID: issueID,
	}
	return updated
}

// isUsable 报告一条状态记录是否可直接复用：success、有正文路径与哈希，
// 文件存在且内容哈希与记录一致（spec 4.1）。
func isUsable(entry model.ArticleState, store port.TextStore) (bool, error) {
	if entry.Status != model.StatusSuccess || entry.TextPath == "" || entry.Hash == "" {
		return false, nil
	}

	exists, err := store.Exists(entry.TextPath)
	if err != nil {
		return false, fmt.Errorf("检查正文文件 %s 失败：%w", entry.TextPath, err)
	}
	if !exists {
		return false, nil
	}

	body, err := store.ReadArticleText(entry.TextPath)
	if err != nil {
		return false, fmt.Errorf("读取正文文件 %s 失败：%w", entry.TextPath, err)
	}
	return Hash(body) == entry.Hash, nil
}

// findEntry 在既有状态中按归一化 URL 查找文章记录（URL 缺失时退回目录顺序）。
func findEntry(current model.State, article model.Article) (model.ArticleState, bool) {
	for _, entry := range current.Articles {
		if sameEntry(entry, model.ArticleState{NormalizedURL: article.Normalized, Order: article.Order}) {
			return entry, true
		}
	}
	return model.ArticleState{}, false
}

// sameEntry 判定两条记录是否指向同一篇：优先比归一化 URL，缺失时比目录顺序。
func sameEntry(left, right model.ArticleState) bool {
	if left.NormalizedURL != "" && right.NormalizedURL != "" {
		return left.NormalizedURL == right.NormalizedURL
	}
	return left.Order == right.Order && left.Order != 0
}
