package issue

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
)

// ParsePeriod 按 I1→I2→I3 的优先级解析期号；I3 取面包屑最后一个链接的文本。
// 严禁对整页文本全扫：期号页同时含卷期「2026年第37期」，全扫会把 37 误判为期号（§8.1 硬约束）。
func ParsePeriod(document *goquery.Document, set selector.Issue) string {
	candidates := []struct {
		selector string
		last     bool
	}{
		{selector: set.PeriodTitle},
		{selector: set.PeriodHeadTitle},
		{selector: set.PeriodBreadcrumb, last: true},
	}
	for _, candidate := range candidates {
		nodes := document.Find(candidate.selector)
		if candidate.last {
			nodes = nodes.Last()
		} else {
			nodes = nodes.First()
		}
		if nodes.Length() == 0 {
			continue
		}
		if period, found := MatchPeriod(nodes.Text()); found {
			return period
		}
	}
	return ""
}

// ParseIssue 解析期号页 HTML：期号（解析失败回退 URL slug）、已 sanitize 的目录名，
// 以及按归一化 URL 去重、保持目录顺序的文章列表（spec 3.6）。列表为空视为页面结构异常。
func ParseIssue(pageHTML string, set selector.Set, rawURL string) (model.Issue, error) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return model.Issue{}, fmt.Errorf("%w：解析期号页失败：%w", model.ErrFetch, err)
	}

	issue := model.Issue{URL: rawURL, ID: ParsePeriod(document, set.Issue)}
	if issue.ID == "" {
		issue.ID = SlugFromURL(rawURL)
	}
	issue.DirName = Sanitize(issue.ID)
	if issue.DirName == "" {
		issue.DirName = Sanitize(SlugFromURL(rawURL))
	}

	issue.Articles = parseArticles(document, set.Issue, rawURL)
	if len(issue.Articles) == 0 {
		return model.Issue{}, fmt.Errorf("%w：期号页未解析到任何文章", model.ErrFetch)
	}
	return issue, nil
}

// parseArticles 只认 I6「dl > dt > a[href]」：导航、订阅、页脚等链接天然不在选择范围内（§8.1 I10）。
// 去重按归一化 URL，保留首次出现位置（§8.1 I11/I12）。
func parseArticles(document *goquery.Document, set selector.Issue, baseURL string) []model.Article {
	seen := make(map[string]bool)
	var articles []model.Article

	document.Find(set.EntryLink).Each(func(_ int, anchor *goquery.Selection) {
		href := strings.TrimSpace(anchor.AttrOr("href", ""))
		if href == "" {
			return
		}
		resolved := resolveURL(baseURL, href)
		normalized := NormalizeURL(resolved)
		if normalized == "" || seen[normalized] {
			return
		}
		seen[normalized] = true
		articles = append(articles, model.Article{
			Order:      len(articles) + 1,
			Title:      foldWhitespace(anchor.Text()),
			Author:     entryAuthor(anchor, set),
			URL:        resolved,
			Normalized: normalized,
		})
	})
	return articles
}

// entryAuthor 读取条目署名（I8，仅用于日志）：取所在 dl 内的 dd.date。
func entryAuthor(anchor *goquery.Selection, set selector.Issue) string {
	entry := anchor.Closest("dl")
	if entry.Length() == 0 {
		return ""
	}
	return foldWhitespace(entry.Find(relativeToEntry(set.EntryByline)).First().Text())
}

// relativeToEntry 把以 dl 为根书写的 I7/I8 selector 转为可在单个 dl 内查询的相对形式。
func relativeToEntry(absolute string) string {
	return strings.TrimPrefix(absolute, "dl > ")
}
