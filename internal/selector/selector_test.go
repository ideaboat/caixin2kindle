package selector

import (
	"slices"
	"testing"
)

// TestDefaultMatchesFinalizedTable 钉住 architecture.md §8.1 定稿表：
// 本包是 selector 的唯一真相源，任何一处漂移都应在此失败。
func TestDefaultMatchesFinalizedTable(t *testing.T) {
	// Arrange
	set := Default()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"I1 期号首选", set.Issue.PeriodTitle, "div.mainMagContent div.report div.title"},
		{"I2 期号回退1 head title", set.Issue.PeriodHeadTitle, "head > title"},
		{"I3 期号回退2 面包屑", set.Issue.PeriodBreadcrumb, "div.positionNav > a"},
		{"I4 卷期 source", set.Issue.VolumeSource, "div.source"},
		{"I4 卷期 date", set.Issue.VolumeDate, "div.date span"},
		{"I4 卷期 文章页", set.Issue.VolumeArticleInfo, "div#artInfo"},
		{"I6 条目链接", set.Issue.EntryLink, "dl > dt > a[href]"},
		{"I7 条目标题", set.Issue.EntryTitle, "dl > dt > a"},
		{"I8 条目署名", set.Issue.EntryByline, "dl > dd.date"},
		{"I8 条目摘要", set.Issue.EntrySummary, "dl > dd"},
		{"I9 栏目名", set.Issue.ColumnName, "div.magIntrotit > span"},

		{"A1 标题", set.Article.Title, "#the_content #conTit h1"},
		{"A2 作者首选", set.Article.Author, "#the_content #conTit #author_baidu"},
		{"A3 作者回退", set.Article.AuthorFallback, "#Main_Content_Val p > b"},
		{"A4 导语", set.Article.Subhead, "#the_content #conTit div#subhead.subhead"},
		{"A5 正文容器", set.Article.Body, "#the_content div.content div.textbox > div#Main_Content_Val"},
		{"A6 正文段落", set.Article.Paragraph, "p"},
		{"A7 小节标题", set.Article.SectionHead, "h2.cx-app-content-subheads"},
		{"A8 题图图说", set.Article.LeadCaption, "#the_content div.media dl.media_pic > dd"},
		{"A9 正文图图说", set.Article.ImageCaption, "cximg div.article_img_talk"},

		{"F1 按钮容器", set.Article.PageBtnContainer, "#the_content div#pageNext div#pageBtn"},
		{"F1 按钮集", set.Article.PageButtons, "#the_content div#pageNext div#pageBtn > a"},
		{"F2 付费墙可见性", set.Article.PagePaywall, "#the_content div#chargeWall div#pcapp div#chargeWallContent"},
		{"F3 小节表", set.Article.PageNavSections, "#the_content div#pageNext ul#pageNav > li"},

		{"L1 付费墙可见（登录信号）", set.Login.Paywall, "div#chargeWallContent"},

		{"按钮文案 余下全文", set.Article.LabelExpand, "余下全文"},
		{"按钮文案 下一页", set.Article.LabelNextPage, "下一页"},
		{"按钮文案 上一页", set.Article.LabelPrevPage, "上一页"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Assert
			if tc.got != tc.want {
				t.Fatalf("selector 与 §8.1 定稿表不一致\n got: %q\nwant: %q", tc.got, tc.want)
			}
		})
	}
}

// TestDefaultListFields 覆盖 §8.1 中的列表型 selector（条目容器、排除项、剔除清单、登录页锚点）。
func TestDefaultListFields(t *testing.T) {
	// Arrange
	set := Default()

	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{
			"I5 条目容器",
			set.Issue.EntryContainers,
			[]string{"div.report dl", "div.magContent2 dl"},
		},
		{
			"I10 排除非文章链接",
			set.Issue.ExcludeLinkScopes,
			[]string{"div.cover div.subscribe a", "div.positionNav a", "div.bottom", "div.navBottom"},
		},
		{
			"E1-E13 整块剔除",
			set.Article.RemoveBlocks,
			[]string{
				"div#artInfo",
				"div#questions_container",
				"div.pip",
				"div#pageNext",
				"div.content-tag",
				"div.lanmu_textend",
				"div.moreReport",
				"div.idetor",
				"div#chargeWall",
				"div#pcapp",
				"div#pay-layer-ad",
				"div#pay-layer-pro-ad",
				"div#pay-box",
				"p.aitt",
				"script",
				"style",
			},
		},
		{
			"E12 媒体占位",
			set.Article.RemoveMedia,
			[]string{"img", "picture", "source", "svg", "video", "audio", "iframe"},
		},
		{
			"E11 分页锚点",
			set.Article.RemovePageAnchors,
			[]string{`a[name^="page"]`, `anchor[id^="page"]`},
		},
		{
			"E14/E15 链接拆壳",
			set.Article.UnwrapLinks,
			[]string{"a"},
		},
		{
			"A1 标题内剔除",
			set.Article.TitleStrips,
			[]string{"em.icon_key"},
		},
		{
			"L2 登录表单",
			set.Login.Form,
			[]string{"form#loginForm", `input[name*="password"]`, ".loginBox"},
		},
		{
			"L3 验证码",
			set.Login.Captcha,
			[]string{`iframe[src*="captcha"]`, `img[src*="captcha"]`, "#captchaImg"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Assert
			if !slices.Equal(tc.got, tc.want) {
				t.Fatalf("selector 列表与 §8.1 定稿表不一致\n got: %q\nwant: %q", tc.got, tc.want)
			}
		})
	}
}
