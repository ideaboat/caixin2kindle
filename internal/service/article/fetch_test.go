package article

import (
	"context"
	"errors"
	"slices"
	"testing"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/testutil"
)

// pagedRouteParagraphs 是「下一页」路线（article-paged-01..04 去掉并存按钮）拼接后的期望正文。
// 跨页重复的题图图说、署名与 br 分隔行按段落级去重只保留首次出现（架构 §6.2 H5）。
var pagedRouteParagraphs = []string{
	"示例题图说明。图：示例摄影",
	"文｜财新周刊 示例记者",
	"示例正文·第1节·第一段：为 selector 与完整性判定准备的合成文本，不含真实报道内容。",
	"示例正文·第1节·第二段：含一个示例内部链接，提取时只保留链接文字。",
	"本行用于验证 br 转段落分隔。",
	"示例正文图说明·第1节。图：示例摄影",
	"示例小节一",
	"示例正文·第1节·第三段：小节标题之后的合成文本，用于验证小节归属。",
	"示例小节二",
	"示例正文·第2节·第一段：为 selector 与完整性判定准备的合成文本，不含真实报道内容。",
	"示例正文·第2节·第二段：含一个示例内部链接，提取时只保留链接文字。",
	"示例正文图说明·第2节。图：示例摄影",
	"示例小节三",
	"示例正文·第3节·第一段：为 selector 与完整性判定准备的合成文本，不含真实报道内容。",
	"示例正文·第3节·第二段：含一个示例内部链接，提取时只保留链接文字。",
	"示例正文图说明·第3节。图：示例摄影",
	"示例小节四",
	"示例正文·第4节·第一段：为 selector 与完整性判定准备的合成文本，不含真实报道内容。",
	"示例正文·第4节·最后一段：结尾处有一个指向文章自身的链接。",
}

// testArticle 是抓取用例使用的篇目元数据。
var testArticle = model.Article{
	Order: 1,
	Title: "封面报道｜示例报道标题",
	URL:   "https://weekly.caixin.com/2026-09-18/900000001.html",
}

// newTestFetcher 组装一个不真正等待的 Fetcher，并返回记录型 waiter。
func newTestFetcher(mutate func(*config.Config)) (*Fetcher, *noopClickWaiter) {
	waiter := &noopClickWaiter{}
	return NewFetcher(defaultSet(), testConfig(mutate), waiter), waiter
}

// TestFetchFullTextRoute 覆盖「余下全文单击即完整」：一次展开后早退，正文不重不漏。
func TestFetchFullTextRoute(t *testing.T) {
	// Arrange
	nav := &testutil.FakeNavigator{Frames: []string{
		mustFixture(t, "article-fulltext-before.html"),
		mustFixture(t, "article-fulltext-after.html"),
	}}
	fetcher, waiter := newTestFetcher(nil)

	// Act
	text, err := fetcher.Fetch(context.Background(), nav, testArticle)

	// Assert
	if err != nil {
		t.Fatalf("Fetch 返回错误：%v", err)
	}
	if text.Title != testArticle.Title {
		t.Errorf("Title = %q，期望 %q", text.Title, testArticle.Title)
	}
	if text.Author != "示例记者" {
		t.Errorf("Author = %q，期望 %q", text.Author, "示例记者")
	}
	if !slices.Equal(text.Paragraphs, pagedRouteParagraphs) {
		t.Errorf("Paragraphs 与期望不一致（跨页重复段落按全局去重只留首次）\n got: %q\nwant: %q", text.Paragraphs, pagedRouteParagraphs)
	}
	if kinds := nav.ActionKinds(); !slices.Equal(kinds, []model.ActionKind{model.ActionExpandFullText}) {
		t.Errorf("动作序列 = %v，期望仅一次展开", kinds)
	}
	if waiter.calls != 1 {
		t.Errorf("篇内等待次数 = %d，期望 1", waiter.calls)
	}
	if nav.NavigatedURL != testArticle.URL {
		t.Errorf("导航 URL = %q，期望 %q", nav.NavigatedURL, testArticle.URL)
	}
}

