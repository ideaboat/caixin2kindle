package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/service/ebook"
	"caixin2kindle/internal/service/kindle"
	"caixin2kindle/internal/service/state"
)

// builtArtifacts 是一次产物构建的结果：更新后的状态、两个成品路径，以及仍未抓到的篇目。
type builtArtifacts struct {
	State    model.State
	EPUBPath string
	MOBIPath string
	Missing  []model.Article // 降级构建时未能提供正文的篇目（v1.9.1）
}

// buildArtifacts 执行「复核 → 回抓 → 再复核」循环，然后产出 EPUB 与 MOBI，最后标记已构建（§7）。
// 回抓额度用尽后仍有不可用篇目时**降级构建**：用成功篇目产出成品并警告缺失篇目，避免整期白跑（v1.9.1）；
// 若一篇可用正文都没有，则不产出空书，返回 ErrFetch。
// 输出文件名由工作区约定固定为 <期号>.epub，故不使用 Build 的 name 返回值。
func (a *App) buildArtifacts(
	ctx context.Context,
	issue model.Issue,
	issueDir string,
	current model.State,
	cfg config.Config,
) (builtArtifacts, error) {
	current, unavailable, err := a.verifyAndRefetch(ctx, issue, issueDir, current, cfg)
	if err != nil {
		return builtArtifacts{}, err
	}
	if len(unavailable) > 0 {
		a.deps.Reporter.Warn(fmt.Sprintf("本期有 %d 篇重试后仍未抓到，将用其余篇目构建：%s",
			len(unavailable), strings.Join(articleTitles(unavailable), "、")))
	}

	texts, err := a.readTexts(issue, current)
	if err != nil {
		return builtArtifacts{}, err
	}
	if len(texts) == 0 {
		return builtArtifacts{}, fmt.Errorf("%w：没有任何可用正文，无法构建", model.ErrFetch)
	}

	_, data, err := a.deps.EPUB.Build(issue, texts)
	if err != nil {
		// EPUB 生成属「产物转换」环节，统一归类为 ErrConvert（退出码 4）。
		return builtArtifacts{}, fmt.Errorf("%w：生成 EPUB 失败：%w", model.ErrConvert, err)
	}

	epubPath, err := a.deps.Artifacts.WriteEPUB(issue.DirName, data)
	if err != nil {
		return builtArtifacts{}, fmt.Errorf("%w：写入 EPUB 失败：%w", model.ErrConvert, err)
	}

	mobiPath := strings.TrimSuffix(epubPath, filepath.Ext(epubPath)) + ".mobi"
	plan := ebook.BuildConvertPlan(epubPath, mobiPath, cfg.OutputProfile)
	if err := a.deps.Converter.ToMOBI(ctx, plan); err != nil {
		return builtArtifacts{}, err
	}

	built := state.MarkBuilt(current, issue.ID, filepath.Base(epubPath), filepath.Base(mobiPath))
	if err := a.deps.States.Save(ctx, issueDir, built); err != nil {
		return builtArtifacts{}, fetchError("保存构建标记", err)
	}
	return builtArtifacts{State: built, EPUBPath: epubPath, MOBIPath: mobiPath, Missing: unavailable}, nil
}

// verifyAndRefetch 是构建前的正文复核回环：回抓次数上限 BuildRetryRounds，
// 每次回抓后都回到复核，保证「回抓成功即构建」（审查意见 4/12）。
// 额度用尽仍有不可用篇目时返回这些篇目（而非报错），交由调用方降级构建。
func (a *App) verifyAndRefetch(
	ctx context.Context,
	issue model.Issue,
	issueDir string,
	current model.State,
	cfg config.Config,
) (model.State, []model.Article, error) {
	retriesLeft := cfg.BuildRetryRounds

	for {
		bad, err := state.VerifyTexts(issue, current, a.deps.Artifacts)
		if err != nil {
			return current, nil, fetchError("构建前复核", err)
		}
		if len(bad) == 0 {
			return current, nil, nil
		}
		if retriesLeft == 0 {
			return current, bad, nil
		}
		a.deps.Reporter.Warn(fmt.Sprintf("构建前复核发现 %d 篇正文不可用，回抓", len(bad)))
		current, err = a.acquire(ctx, issue, issueDir, current, bad, cfg)
		if err != nil {
			return current, nil, err
		}
		retriesLeft--
	}
}

// readTexts 从本地正文重建 EPUB 入参；顺序与目录一致，标题与署名取自状态记录。
// 仅收「已成功且有可读正文」的篇目；其余由 verifyAndRefetch 的缺失列表覆盖，故此处跳过。
func (a *App) readTexts(issue model.Issue, current model.State) ([]model.ArticleText, error) {
	texts := make([]model.ArticleText, 0, len(issue.Articles))
	for _, article := range issue.Articles {
		entry, found := stateEntry(current, article)
		if !found || entry.Status != model.StatusSuccess {
			continue
		}
		body, err := a.deps.Artifacts.ReadArticleText(entry.TextPath)
		if err != nil {
			continue
		}
		texts = append(texts, model.ArticleText{
			Order:      entry.Order,
			Title:      entry.Title,
			Author:     entry.Author,
			Paragraphs: splitParagraphs(body),
		})
	}
	return texts, nil
}

// articleTitles 提取篇目标题，供警告与结果渲染。
func articleTitles(articles []model.Article) []string {
	titles := make([]string, 0, len(articles))
	for _, article := range articles {
		titles = append(titles, fmt.Sprintf("第 %d 篇 %s", article.Order, article.Title))
	}
	return titles
}

// splitParagraphs 还原正文段落：落盘时以空行连接，段内不含换行，故可直接按空行切分。
func splitParagraphs(body string) []string {
	if body == "" {
		return nil
	}
	return strings.Split(body, "\n\n")
}

// deliver 定位 Kindle 并拷贝 MOBI。未检测到设备属正常路径：提示成品位置并正常返回（退出 0）。
func (a *App) deliver(ctx context.Context, cfg config.Config, mobiPath string) (bool, error) {
	if cfg.NoKindle {
		return false, nil
	}

	volumes, err := a.deps.Volumes.Volumes(ctx)
	if err != nil {
		return false, fmt.Errorf("%w：扫描挂载卷失败：%w", model.ErrCopy, err)
	}

	request := model.KindleRequest{Mount: cfg.KindleMount, Explicit: cfg.KindleMountExplicit}
	volume, found, err := a.deps.Kindle.Select(request, volumes)
	if err != nil {
		return false, err
	}
	if !found {
		a.deps.Reporter.Hint(fmt.Sprintf("未检测到设备，成品文件位于 %s，请自行拷入", filepath.Dir(mobiPath)))
		return false, nil
	}

	if kindle.NeedDocumentsDir(request) {
		if err := a.deps.Device.EnsureDocuments(ctx, volume); err != nil {
			return false, err
		}
	}
	if err := a.deps.Device.Copy(ctx, volume, mobiPath, filepath.Base(mobiPath)); err != nil {
		return false, err
	}
	return true, nil
}
