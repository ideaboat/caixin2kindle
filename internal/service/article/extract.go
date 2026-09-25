package article

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/selector"
)

// Page 是单个页面快照的纯解析产物：标题/作者/正文段落，以及服务完整性判定的页面事实。
// 标题与作者只有第 1 页有效，调用方按 §6.2 步骤 5 只取一次。
type Page struct {
	Title      string          // 文章标题，仅第 1 页有效
	Author     string          // 署名，仅第 1 页有效
	Paragraphs []string        // 本页正文段落（含小节标题与图说），按 DOM 顺序
	Facts      model.PageFacts // 页面事实：#pageBtn 按钮集、付费墙可见性、#pageNav 小节表与小节全集
}

// sectionOrdinalPattern 匹配小节标题的序号前缀（如「01 示例小节一」中的「01 」）。
// 按 §8.1 约定，正则里的 \s 为 ASCII 语义；全角空白由 foldWhitespace 先行处理。
var sectionOrdinalPattern = regexp.MustCompile(`^[\s]*[0-9]{1,2}[\s]+`)

// Extract 解析单页文章 HTML：先读取页面事实与标题/作者，再剔除非正文元素，最后按 DOM 顺序拼段落。
// 标题/署名允许缺失（实测有栏目页面的标题节点形态不同）：此处返回空串，由 Fetcher 回退到列表页信息；
// 只有正文容器缺失才视为结构异常并返回包装 model.ErrFetch 的错误。
func Extract(pageHTML string, set selector.Set) (Page, error) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(pageHTML))
	if err != nil {
		return Page{}, fmt.Errorf("%w：解析文章页失败：%w", model.ErrFetch, err)
	}

	page := Page{
		Title:  extractTitle(document, set.Article),
		Author: extractAuthor(document, set.Article),
		Facts:  extractFacts(document, set.Article),
	}

	leadCaptions := collectCaptions(document, set.Article.LeadCaption)
	cleanDOM(document, set.Article)

	paragraphs, err := collectParagraphs(document, set.Article, leadCaptions)
	if err != nil {
		return Page{}, err
	}
	page.Paragraphs = paragraphs
	return page, nil
}

// extractTitle 读取 A1 标题，剔除 TitleStrips 指定的装饰节点后折叠空白。
func extractTitle(document *goquery.Document, set selector.Article) string {
	title := document.Find(set.Title).First()
	if title.Length() == 0 {
		return ""
	}
	for _, strip := range set.TitleStrips {
		title.Find(strip).Remove()
	}
	return foldWhitespace(selectionText(title))
}

// extractAuthor 按 A2 优先、A3 回退读取署名；两者皆无时返回空串（署名非必需）。
func extractAuthor(document *goquery.Document, set selector.Article) string {
	if primary := document.Find(set.Author).First(); primary.Length() > 0 {
		text := foldWhitespace(selectionText(primary))
		text = strings.TrimPrefix(text, "作者：")
		text = strings.TrimPrefix(text, "作者:")
		if text != "" {
			return text
		}
	}

	var fallback string
	document.Find(set.AuthorFallback).EachWithBreak(func(_ int, candidate *goquery.Selection) bool {
		text := foldWhitespace(selectionText(candidate))
		if strings.HasPrefix(text, "文｜") || strings.HasPrefix(text, "文|") {
			fallback = text
			return false
		}
		return true
	})
	return fallback
}

// extractFacts 读取 F1–F4 四类页面事实。必须在剔除非正文节点之前调用：
// F1/F3 位于 div#pageNext 内，F2 位于 div#chargeWall 内，二者随后都会被移除。
func extractFacts(document *goquery.Document, set selector.Article) model.PageFacts {
	var facts model.PageFacts

	document.Find(set.PageButtons).Each(func(_ int, button *goquery.Selection) {
		if text := foldWhitespace(selectionText(button)); text != "" {
			facts.Buttons = append(facts.Buttons, text)
		}
	})

	if paywall := document.Find(set.PagePaywall).First(); paywall.Length() > 0 {
		facts.PaywallVisible = !hasInlineDisplayNone(paywall.AttrOr("style", ""))
	}

	document.Find(set.PageNavSections).Each(func(_ int, item *goquery.Selection) {
		text := foldWhitespace(selectionText(item.Find("a").First()))
		if text == "" {
			text = foldWhitespace(selectionText(item))
		}
		if text != "" {
			facts.NavSections = append(facts.NavSections, stripSectionOrdinal(text))
		}
	})

	document.Find(set.SectionHead).Each(func(_ int, head *goquery.Selection) {
		if text := foldWhitespace(selectionText(head)); text != "" {
			facts.BodySections = append(facts.BodySections, text)
		}
	})
	return facts
}

// collectCaptions 读取 A8 题图图说；图说在 div.media 内，位于正文容器之外，故单独收集后前置。
func collectCaptions(document *goquery.Document, captionSelector string) []string {
	var captions []string
	document.Find(captionSelector).Each(func(_ int, caption *goquery.Selection) {
		if paragraph := captionParagraph(selectionText(caption)); paragraph != "" {
			captions = append(captions, paragraph)
		}
	})
	return captions
}

