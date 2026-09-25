package issue

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
)

// issueURL 是 fixture 对应的期号页地址（用于 slug 回退）。
const issueURL = "https://weekly.caixin.com/2026/cw1224/"

func TestParseIssueFixture(t *testing.T) {
	// Arrange
	html := mustFixture(t, "issue-settled.html")

	// Act
	issue, err := ParseIssue(html, selector.Default(), issueURL)

	// Assert
	if err != nil {
		t.Fatalf("ParseIssue 返回错误：%v", err)
	}
	if issue.ID != "财新周刊第1224期" {
		t.Errorf("ID = %q，期望 %q", issue.ID, "财新周刊第1224期")
	}
	if issue.DirName != "财新周刊第1224期" {
		t.Errorf("DirName = %q，期望 %q", issue.DirName, "财新周刊第1224期")
	}

	wantTitles := []string{
		"封面报道｜示例标题甲",
		"示例标题乙",
		"示例标题丙",
		"示例标题戊",
		"示例标题丁",
		"示例标题己",
	}
	wantNormalized := []string{
		"https://weekly.caixin.com/2026-09-18/900000001.html",
		"https://weekly.caixin.com/2026-09-19/900000002.html",
		"https://weekly.caixin.com/2026-09-19/900000003.html",
		"https://weekly.caixin.com/2026-09-19/900000005.html",
		"https://weekly.caixin.com/2026-09-19/900000004.html",
		"https://weekly.caixin.com/2026-09-19/900000006.html",
	}
	if len(issue.Articles) != len(wantTitles) {
		t.Fatalf("文章数 = %d，期望 %d（应按归一化 URL 去重）", len(issue.Articles), len(wantTitles))
	}
	for index, article := range issue.Articles {
		if article.Order != index+1 {
			t.Errorf("第 %d 篇 Order = %d，期望 %d", index, article.Order, index+1)
		}
		if article.Title != wantTitles[index] {
			t.Errorf("第 %d 篇 Title = %q，期望 %q", index, article.Title, wantTitles[index])
		}
		if article.Normalized != wantNormalized[index] {
			t.Errorf("第 %d 篇 Normalized = %q，期望 %q", index, article.Normalized, wantNormalized[index])
		}
		if !strings.HasPrefix(article.URL, "https://weekly.caixin.com/") {
			t.Errorf("第 %d 篇 URL = %q，期望绝对地址", index, article.URL)
		}
	}

	if got := issue.Articles[1].Author; got != "文｜示例作者乙" {
		t.Errorf("第 2 篇 Author = %q，期望 %q", got, "文｜示例作者乙")
	}
	if got := issue.Articles[0].Author; got != "" {
		t.Errorf("封面篇无 dd.date，Author 应为空串，实际 %q", got)
	}
}

// TestParseIssueDoesNotScanWholePageForPeriod 钉住 §8.1 的硬约束：
// 期号正则只允许作用在 I1/I2/I3，绝不能对整页文本全扫（否则卷期 37 会被误判为期号）。
func TestParseIssueDoesNotScanWholePageForPeriod(t *testing.T) {
	// Arrange
	html := `<html><head><title>财新周刊频道</title></head><body>
<div class="mainMagContent"><div class="report"><div class="title"></div>
<dl><dt><a href="https://weekly.caixin.com/2026-09-18/900000001.html">示例标题甲</a></dt></dl></div></div>
<p>本文来源于《财新周刊》2026年第9999期</p>
</body></html>`

	// Act
	issue, err := ParseIssue(html, selector.Default(), issueURL)

	// Assert
	if err != nil {
		t.Fatalf("ParseIssue 返回错误：%v", err)
	}
	if issue.ID != "2026-cw1224" {
		t.Errorf("ID = %q，期望回退为 slug %q（不得被整页文本里的期号污染）", issue.ID, "2026-cw1224")
	}
	if issue.DirName != "2026-cw1224" {
		t.Errorf("DirName = %q，期望 %q", issue.DirName, "2026-cw1224")
	}
}

// TestParseIssuePeriodFallbacks 覆盖 I1→I2→I3 的期号回退顺序。
func TestParseIssuePeriodFallbacks(t *testing.T) {
	entry := `<dl><dt><a href="https://weekly.caixin.com/2026-09-18/900000001.html">甲</a></dt></dl>`
	cases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "I1 命中",
			html: `<html><body><div class="mainMagContent"><div class="report"><div class="title">《财新周刊》总第1224期</div>` + entry + `</div></div></body></html>`,
			want: "财新周刊第1224期",
		},
		{
			name: "I1 缺失回退 I2（head title）",
			html: `<html><head><title>《财新周刊》第1234期</title></head><body><div class="mainMagContent"><div class="report">` + entry + `</div></div></body></html>`,
			want: "财新周刊第1234期",
		},
		{
			name: "I1/I2 缺失回退 I3（面包屑最后一个 a）",
			html: `<html><head><title>财新周刊频道</title></head><body><div class="positionNav">位置：<a href="/">频道</a> &gt; <a href="/x/">《财新周刊》第1250期</a></div>` + entry + `</body></html>`,
			want: "财新周刊第1250期",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			issue, err := ParseIssue(tc.html, selector.Default(), issueURL)

			// Assert
			if err != nil {
				t.Fatalf("ParseIssue 返回错误：%v", err)
			}
			if issue.ID != tc.want {
				t.Errorf("ID = %q，期望 %q", issue.ID, tc.want)
			}
		})
	}
}

