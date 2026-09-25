package state

import (
	"errors"
	"slices"
	"testing"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/testutil"
)

// testIssue 是三篇文章的期，供待抓列表与重建判定用例复用。
func testIssue() model.Issue {
	return model.Issue{
		ID:      "财新周刊第1224期",
		DirName: "财新周刊第1224期",
		Articles: []model.Article{
			{Order: 1, Title: "甲", URL: "https://x/1.html", Normalized: "https://x/1.html"},
			{Order: 2, Title: "乙", URL: "https://x/2.html", Normalized: "https://x/2.html"},
			{Order: 3, Title: "丙", URL: "https://x/3.html", Normalized: "https://x/3.html"},
		},
	}
}

// textPath 返回某一篇的正文文件路径。
func textPath(article model.Article) string {
	return "articles/" + article.Title + ".txt"
}

// successEntry 构造一条“已成功”的状态记录，哈希与 body 一致。
func successEntry(article model.Article, body string) model.ArticleState {
	return model.ArticleState{
		Order:         article.Order,
		URL:           article.URL,
		NormalizedURL: article.Normalized,
		Title:         article.Title,
		Status:        model.StatusSuccess,
		TextPath:      textPath(article),
		Hash:          Hash(body),
	}
}

// fullState 构造三篇全部成功、正文齐全的状态。
func fullState(issue model.Issue, bodies map[int]string) (model.State, *testutil.FakeTextStore) {
	current := NewState(issue)
	store := &testutil.FakeTextStore{Files: map[string]string{}}
	for _, article := range issue.Articles {
		body := bodies[article.Order]
		current.Articles = append(current.Articles, successEntry(article, body))
		store.Files[textPath(article)] = body
	}
	return current, store
}

// titlesOf 把待抓列表归一为标题切片，便于断言。
func titlesOf(articles []model.Article) []string {
	titles := make([]string, 0, len(articles))
	for _, article := range articles {
		titles = append(titles, article.Title)
	}
	return titles
}

func TestNewState(t *testing.T) {
	// Act
	current := NewState(testIssue())

	// Assert
	if current.SchemaVersion != model.SchemaVersion {
		t.Errorf("SchemaVersion = %d，期望 %d", current.SchemaVersion, model.SchemaVersion)
	}
	if current.IssueID != "财新周刊第1224期" {
		t.Errorf("IssueID = %q，期望 %q", current.IssueID, "财新周刊第1224期")
	}
	if len(current.Articles) != 0 {
		t.Errorf("新建状态不应含文章记录，实际 %d 条", len(current.Articles))
	}
	if current.Artifacts.BuiltIssueID != "" {
		t.Errorf("新建状态不应标记已构建，实际 %q", current.Artifacts.BuiltIssueID)
	}
}

func TestDecideSkipsUsableArticles(t *testing.T) {
	// Arrange：三篇全部 success 且正文与哈希一致
	issue := testIssue()
	current, store := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})

	// Act
	pending, err := Decide(issue, current, store)

	// Assert
	if err != nil {
		t.Fatalf("Decide 返回错误：%v", err)
	}
	if len(pending) != 0 {
		t.Errorf("全部可跳过时待抓列表应为空，实际 %v", titlesOf(pending))
	}
}

func TestDecideRequiresRefetch(t *testing.T) {
	issue := testIssue()
	cases := []struct {
		name    string
		mutate  func(*model.State, *testutil.FakeTextStore)
		wantHit []string
	}{
		{
			name: "状态为 pending",
			mutate: func(current *model.State, _ *testutil.FakeTextStore) {
				current.Articles[1].Status = model.StatusPending
			},
			wantHit: []string{"乙"},
		},
		{
			name: "状态为 fail",
			mutate: func(current *model.State, _ *testutil.FakeTextStore) {
				current.Articles[1].Status = model.StatusFail
			},
			wantHit: []string{"乙"},
		},
		{
			name: "正文文件缺失",
			mutate: func(_ *model.State, store *testutil.FakeTextStore) {
				delete(store.Files, textPath(issue.Articles[1]))
			},
			wantHit: []string{"乙"},
		},
		{
			name: "正文与哈希不符",
			mutate: func(_ *model.State, store *testutil.FakeTextStore) {
				store.Files[textPath(issue.Articles[1])] = "被改动的正文"
			},
			wantHit: []string{"乙"},
		},
		{
			name: "TextPath 为空",
			mutate: func(current *model.State, _ *testutil.FakeTextStore) {
				current.Articles[1].TextPath = ""
			},
			wantHit: []string{"乙"},
		},
		{
			name: "状态记录缺失",
			mutate: func(current *model.State, _ *testutil.FakeTextStore) {
				current.Articles = current.Articles[:1]
			},
			wantHit: []string{"乙", "丙"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			current, store := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})
			tc.mutate(&current, store)

			// Act
			pending, err := Decide(issue, current, store)

			// Assert
			if err != nil {
				t.Fatalf("Decide 返回错误：%v", err)
			}
			if !slices.Equal(titlesOf(pending), tc.wantHit) {
				t.Errorf("待抓列表 = %v，期望 %v", titlesOf(pending), tc.wantHit)
			}
		})
	}
}

