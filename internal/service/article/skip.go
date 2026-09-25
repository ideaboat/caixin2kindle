package article

import "strings"

// imageColumnPrefixes 是「图片型栏目」的标题前缀。这类报道以图片为主，正文容器与文字稿不同，
// 纯文字 EPUB 无法承载其内容；按需求方 2026-09-25 确认：直接放弃抓取并给出说明，
// 而不是为一个栏目猜测或堆叠正文容器退化规则（需求、设计与简洁的平衡）。
var imageColumnPrefixes = []string{"显影"}

// SkipReason 返回「有意跳过该篇」的原因；空串表示正常抓取。
// 判定只看期号页列表标题的前缀，不依赖文章页 DOM，因而与页面结构解耦、可稳定单测。
func SkipReason(title string) string {
	trimmed := strings.TrimSpace(title)
	for _, prefix := range imageColumnPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return "图片型栏目以图片为主，纯文字 EPUB 无法承载其内容"
		}
	}
	return ""
}
