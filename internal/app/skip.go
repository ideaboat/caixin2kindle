package app

import (
	"context"
	"fmt"
	"strings"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/service/article"
	"caixin2kindle/internal/service/state"
)

// markSkipped 把「有意跳过」的篇目（图片型栏目等）记为 skipped 状态，使增量判定与构建复核都不再纠缠它们：
// 既不抓取、也不算失败、也不进 EPUB。返回更新后的状态、跳过篇目与原因说明；
// 状态没有实际变化时不重复落盘，但每次运行都会重新输出一次说明。
func (a *App) markSkipped(
	ctx context.Context,
	issue model.Issue,
	issueDir string,
	current model.State,
) (model.State, []model.Article, string, error) {
	var (
		skipped []model.Article
		note    string
		updated = current
		changed bool
	)

	for _, candidate := range issue.Articles {
		reason := article.SkipReason(candidate.Title)
		if reason == "" {
			continue
		}
		note = reason
		skipped = append(skipped, candidate)

		existing, found := stateEntry(updated, candidate)
		if found && existing.Status == model.StatusSkipped {
			continue
		}
		updated = state.WithArticleState(updated, model.ArticleState{
			Order:         candidate.Order,
			URL:           candidate.URL,
			NormalizedURL: candidate.Normalized,
			Title:         candidate.Title,
			Status:        model.StatusSkipped,
		})
		changed = true
	}

	if len(skipped) == 0 {
		return current, nil, "", nil
	}
	a.deps.Reporter.Hint(fmt.Sprintf("已跳过 %d 篇图片型栏目（%s）：%s",
		len(skipped), note, strings.Join(articleTitles(skipped), "、")))

	if !changed {
		return current, skipped, note, nil
	}
	if err := a.deps.States.Save(ctx, issueDir, updated); err != nil {
		return current, nil, "", fetchError("保存跳过状态", err)
	}
	return updated, skipped, note, nil
}
