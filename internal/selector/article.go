package selector

// defaultArticle 返回文章页的定稿 selector（§8.1(2) 与 §8.1(3)）。
//
// 两条实现须知：
//   - E4 的 div#pageNext 必须**在读取页面事实之后**才移除——F1 按钮集与 F3 小节表都在它内部；
//   - RemoveMedia 命中后，若其容器因此变空，还需移除该空容器（§4.2）。
func defaultArticle() Article {
	return Article{
		Title:          "#the_content #conTit h1",
		TitleStrips:    []string{"em.icon_key"},
		Author:         "#the_content #conTit #author_baidu",
		AuthorFallback: "#Main_Content_Val p > b",
		Subhead:        "#the_content #conTit div#subhead.subhead",
		Body:           "#the_content div.content div.textbox > div#Main_Content_Val",
		Paragraph:      "p",
		SectionHead:    "h2.cx-app-content-subheads",
		LeadCaption:    "#the_content div.media dl.media_pic > dd",
		ImageCaption:   "cximg div.article_img_talk",

		RemoveBlocks: []string{
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
		RemoveMedia:       []string{"img", "picture", "source", "svg", "video", "audio", "iframe"},
		RemovePageAnchors: []string{`a[name^="page"]`, `anchor[id^="page"]`},
		UnwrapLinks:       []string{"a"},

		PageBtnContainer: "#the_content div#pageNext div#pageBtn",
		PageButtons:      "#the_content div#pageNext div#pageBtn > a",
		PagePaywall:      "#the_content div#chargeWall div#pcapp div#chargeWallContent",
		PageNavSections:  "#the_content div#pageNext ul#pageNav > li",

		LabelExpand:   "余下全文",
		LabelNextPage: "下一页",
		LabelPrevPage: "上一页",
	}
}