func TestDecidePropagatesStoreErrors(t *testing.T) {
	issue := testIssue()
	current, store := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})

	cases := []struct {
		name   string
		mutate func(*testutil.FakeTextStore)
	}{
		{name: "存在性检查失败", mutate: func(s *testutil.FakeTextStore) { s.ExistsErr = errors.New("io") }},
		{name: "读取正文失败", mutate: func(s *testutil.FakeTextStore) { s.ReadErr = errors.New("io") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			broken := &testutil.FakeTextStore{Files: store.Files}
			tc.mutate(broken)

			// Act
			_, err := Decide(issue, current, broken)

			// Assert
			if err == nil {
				t.Fatal("store 失败时必须报错，不得静默当作待抓")
			}
		})
	}
}

func TestVerifyTexts(t *testing.T) {
	issue := testIssue()
	cases := []struct {
		name    string
		mutate  func(*model.State, *testutil.FakeTextStore)
		wantHit []string
	}{
		{
			name:   "全部通过",
			mutate: func(*model.State, *testutil.FakeTextStore) {},
		},
		{
			name: "文件缺失入待抓",
			mutate: func(_ *model.State, store *testutil.FakeTextStore) {
				delete(store.Files, textPath(issue.Articles[2]))
			},
			wantHit: []string{"丙"},
		},
		{
			name: "哈希不符入待抓",
			mutate: func(_ *model.State, store *testutil.FakeTextStore) {
				store.Files[textPath(issue.Articles[0])] = "损坏正文"
			},
			wantHit: []string{"甲"},
		},
		{
			name: "非 success 状态入待抓",
			mutate: func(current *model.State, _ *testutil.FakeTextStore) {
				current.Articles[0].Status = model.StatusFail
			},
			wantHit: []string{"甲"},
		},
		{
			name: "状态记录缺失入待抓",
			mutate: func(current *model.State, _ *testutil.FakeTextStore) {
				current.Articles = nil
			},
			wantHit: []string{"甲", "乙", "丙"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			current, store := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})
			tc.mutate(&current, store)

			// Act
			bad, err := VerifyTexts(issue, current, store)

			// Assert
			if err != nil {
				t.Fatalf("VerifyTexts 返回错误：%v", err)
			}
			if !slices.Equal(titlesOf(bad), tc.wantHit) {
				t.Errorf("待回抓列表 = %v，期望 %v", titlesOf(bad), tc.wantHit)
			}
		})
	}
}

func TestVerifyTextsPropagatesStoreErrors(t *testing.T) {
	// Arrange
	issue := testIssue()
	current, _ := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})
	store := &testutil.FakeTextStore{Files: map[string]string{}, ExistsErr: errors.New("io")}

	// Act
	_, err := VerifyTexts(issue, current, store)

	// Assert
	if err == nil {
		t.Fatal("store 失败时必须报错")
	}
}

