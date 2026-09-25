package state

import (
	"testing"

	"caixin2kindle/internal/model"
)

// TestDecideSkipsIntentionallySkippedArticles 覆盖有意跳过（如图片型栏目）：
// 这类篇目既不抓取、也不算失败，绝不能反复进入待抓列表。
func TestDecideSkipsIntentionallySkippedArticles(t *testing.T) {
	// Arrange
	issue := testIssue()
	current, store := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})
	current.Articles[1].Status = model.StatusSkipped
	delete(store.Files, textPath(issue.Articles[1]))

	// Act
	pending, err := Decide(issue, current, store)

	// Assert
	if err != nil {
		t.Fatalf("Decide 返回错误：%v", err)
	}
	if len(pending) != 0 {
		t.Errorf("有意跳过的篇目不应进待抓列表，实际 %v", titlesOf(pending))
	}
}

// TestVerifyTextsIgnoresSkippedArticles 覆盖构建前复核：有意跳过的篇目不算「正文不可用」。
func TestVerifyTextsIgnoresSkippedArticles(t *testing.T) {
	// Arrange
	issue := testIssue()
	current, store := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})
	current.Articles[1].Status = model.StatusSkipped
	delete(store.Files, textPath(issue.Articles[1]))

	// Act
	bad, err := VerifyTexts(issue, current, store)

	// Assert
	if err != nil {
		t.Fatalf("VerifyTexts 返回错误：%v", err)
	}
	if len(bad) != 0 {
		t.Errorf("有意跳过的篇目不应进待回抓列表，实际 %v", titlesOf(bad))
	}
}

// TestNeedRebuildTreatsSkippedAsAcceptable 覆盖重建判定：success 与 skipped 之外才需要重建。
func TestNeedRebuildTreatsSkippedAsAcceptable(t *testing.T) {
	// Arrange
	issue := testIssue()
	current, _ := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})
	current.Articles[1].Status = model.StatusSkipped
	current = MarkBuilt(current, issue.ID, "a.epub", "a.mobi")

	// Act
	rebuild := NeedRebuild(issue, current, true, true)

	// Assert
	if rebuild {
		t.Error("全部 success/skipped 且产物齐备时应可跳过重建")
	}
}