func TestParseIssueResolvesRelativeLinks(t *testing.T) {
	// Arrange
	html := `<html><head><title>《财新周刊》第1224期</title></head><body>
<dl><dt><a href="/2026-09-18/900000001.html">甲</a></dt></dl>
<dl><dt><a href="/2026-09-19/900000002.html">乙</a></dt></dl>
</body></html>`

	// Act
	issue, err := ParseIssue(html, selector.Default(), issueURL)

	// Assert
	if err != nil {
		t.Fatalf("ParseIssue 返回错误：%v", err)
	}
	want := []string{
		"https://weekly.caixin.com/2026-09-18/900000001.html",
		"https://weekly.caixin.com/2026-09-19/900000002.html",
	}
	if len(issue.Articles) != 2 {
		t.Fatalf("文章数 = %d，期望 2", len(issue.Articles))
	}
	for index, article := range issue.Articles {
		if article.Normalized != want[index] {
			t.Errorf("第 %d 篇 Normalized = %q，期望 %q", index, article.Normalized, want[index])
		}
	}
}

func TestParseIssueEmptyListReportsError(t *testing.T) {
	// Arrange
	html := `<html><head><title>《财新周刊》第1224期</title></head><body></body></html>`

	// Act
	_, err := ParseIssue(html, selector.Default(), issueURL)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("错误应包装 model.ErrFetch，实际：%v", err)
	}
}

// TestParseIssueSkipsLinksOutsideEntries 钉住「只认 dl > dt > a」：导航/订阅等链接不得入列表。
func TestParseIssueSkipsLinksOutsideEntries(t *testing.T) {
	// Arrange
	html := `<html><head><title>《财新周刊》第1224期</title></head><body>
<div class="positionNav"><a href="https://weekly.caixin.com/centuryweeklylist/">频道目录</a></div>
<div class="cover"><div class="subscribe"><a href="https://mall.caixin.com/sub">订阅</a></div></div>
<dl><dt><a href="https://weekly.caixin.com/2026-09-18/900000001.html">甲</a></dt></dl>
<div class="bottom"><a href="https://weekly.caixin.com/about/">关于我们</a></div>
</body></html>`

	// Act
	issue, err := ParseIssue(html, selector.Default(), issueURL)

	// Assert
	if err != nil {
		t.Fatalf("ParseIssue 返回错误：%v", err)
	}
	want := []string{"https://weekly.caixin.com/2026-09-18/900000001.html"}
	if len(issue.Articles) != 1 {
		t.Fatalf("文章数 = %d，期望 1（只认条目链接）", len(issue.Articles))
	}
	if issue.Articles[0].Normalized != want[0] {
		t.Errorf("Normalized = %q，期望 %q", issue.Articles[0].Normalized, want[0])
	}
}

// TestParseIssueDeduplicatesByNormalizedURL 覆盖 URL 归一化去重（spec 3.6）。
func TestParseIssueDeduplicatesByNormalizedURL(t *testing.T) {
	// Arrange
	html := `<html><head><title>《财新周刊》第1224期</title></head><body>
<dl><dt><a href="https://weekly.caixin.com/x/1.html">首次</a></dt></dl>
<dl><dt><a href="https://weekly.caixin.com/x/1.html?from=weekly">重复</a></dt></dl>
<dl><dt><a href="https://weekly.caixin.com/x/1.html/">重复</a></dt></dl>
<dl><dt><a href="https://weekly.caixin.com/x/1.html#page2">重复</a></dt></dl>
</body></html>`

	// Act
	issue, err := ParseIssue(html, selector.Default(), issueURL)

	// Assert
	if err != nil {
		t.Fatalf("ParseIssue 返回错误：%v", err)
	}
	if len(issue.Articles) != 1 {
		t.Fatalf("文章数 = %d，期望 1", len(issue.Articles))
	}
	if issue.Articles[0].Title != "首次" {
		t.Errorf("应保留首次出现的标题，实际 %q", issue.Articles[0].Title)
	}
	if got := issue.Articles[0].Order; got != 1 {
		t.Errorf("Order = %d，期望 1", got)
	}
}

// listingHTML 断言切片顺序辅助：确保列表顺序与 DOM 一致。
func TestParseIssueKeepsDOMOrder(t *testing.T) {
	// Arrange
	html := mustFixture(t, "issue-settled.html")

	// Act
	issue, err := ParseIssue(html, selector.Default(), issueURL)

	// Assert
	if err != nil {
		t.Fatalf("ParseIssue 返回错误：%v", err)
	}
	got := make([]string, 0, len(issue.Articles))
	for _, article := range issue.Articles {
		got = append(got, article.Title)
	}
	want := []string{"封面报道｜示例标题甲", "示例标题乙", "示例标题丙", "示例标题戊", "示例标题丁", "示例标题己"}
	if !slices.Equal(got, want) {
		t.Errorf("列表顺序 = %q，期望 %q", got, want)
	}
}
