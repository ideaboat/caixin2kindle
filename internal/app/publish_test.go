package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"caixin2kindle/internal/model"
)

func TestBuildArtifactsRefetchesBadThenBuilds(t *testing.T) {
	// Arrange：状态说成功，但第 2 篇正文文件已丢失 → 必须回抓后构建（审查意见 4/12）
	h := newHarness(t)
	parsed := h.issue()
	h.seedAllSuccess(parsed)
	h.scriptHappyArticles()
	target := parsed.Articles[1]
	delete(h.artifacts.Files, h.artifacts.Path(filepath.Join(parsed.DirName, "articles", "002.txt")))
	current := h.states.States[h.issueDir(parsed)]

	// Act
	built, err := h.app.buildArtifacts(context.Background(), parsed, h.issueDir(parsed), current, h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("buildArtifacts 返回错误：%v", err)
	}
	if h.epub.Calls != 1 {
		t.Errorf("回抓成功后必须构建 EPUB，实际 %d 次", h.epub.Calls)
	}
	if len(h.converter.Plans) != 1 {
		t.Fatalf("应执行一次 ebook-convert，实际 %d 次", len(h.converter.Plans))
	}
	if built.State.Artifacts.BuiltIssueID != parsed.ID {
		t.Errorf("BuiltIssueID = %q，期望 %q", built.State.Artifacts.BuiltIssueID, parsed.ID)
	}
	if !slices.Contains(h.nav.NavigatedURLs, target.URL) {
		t.Errorf("应回抓第 2 篇，实际访问 %v", h.nav.NavigatedURLs)
	}
	if built.MOBIPath != filepath.Join(h.issueDir(parsed), parsed.DirName+".mobi") {
		t.Errorf("MOBIPath = %q", built.MOBIPath)
	}
}

// TestBuildArtifactsBuildsRemainingAfterRetriesExhausted 覆盖降级策略：
// 回抓额度用尽仍有篇目不可用时，用成功篇目继续构建，并把缺失篇目录入警告与结果。
func TestBuildArtifactsBuildsRemainingAfterRetriesExhausted(t *testing.T) {
	// Arrange
	h := newHarness(t)
	parsed := h.issue()
	h.seedAllSuccess(parsed)
	h.scriptHappyArticles()
	h.cfg.BuildRetryRounds = 0
	missing := parsed.Articles[3]
	delete(h.artifacts.Files, h.artifacts.Path(filepath.Join(parsed.DirName, "articles", "004.txt")))
	current := h.states.States[h.issueDir(parsed)]

	// Act
	built, err := h.app.buildArtifacts(context.Background(), parsed, h.issueDir(parsed), current, h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("仍有可用正文时应完成构建，实际：%v", err)
	}
	if h.epub.Calls != 1 {
		t.Fatalf("应构建 EPUB，实际 %d 次", h.epub.Calls)
	}
	if len(h.epub.LastTexts) != len(parsed.Articles)-1 {
		t.Errorf("EPUB 入参篇数 = %d，期望 %d", len(h.epub.LastTexts), len(parsed.Articles)-1)
	}
	if len(built.Missing) != 1 || built.Missing[0].Order != missing.Order {
		t.Errorf("Missing = %+v，期望仅第 %d 篇", built.Missing, missing.Order)
	}
	if len(h.reporter.Warnings) == 0 {
		t.Error("缺失篇目应至少产生一条警告")
	}
}

// TestBuildArtifactsFailsWhenNothingUsable 覆盖底线：一篇可用正文都没有时不得产出空书。
func TestBuildArtifactsFailsWhenNothingUsable(t *testing.T) {
	// Arrange
	h := newHarness(t)
	parsed := h.issue()
	h.seedAllSuccess(parsed)
	h.scriptAllArticlesAs("article-incomplete.html")
	h.cfg.BuildRetryRounds = 0
	for _, article := range parsed.Articles {
		delete(h.artifacts.Files, h.artifacts.Path(filepath.Join(parsed.DirName, "articles", fmt.Sprintf("%03d.txt", article.Order))))
	}
	current := h.states.States[h.issueDir(parsed)]

	// Act
	_, err := h.app.buildArtifacts(context.Background(), parsed, h.issueDir(parsed), current, h.cfg)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("无可用正文时应包装 model.ErrFetch，实际：%v", err)
	}
	if h.epub.Calls != 0 {
		t.Error("无可用正文时不得构建 EPUB")
	}
}

func TestBuildArtifactsBuildsWhenAllTextsUsable(t *testing.T) {
	// Arrange
	h := newHarness(t)
	parsed := h.issue()
	h.seedAllSuccess(parsed)
	current := h.states.States[h.issueDir(parsed)]

	// Act
	built, err := h.app.buildArtifacts(context.Background(), parsed, h.issueDir(parsed), current, h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("buildArtifacts 返回错误：%v", err)
	}
	if h.epub.Calls != 1 {
		t.Errorf("EPUB 构建次数 = %d，期望 1", h.epub.Calls)
	}
	if len(h.epub.LastTexts) != 6 {
		t.Errorf("EPUB 入参篇数 = %d，期望 6", len(h.epub.LastTexts))
	}
	if _, found := h.artifacts.Files[built.EPUBPath]; !found {
		t.Errorf("EPUB 未写入 %q", built.EPUBPath)
	}
	if _, found := h.artifacts.Files[built.MOBIPath]; !found {
		t.Errorf("MOBI 未写入 %q", built.MOBIPath)
	}
	if built.State.Artifacts.BuiltIssueID != parsed.ID {
		t.Error("构建结束后应标记 built_issue_id")
	}
}