// TestFetchNextPageRoute 覆盖「下一页逐页点到按钮消失」，并回归末页正文不丢（审查意见 2）。
func TestFetchNextPageRoute(t *testing.T) {
	// Arrange
	nav := &testutil.FakeNavigator{Frames: []string{
		withoutExpand(t, "article-paged-01.html"),
		withoutExpand(t, "article-paged-02.html"),
		withoutExpand(t, "article-paged-03.html"),
		withoutExpand(t, "article-paged-04.html"),
	}}
	fetcher, _ := newTestFetcher(nil)

	// Act
	text, err := fetcher.Fetch(context.Background(), nav, testArticle)

	// Assert
	if err != nil {
		t.Fatalf("Fetch 返回错误：%v", err)
	}
	if !slices.Equal(text.Paragraphs, pagedRouteParagraphs) {
		t.Errorf("Paragraphs 与期望不一致\n got: %q\nwant: %q", text.Paragraphs, pagedRouteParagraphs)
	}
	wantKinds := []model.ActionKind{model.ActionNextPage, model.ActionNextPage, model.ActionNextPage}
	if kinds := nav.ActionKinds(); !slices.Equal(kinds, wantKinds) {
		t.Errorf("动作序列 = %v，期望 %v", kinds, wantKinds)
	}
}

// TestFetchPaywallReturnsLoginError 覆盖未登录：付费墙可见必须返回 ErrLogin 供 app 走登录恢复。
func TestFetchPaywallReturnsLoginError(t *testing.T) {
	// Arrange
	nav := &testutil.FakeNavigator{Frames: []string{mustFixture(t, "article-paywall.html")}}
	fetcher, _ := newTestFetcher(nil)

	// Act
	_, err := fetcher.Fetch(context.Background(), nav, testArticle)

	// Assert
	if !errors.Is(err, model.ErrLogin) {
		t.Fatalf("错误应包装 model.ErrLogin，实际：%v", err)
	}
}

// TestFetchIncompleteReturnsFetchError 回归「done 出口的不完整正文判失败」（审查意见 3）。
func TestFetchIncompleteReturnsFetchError(t *testing.T) {
	// Arrange
	nav := &testutil.FakeNavigator{Frames: []string{mustFixture(t, "article-incomplete.html")}}
	fetcher, _ := newTestFetcher(nil)

	// Act
	_, err := fetcher.Fetch(context.Background(), nav, testArticle)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("错误应包装 model.ErrFetch，实际：%v", err)
	}
	if errors.Is(err, model.ErrLogin) {
		t.Fatalf("不应判为登录失败：%v", err)
	}
}

// TestFetchClickLimitStopsRun 覆盖单篇点击合计上限（spec 3.7-5）。
func TestFetchClickLimitStopsRun(t *testing.T) {
	// Arrange：按钮点不消失，末帧保持不动
	nav := &testutil.FakeNavigator{Frames: []string{mustFixture(t, "article-fulltext-before.html")}}
	fetcher, _ := newTestFetcher(func(cfg *config.Config) { cfg.ArticleClickLimit = 3 })

	// Act
	_, err := fetcher.Fetch(context.Background(), nav, testArticle)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("错误应包装 model.ErrFetch，实际：%v", err)
	}
	if len(nav.Actions) != 3 {
		t.Errorf("实际点击 %d 次，期望恰好 3 次（达到上限即停）", len(nav.Actions))
	}
}

// TestFetchTotalStepsLimitStopsRun 覆盖动作总步数兜底闸。
func TestFetchTotalStepsLimitStopsRun(t *testing.T) {
	// Arrange
	nav := &testutil.FakeNavigator{Frames: []string{mustFixture(t, "article-fulltext-before.html")}}
	fetcher, _ := newTestFetcher(func(cfg *config.Config) { cfg.ArticleTotalSteps = 3 })

	// Act
	_, err := fetcher.Fetch(context.Background(), nav, testArticle)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("错误应包装 model.ErrFetch，实际：%v", err)
	}
}

