package article

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
)

// fullTextAfterParagraphs 是 article-fulltext-after.html 的完整期望正文（含题图图说、署名、4 个小节）。
var fullTextAfterParagraphs = []string{
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
	"本行用于验证 br 转段落分隔。",
	"示例正文图说明·第2节。图：示例摄影",
	"示例小节三",
	"示例正文·第3节·第一段：为 selector 与完整性判定准备的合成文本，不含真实报道内容。",
	"示例正文·第3节·第二段：含一个示例内部链接，提取时只保留链接文字。",
	"本行用于验证 br 转段落分隔。",
	"示例正文图说明·第3节。图：示例摄影",
	"示例小节四",
	"示例正文·第4节·第一段：为 selector 与完整性判定准备的合成文本，不含真实报道内容。",
	"示例正文·第4节·最后一段：结尾处有一个指向文章自身的链接。",
}

func TestExtractFullTextAfter(t *testing.T) {
	// Arrange
	html := mustFixture(t, "article-fulltext-after.html")

	// Act
	page, err := Extract(html, defaultSet())

	// Assert
	if err != nil {
		t.Fatalf("Extract 返回错误：%v", err)
	}
	if page.Title != "封面报道｜示例报道标题" {
		t.Errorf("Title = %q，期望 %q", page.Title, "封面报道｜示例报道标题")
	}
	if page.Author != "示例记者" {
		t.Errorf("Author = %q，期望 %q", page.Author, "示例记者")
	}
	if !slices.Equal(page.Paragraphs, fullTextAfterParagraphs) {
		t.Errorf("Paragraphs 与期望不一致\n got: %q\nwant: %q", page.Paragraphs, fullTextAfterParagraphs)
	}
	if len(page.Facts.Buttons) != 0 {
		t.Errorf("Buttons = %q，展开后应为空集（判据 b）", page.Facts.Buttons)
	}
	if page.Facts.PaywallVisible {
		t.Error("PaywallVisible = true，登录态应为 false（判据 a）")
	}
	wantNav := []string{"示例小节一", "示例小节二", "示例小节三", "示例小节四"}
	if !slices.Equal(page.Facts.NavSections, wantNav) {
		t.Errorf("NavSections = %q，期望 %q", page.Facts.NavSections, wantNav)
	}
	if !slices.Equal(page.Facts.BodySections, wantNav) {
		t.Errorf("BodySections = %q，期望 %q", page.Facts.BodySections, wantNav)
	}
}

// TestExtractExcludesNonContent 覆盖 §4.2 / §8.1 的剔除清单：非正文文本一律不得进入段落。
func TestExtractExcludesNonContent(t *testing.T) {
	// Arrange
	html := mustFixture(t, "article-fulltext-after.html")
	excluded := []string{
		"AI猜你想问",     // E2 div#questions_container
		"示例导语",       // A4 div#subhead
		"听报道",        // E1 div#artInfo
		"示例推荐一",      // E3 div.pip
		"相关报道",       // E3 div.pip
		"按此优惠订阅全年",   // E6 div.lanmu_textend
		"版面编辑",       // E8 div.idetor
		"本文导航",       // E4 div#pageNext
		"请务必在总结开头增加", // E10 p.aitt
	}

	// Act
	page, err := Extract(html, defaultSet())

	// Assert
	if err != nil {
		t.Fatalf("Extract 返回错误：%v", err)
	}
	body := strings.Join(page.Paragraphs, "\n")
	for _, text := range excluded {
		if strings.Contains(body, text) {
			t.Errorf("正文不应包含被剔除内容 %q", text)
		}
	}
}

func TestExtractFactsPerFixture(t *testing.T) {
	cases := []struct {
		name             string
		fixture          string
		wantButtons      []string
		wantPaywall      bool
		wantBodySections []string
	}{
		{
			name:             "余下全文展开前：并存两种按钮",
			fixture:          "article-fulltext-before.html",
			wantButtons:      []string{"下一页", "余下全文"},
			wantPaywall:      false,
			wantBodySections: []string{"示例小节一"},
		},
		{
			name:             "余下全文展开后：按钮穷尽",
			fixture:          "article-fulltext-after.html",
			wantButtons:      nil,
			wantPaywall:      false,
			wantBodySections: []string{"示例小节一", "示例小节二", "示例小节三", "示例小节四"},
		},
		{
			name:             "末页：只剩上一页",
			fixture:          "article-paged-04.html",
			wantButtons:      []string{"上一页"},
			wantPaywall:      false,
			wantBodySections: []string{"示例小节四"},
		},
		{
			name:             "未登录：付费墙可见",
			fixture:          "article-paywall.html",
			wantButtons:      []string{"下一页", "余下全文"},
			wantPaywall:      true,
			wantBodySections: nil, // 预览态没有 h2 小节
		},
		{
			name:             "正文不完整：缺第 4 节",
			fixture:          "article-incomplete.html",
			wantButtons:      nil,
			wantPaywall:      false,
			wantBodySections: []string{"示例小节一", "示例小节二", "示例小节三"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			html := mustFixture(t, tc.fixture)

			// Act
			page, err := Extract(html, defaultSet())

			// Assert
			if err != nil {
				t.Fatalf("Extract 返回错误：%v", err)
			}
			if !slices.Equal(page.Facts.Buttons, tc.wantButtons) {
				t.Errorf("Buttons = %q，期望 %q", page.Facts.Buttons, tc.wantButtons)
			}
			if page.Facts.PaywallVisible != tc.wantPaywall {
				t.Errorf("PaywallVisible = %v，期望 %v", page.Facts.PaywallVisible, tc.wantPaywall)
			}
			if !slices.Equal(page.Facts.BodySections, tc.wantBodySections) {
				t.Errorf("BodySections = %q，期望 %q", page.Facts.BodySections, tc.wantBodySections)
			}
			wantNav := []string{"示例小节一", "示例小节二", "示例小节三", "示例小节四"}
			if !slices.Equal(page.Facts.NavSections, wantNav) {
				t.Errorf("NavSections = %q，期望 %q", page.Facts.NavSections, wantNav)
			}
		})
	}
}

