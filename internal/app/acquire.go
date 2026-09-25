package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/service/issue"
	"caixin2kindle/internal/service/state"
)

// acquire 抓取待抓列表（§6.3）：篇间等待、单篇重试、登录恢复与连败熔断。
// 返回更新后的状态；任一篇最终失败时，其 fail 状态已落盘，便于下次重跑补抓。
func (a *App) acquire(
	ctx context.Context,
	issue model.Issue,
	issueDir string,
	current model.State,
	targets []model.Article,
	cfg config.Config,
) (model.State, error) {
	consecutiveFailures := 0

	for index, target := range targets {
		if index > 0 {
			if err := a.deps.Waiter.BetweenArticles(ctx); err != nil {
				return current, fetchError("篇间等待", err)
			}
		}

		text, err := a.fetchArticle(ctx, target, cfg)
		if err != nil {
			if errors.Is(err, model.ErrLogin) {
				return current, err
			}
			consecutiveFailures++
			current, err = a.recordFailure(ctx, issueDir, current, target, err)
			if err != nil {
				return current, err
			}
			if consecutiveFailures >= cfg.ConsecutiveFailureLimit {
				a.deps.Reporter.Hint("连续多篇失败，请检查：登录态是否失效 / 网络是否正常 / 页面结构是否变更")
				return current, fmt.Errorf("%w：连续 %d 篇文章失败：%w", model.ErrFetch, consecutiveFailures, err)
			}
			continue
		}

		consecutiveFailures = 0
		current, err = a.saveArticle(ctx, issue, issueDir, current, text)
		if err != nil {
			return current, err
		}
		a.deps.Reporter.Progress(successCount(current), len(issue.Articles), text.Title)
	}
	return current, nil
}

// fetchArticle 抓取单篇：最多 ArticleRetryLimit 次尝试，固定退避；
// ErrLogin 不计入尝试次数，转为登录恢复（上限 LoginRecoveryLimit，用尽即返回 ErrLogin）。
func (a *App) fetchArticle(ctx context.Context, target model.Article, cfg config.Config) (model.ArticleText, error) {
	var lastErr error
	recoveries := 0

	for attempt := 0; attempt < cfg.ArticleRetryLimit; {
		text, err := a.deps.Fetcher.Fetch(ctx, a.deps.Nav, target)
		if err == nil {
			return text, nil
		}
		lastErr = err

		if errors.Is(err, model.ErrLogin) {
			if recoveries >= cfg.LoginRecoveryLimit {
				return model.ArticleText{}, fmt.Errorf("%w：登录恢复次数用尽：%w", model.ErrLogin, err)
			}
			recoveries++
			a.deps.Reporter.Hint("请在浏览器中完成登录/验证后按回车继续")
			if readyErr := a.deps.Login.EnsureReady(ctx); readyErr != nil {
				return model.ArticleText{}, fmt.Errorf("%w：登录恢复失败：%w", model.ErrLogin, readyErr)
			}
			continue
		}

		attempt++
		if attempt < cfg.ArticleRetryLimit {
			if sleepErr := sleepWithContext(ctx, backoffFor(cfg, attempt-1)); sleepErr != nil {
				return model.ArticleText{}, fetchError("重试等待", sleepErr)
			}
		}
	}
	return model.ArticleText{}, fmt.Errorf("%w：重试 %d 次后仍失败：%w", model.ErrFetch, cfg.ArticleRetryLimit, lastErr)
}

// saveArticle 落盘一篇成功正文：**先写正文并 fsync，再原子写 state**（审查意见 16）。
func (a *App) saveArticle(
	ctx context.Context,
	issue model.Issue,
	issueDir string,
	current model.State,
	text model.ArticleText,
) (model.State, error) {
	target, found := articleByOrder(issue, text.Order)
	if !found {
		return current, fmt.Errorf("%w：第 %d 篇不在当期目录中", model.ErrFetch, text.Order)
	}

	body := text.Body()
	textPath := filepath.Join(issue.DirName, "articles", articleFileName(text.Order, text.Title))
	if err := a.deps.Artifacts.WriteArticleText(textPath, body); err != nil {
		return current, fetchError("写入正文", err)
	}

	entry := model.ArticleState{
		Order:         text.Order,
		URL:           target.URL,
		NormalizedURL: target.Normalized,
		Title:         text.Title,
		Author:        text.Author,
		Status:        model.StatusSuccess,
		TextPath:      textPath,
		Hash:          state.Hash(body),
	}
	updated := state.WithArticleState(current, entry)
	if err := a.deps.States.Save(ctx, issueDir, updated); err != nil {
		return current, fetchError("保存进度", err)
	}
	return updated, nil
}

// recordFailure 把一篇的失败记入状态并落盘，同时输出告警。
func (a *App) recordFailure(
	ctx context.Context,
	issueDir string,
	current model.State,
	target model.Article,
	cause error,
) (model.State, error) {
	a.deps.Reporter.Warn(fmt.Sprintf("第 %d 篇失败：%s（%v）", target.Order, target.Title, cause))

	entry := model.ArticleState{
		Order:         target.Order,
		URL:           target.URL,
		NormalizedURL: target.Normalized,
		Title:         target.Title,
		Status:        model.StatusFail,
	}
	updated := state.WithArticleState(current, entry)
	if err := a.deps.States.Save(ctx, issueDir, updated); err != nil {
		return current, fetchError("保存失败状态", err)
	}
	return updated, nil
}

// articleByOrder 按期目录顺序查找篇目。
func articleByOrder(issue model.Issue, order int) (model.Article, bool) {
	for _, article := range issue.Articles {
		if article.Order == order {
			return article, true
		}
	}
	return model.Article{}, false
}

// articleFileName 生成正文文件名：NNN-<sanitize 后的标题>.txt。
func articleFileName(order int, title string) string {
	sanitized := issue.Sanitize(title)
	if sanitized == "" {
		sanitized = "article"
	}
	return fmt.Sprintf("%03d-%s.txt", order, sanitized)
}

// backoffFor 取第 index 次退避时长；配置不足时退化为 0。
func backoffFor(cfg config.Config, index int) time.Duration {
	if index < 0 || index >= len(cfg.RetryBackoff) {
		return 0
	}
	return cfg.RetryBackoff[index]
}

// sleepWithContext 是可被取消的定时等待，避免无上限挂起（spec 3.2-3）。
func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