func TestNeedRebuild(t *testing.T) {
	issue := testIssue()
	built := func(current model.State) model.State {
		return MarkBuilt(current, issue.ID, issue.DirName+".epub", issue.DirName+".mobi")
	}

	cases := []struct {
		name        string
		mutate      func(*model.State)
		epubExists  bool
		mobiExists  bool
		wantRebuild bool
	}{
		{
			name:        "三条件齐备则跳过重建",
			mutate:      func(*model.State) {},
			epubExists:  true,
			mobiExists:  true,
			wantRebuild: false,
		},
		{
			name:        "built_issue_id 不匹配则重建",
			mutate:      func(current *model.State) { current.Artifacts.BuiltIssueID = "财新周刊第1223期" },
			epubExists:  true,
			mobiExists:  true,
			wantRebuild: true,
		},
		{
			name:        "EPUB 缺失则重建",
			mutate:      func(*model.State) {},
			epubExists:  false,
			mobiExists:  true,
			wantRebuild: true,
		},
		{
			name:        "MOBI 缺失则重建（--no-kindle 同样要求两者齐全）",
			mutate:      func(*model.State) {},
			epubExists:  true,
			mobiExists:  false,
			wantRebuild: true,
		},
		{
			name: "有文章未成功则重建",
			mutate: func(current *model.State) {
				current.Articles[2].Status = model.StatusFail
			},
			epubExists:  true,
			mobiExists:  true,
			wantRebuild: true,
		},
		{
			name: "文章记录缺失则重建",
			mutate: func(current *model.State) {
				current.Articles = current.Articles[:1]
			},
			epubExists:  true,
			mobiExists:  true,
			wantRebuild: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			current, _ := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})
			current = built(current)
			tc.mutate(&current)

			// Act
			got := NeedRebuild(issue, current, tc.epubExists, tc.mobiExists)

			// Assert
			if got != tc.wantRebuild {
				t.Errorf("NeedRebuild = %v，期望 %v", got, tc.wantRebuild)
			}
		})
	}
}

func TestWithArticleStateUpdatesAndAppends(t *testing.T) {
	// Arrange
	issue := testIssue()
	current := NewState(issue)
	first := successEntry(issue.Articles[0], "甲文")
	second := successEntry(issue.Articles[1], "乙文")

	// Act
	afterFirst := WithArticleState(current, first)
	afterSecond := WithArticleState(afterFirst, second)
	updated := successEntry(issue.Articles[0], "甲文-新")
	afterUpdate := WithArticleState(afterSecond, updated)

	// Assert
	if len(current.Articles) != 0 {
		t.Errorf("原状态不应被修改，实际 %d 条", len(current.Articles))
	}
	if len(afterSecond.Articles) != 2 {
		t.Fatalf("追加后应有 2 条，实际 %d", len(afterSecond.Articles))
	}
	if len(afterUpdate.Articles) != 2 {
		t.Fatalf("更新不应新增记录，实际 %d 条", len(afterUpdate.Articles))
	}
	if afterUpdate.Articles[0].Hash != updated.Hash {
		t.Errorf("同 URL 记录应被覆盖，实际 Hash = %q", afterUpdate.Articles[0].Hash)
	}
}

func TestWithArticleStateDoesNotAlias(t *testing.T) {
	// Arrange
	issue := testIssue()
	current, _ := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})
	originalHash := current.Articles[0].Hash

	// Act
	updated := WithArticleState(current, successEntry(issue.Articles[0], "改写"))
	updated.Articles[0].Title = "被外部篡改"

	// Assert
	if current.Articles[0].Hash != originalHash {
		t.Error("返回的状态与原状态共享底层数组，违反了值语义")
	}
	if current.Articles[0].Title == "被外部篡改" {
		t.Error("外部改动泄漏回了原状态")
	}
}

func TestMarkBuilt(t *testing.T) {
	// Arrange
	issue := testIssue()
	current, _ := fullState(issue, map[int]string{1: "甲文", 2: "乙文", 3: "丙文"})

	// Act
	built := MarkBuilt(current, issue.ID, "财新周刊第1224期.epub", "财新周刊第1224期.mobi")

	// Assert
	if built.Artifacts.BuiltIssueID != issue.ID {
		t.Errorf("BuiltIssueID = %q，期望 %q", built.Artifacts.BuiltIssueID, issue.ID)
	}
	if built.Artifacts.EPUB != "财新周刊第1224期.epub" || built.Artifacts.MOBI != "财新周刊第1224期.mobi" {
		t.Errorf("产物名记录不正确：%+v", built.Artifacts)
	}
	if len(built.Articles) != len(current.Articles) {
		t.Errorf("MarkBuilt 不应改动文章记录")
	}
	if current.Artifacts.BuiltIssueID != "" {
		t.Error("MarkBuilt 不应修改入参状态")
	}
}