func TestFetchPropagatesSentinelErrors(t *testing.T) {
	fetcher, _ := newTestFetcher(nil)
	cases := []struct {
		name string
		nav  *testutil.FakeNavigator
	}{
		{
			name: "导航返回登录失败",
			nav:  &testutil.FakeNavigator{NavigateErr: model.ErrLogin},
		},
		{
			name: "取 HTML 返回登录失败",
			nav:  &testutil.FakeNavigator{HTMLErr: model.ErrLogin},
		},
		{
			name: "执行动作返回登录失败",
			nav: &testutil.FakeNavigator{
				Frames:     []string{mustFixture(t, "article-fulltext-before.html")},
				ExecuteErr: model.ErrLogin,
			},
		},
		{
			name: "登出探测命中",
			nav: &testutil.FakeNavigator{
				Frames: []string{mustFixture(t, "article-fulltext-before.html")},
				Logout: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			_, err := fetcher.Fetch(context.Background(), tc.nav, testArticle)

			// Assert
			if !errors.Is(err, model.ErrLogin) {
				t.Fatalf("错误应包装 model.ErrLogin，实际：%v", err)
			}
		})
	}
}

// TestFetchWithoutPaginationSucceeds 覆盖无分页/无小节文章的正常路径。
func TestFetchWithoutPaginationSucceeds(t *testing.T) {
	// Arrange
	html := `<html><body><div id="the_content"><div id="conTit"><h1>无分页标题</h1>
<span id="author_baidu">作者：示例记者</span></div><div class="content"><div class="textbox">
<div id="Main_Content_Val"><p>唯一段落。</p></div></div></div></div></body></html>`
	nav := &testutil.FakeNavigator{Frames: []string{html}}
	fetcher, _ := newTestFetcher(nil)

	// Act
	text, err := fetcher.Fetch(context.Background(), nav, testArticle)

	// Assert
	if err != nil {
		t.Fatalf("Fetch 返回错误：%v", err)
	}
	if want := []string{"唯一段落。"}; !slices.Equal(text.Paragraphs, want) {
		t.Errorf("Paragraphs = %q，期望 %q", text.Paragraphs, want)
	}
	if len(nav.Actions) != 0 {
		t.Errorf("无分页文章不应产生点击，实际 %d 次", len(nav.Actions))
	}
}

// TestFetchFallsBackToIndexTitleAndByline 覆盖实测暴露的页面差异（有文章页无 #conTit h1）：
// 正文页取不到标题/署名时回退到列表页的条目信息，不得因此判整篇失败。
func TestFetchFallsBackToIndexTitleAndByline(t *testing.T) {
	// Arrange
	html := `<html><body><div id="the_content"><div id="conTit"></div>
<div class="content"><div class="textbox"><div id="Main_Content_Val"><p>正文段落。</p></div></div></div></div></body></html>`
	nav := &testutil.FakeNavigator{Frames: []string{html}}
	fetcher, _ := newTestFetcher(nil)
	indexEntry := model.Article{
		Order:      21,
		Title:      "显影｜亲历德国式家庭照护",
		Author:     "文｜财新周刊 示例记者",
		URL:        "https://weekly.caixin.com/2026-09-19/900000021.html",
		Normalized: "https://weekly.caixin.com/2026-09-19/900000021.html",
	}

	// Act
	text, err := fetcher.Fetch(context.Background(), nav, indexEntry)

	// Assert
	if err != nil {
		t.Fatalf("Fetch 返回错误：%v", err)
	}
	if text.Title != indexEntry.Title {
		t.Errorf("Title = %q，期望回退为列表标题 %q", text.Title, indexEntry.Title)
	}
	if text.Author != indexEntry.Author {
		t.Errorf("Author = %q，期望回退为列表署名 %q", text.Author, indexEntry.Author)
	}
}

// TestFetchEmptyBodyFails 覆盖空正文防线：页面事实全部满足但没有任何段落时不得写 success。
func TestFetchEmptyBodyFails(t *testing.T) {
	// Arrange
	html := `<html><body><div id="the_content"><div id="conTit"><h1>无正文标题</h1></div>
<div class="content"><div class="textbox"><div id="Main_Content_Val"></div></div></div></div></body></html>`
	nav := &testutil.FakeNavigator{Frames: []string{html}}
	fetcher, _ := newTestFetcher(nil)

	// Act
	_, err := fetcher.Fetch(context.Background(), nav, testArticle)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("空正文应包装 model.ErrFetch，实际：%v", err)
	}
}