// cleanDOM 按 §4.2 / §8.1 剔除清单就地清理文档：整块移除、分页锚点、媒体节点及其空容器。
func cleanDOM(document *goquery.Document, set selector.Article) {
	for _, block := range set.RemoveBlocks {
		document.Find(block).Remove()
	}
	for _, anchor := range set.RemovePageAnchors {
		document.Find(anchor).Remove()
	}

	var emptiedParents []*goquery.Selection
	for _, media := range set.RemoveMedia {
		document.Find(media).Each(func(_ int, node *goquery.Selection) {
			if parent := node.Parent(); parent.Length() > 0 && !isProtectedContainer(parent, set) {
				emptiedParents = append(emptiedParents, parent)
			}
			node.Remove()
		})
	}
	for _, parent := range emptiedParents {
		if isEmptyElement(parent) {
			parent.Remove()
		}
	}
}

// collectParagraphs 在正文容器内按 DOM 顺序收集段落，并前置题图图说。
func collectParagraphs(document *goquery.Document, set selector.Article, leadCaptions []string) ([]string, error) {
	body := document.Find(set.Body).First()
	if body.Length() == 0 {
		return nil, fmt.Errorf("%w：未找到正文容器（%s）", model.ErrFetch, set.Body)
	}

	paragraphs := append([]string(nil), leadCaptions...)
	tokens := strings.Join([]string{set.Paragraph, set.SectionHead, set.ImageCaption}, ", ")
	body.Find(tokens).Each(func(_ int, node *goquery.Selection) {
		if node.Is(set.ImageCaption) {
			if paragraph := captionParagraph(selectionText(node)); paragraph != "" {
				paragraphs = append(paragraphs, paragraph)
			}
			return
		}
		paragraphs = append(paragraphs, foldParagraphs(selectionText(node))...)
	})
	return paragraphs, nil
}

// captionParagraph 把图说规范为段落：图说多已自带「图：」标注，缺失时补前缀（§4.2）。
func captionParagraph(text string) string {
	folded := foldWhitespace(text)
	if folded == "" {
		return ""
	}
	if !strings.Contains(folded, "图：") {
		folded = "图：" + folded
	}
	return folded
}

// foldParagraphs 把节点文本按换行拆段（<br> 已转成 \n），逐段折叠空白并丢弃空段。
func foldParagraphs(text string) []string {
	var paragraphs []string
	for _, line := range strings.Split(text, "\n") {
		if folded := foldWhitespace(line); folded != "" {
			paragraphs = append(paragraphs, folded)
		}
	}
	return paragraphs
}

// foldWhitespace 折叠全部空白（含全角空格与 &nbsp;）为单个半角空格并去首尾（§8.1 通用约定）。
func foldWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// selectionText 递归拼接节点文本：文本节点原样写入，<br> 转成换行，其余元素递归。
// 链接只保留文字、天然丢弃 href（E15），无需额外拆壳。
func selectionText(selection *goquery.Selection) string {
	var builder strings.Builder
	selection.Contents().Each(func(_ int, child *goquery.Selection) {
		node := child.Get(0)
		if node == nil {
			return
		}
		switch node.Type {
		case html.TextNode:
			builder.WriteString(node.Data)
		case html.ElementNode:
			if node.Data == "br" {
				builder.WriteString("\n")
				return
			}
			builder.WriteString(selectionText(child))
		}
	})
	return builder.String()
}

// hasInlineDisplayNone 判断 inline style 是否声明 display:none（去空白转小写后包含即可，不做层叠计算）。
func hasInlineDisplayNone(style string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(style), ""))
	return strings.Contains(normalized, "display:none")
}

// stripSectionOrdinal 去掉小节标题的序号前缀（F3/F4 比对前统一形态）。
func stripSectionOrdinal(text string) string {
	return strings.TrimSpace(sectionOrdinalPattern.ReplaceAllString(text, ""))
}

// isProtectedContainer 报告 parent 是否属于不可移除的容器（html/body 与正文容器）。
func isProtectedContainer(parent *goquery.Selection, set selector.Article) bool {
	if parent.Is(set.Body) {
		return true
	}
	node := parent.Get(0)
	if node == nil {
		return true
	}
	switch node.Data {
	case "html", "body", "head":
		return true
	default:
		return false
	}
}

// isEmptyElement 报告元素是否既无文本也无子元素（媒体节点移除后用于清理空容器）。
func isEmptyElement(selection *goquery.Selection) bool {
	if selection.Length() == 0 {
		return false
	}
	if foldWhitespace(selectionText(selection)) != "" {
		return false
	}
	hasChildElement := false
	selection.Contents().Each(func(_ int, child *goquery.Selection) {
		if node := child.Get(0); node != nil && node.Type == html.ElementNode {
			hasChildElement = true
		}
	})
	return !hasChildElement
}