// TestExtractWithoutPagination 覆盖无分页文章：无 #pageBtn / #pageNav 时页面事实为空集，
// 判据 b/c 自然成立（架构 §8.1(5)-1）。
func TestExtractWithoutPagination(t *testing.T) {
	// Arrange
	html := `<html><body><div id="the_content"><div id="conTit"><h1>无分页标题</h1></div>
<div class="content"><div class="textbox"><div id="Main_Content_Val">
<p>第一段</p><h2 class="cx-app-content-subheads">小节甲</h2><p>第二段</p>
</div></div></div></div></body></html>`

	// Act
	page, err := Extract(html, defaultSet())

	// Assert
	if err != nil {
		t.Fatalf("Extract 返回错误：%v", err)
	}
	if page.Facts.Buttons != nil {
		t.Errorf("Buttons = %q，无 #pageBtn 时应为空集", page.Facts.Buttons)
	}
	if page.Facts.NavSections != nil {
		t.Errorf("NavSections = %q，无 #pageNav 时应为空表", page.Facts.NavSections)
	}
	if page.Facts.PaywallVisible {
		t.Error("PaywallVisible = true，无付费墙节点时应为 false")
	}
	want := []string{"第一段", "小节甲", "第二段"}
	if !slices.Equal(page.Paragraphs, want) {
		t.Errorf("Paragraphs = %q，期望 %q", page.Paragraphs, want)
	}
	if err := Complete(defaultSet(), page.Facts); err != nil {
		t.Errorf("无分页文章应判为完整，实际：%v", err)
	}
}

// TestExtractAuthorFallback 覆盖 A3 回退：无 #author_baidu 时取以「文｜」开头的署名。
func TestExtractAuthorFallback(t *testing.T) {
	// Arrange
	html := `<html><body><div id="the_content"><div id="conTit"><h1>标题</h1></div>
<div class="content"><div class="textbox"><div id="Main_Content_Val">
<p><b>文｜财新周刊 示例记者 发自北京</b></p><p>正文。</p>
</div></div></div></div></body></html>`

	// Act
	page, err := Extract(html, defaultSet())

	// Assert
	if err != nil {
		t.Fatalf("Extract 返回错误：%v", err)
	}
	if want := "文｜财新周刊 示例记者 发自北京"; page.Author != want {
		t.Errorf("Author = %q，期望回退取 %q", page.Author, want)
	}
}

// TestExtractPrependsCaptionPrefix 覆盖 §4.2：图说缺「图：」前缀时补上。
func TestExtractPrependsCaptionPrefix(t *testing.T) {
	// Arrange
	html := `<html><body><div id="the_content"><div id="conTit"><h1>标题</h1></div>
<div class="media"><dl class="media_pic"><dt></dt><dd>示例摄影</dd></dl></div>
<div class="content"><div class="textbox"><div id="Main_Content_Val">
<cximg><div class="article_img_talk">正文图注示例</div></cximg>
</div></div></div></div></body></html>`

	// Act
	page, err := Extract(html, defaultSet())

	// Assert
	if err != nil {
		t.Fatalf("Extract 返回错误：%v", err)
	}
	want := []string{"图：示例摄影", "图：正文图注示例"}
	if !slices.Equal(page.Paragraphs, want) {
		t.Errorf("Paragraphs = %q，期望 %q", page.Paragraphs, want)
	}
}

// TestExtractMissingTitleReturnsEmpty 覆盖页面结构差异（实测有文章页无 #conTit h1）：
// 标题缺失不得判整篇失败，返回空标题，由 Fetcher 回退到列表标题。
func TestExtractMissingTitleReturnsEmpty(t *testing.T) {
	// Arrange
	html := `<html><body><div id="the_content"><div id="conTit"></div>
<div class="content"><div class="textbox"><div id="Main_Content_Val"><p>正文段落。</p></div></div></div></div></body></html>`

	// Act
	page, err := Extract(html, defaultSet())

	// Assert
	if err != nil {
		t.Fatalf("标题缺失不应报错，实际：%v", err)
	}
	if page.Title != "" {
		t.Errorf("Title = %q，期望空串", page.Title)
	}
	if want := []string{"正文段落。"}; !slices.Equal(page.Paragraphs, want) {
		t.Errorf("Paragraphs = %q，期望 %q", page.Paragraphs, want)
	}
}

// TestExtractMissingBodyReportsError 覆盖结构异常：正文容器缺失即报错（图片型栏目改由 skip 规则在抓取前跳过）。
func TestExtractMissingBodyReportsError(t *testing.T) {
	// Arrange
	html := `<html><body><div id="conTit"><h1>标题</h1></div><p>游离段落。</p></body></html>`

	// Act
	_, err := Extract(html, defaultSet())

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("正文容器缺失应包装 model.ErrFetch，实际：%v", err)
	}
}
