package issue

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// 期号文案：期号页同时存在「《财新周刊》总第1224期」与「2026年第37期」两套写法，
// 故先试「总第N期」，再退回「第N期」；调用方只允许把正则作用在 I1/I2/I3 三个节点上（§8.1）。
var (
	totalPeriodPattern = regexp.MustCompile(`总第[\s]*([0-9]+)[\s]*期`)
	periodPattern      = regexp.MustCompile(`第[\s]*([0-9]+)[\s]*期`)
	yearSegmentPattern = regexp.MustCompile(`^[0-9]{4}$`)
)

// illegalNameRunes 是文件系统非法或易歧义的字符（§4.1 第 2 条）。
const illegalNameRunes = `\/:*?"<>|`

// nameByteLimit 是目录名/文件名的字节上限（按 rune 边界截断，不切断 UTF-8，§4.1 第 5 条）。
const nameByteLimit = 120

// MatchPeriod 从一段文本里匹配期号并归一为「财新周刊第N期」；匹配不到返回 found=false。
// 文本先折叠空白，以覆盖全角空格与 &nbsp;（§8.1 通用约定）。
func MatchPeriod(text string) (string, bool) {
	folded := foldWhitespace(text)
	if match := totalPeriodPattern.FindStringSubmatch(folded); match != nil {
		return "财新周刊第" + match[1] + "期", true
	}
	if match := periodPattern.FindStringSubmatch(folded); match != nil {
		return "财新周刊第" + match[1] + "期", true
	}
	return "", false
}

// Sanitize 按 §4.1 规则把期号整理为安全的目录名/文件名：NFC 归一化、非法字符替换、
// 折叠空白与下划线、去首尾与尾部点，超长按 rune 边界截断到 120 字节。
// 输入为空串、"." 或 ".." 时返回空串，由调用方回退到 URL slug。
func Sanitize(name string) string {
	normalized := norm.NFC.String(strings.TrimSpace(name))

	var builder strings.Builder
	for _, symbol := range normalized {
		switch {
		case symbol < 0x20 || symbol == 0x7f:
			builder.WriteRune('_')
		case strings.ContainsRune(illegalNameRunes, symbol):
			builder.WriteRune('_')
		default:
			builder.WriteRune(symbol)
		}
	}

	collapsed := collapseSeparators(builder.String())
	collapsed = strings.Trim(collapsed, " _")
	collapsed = strings.TrimRight(collapsed, ".")
	if collapsed == "" || collapsed == "." || collapsed == ".." {
		return ""
	}
	return truncateBytes(collapsed, nameByteLimit)
}

// collapseSeparators 把连续空白与下划线折叠为单个下划线。
func collapseSeparators(text string) string {
	var builder strings.Builder
	pendingSeparator := false
	for _, symbol := range text {
		if symbol == '_' || unicode.IsSpace(symbol) {
			pendingSeparator = true
			continue
		}
		if pendingSeparator && builder.Len() > 0 {
			builder.WriteRune('_')
		}
		pendingSeparator = false
		builder.WriteRune(symbol)
	}
	return builder.String()
}

// truncateBytes 按 rune 边界把字符串截断到不超过 limit 字节。
func truncateBytes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// SlugFromURL 从期号页 URL 提取回退目录名：优先取「4 位年份段 + 紧随其后的期号段」，
// 如 weekly.caixin.com/2026/cw1224/ → 2026-cw1224（spec 3.8）。
func SlugFromURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	segments := nonEmptySegments(parsed.Path)
	for index, segment := range segments {
		if yearSegmentPattern.MatchString(segment) && index+1 < len(segments) {
			return segment + "-" + segments[index+1]
		}
	}
	switch len(segments) {
	case 0:
		return ""
	case 1:
		return segments[0]
	default:
		return segments[len(segments)-2] + "-" + segments[len(segments)-1]
	}
}

// nonEmptySegments 返回路径中的所有非空段。
func nonEmptySegments(path string) []string {
	var segments []string
	for _, segment := range strings.Split(path, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

// NormalizeURL 归一化文章 URL 用于去重：去掉 query 与 fragment，再去掉路径尾部的 "/"。
// 归一化后仍保留原始大小写（不做大小写折叠，§8.1 I11）。
func NormalizeURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return strings.TrimRight(rawURL, "/")
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String()
}

// resolveURL 把 href 解析为绝对地址：相对地址按期号页 URL 解析，非法输入原样返回。
func resolveURL(baseURL, href string) string {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return strings.TrimSpace(href)
	}
	reference, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return strings.TrimSpace(href)
	}
	return base.ResolveReference(reference).String()
}

// foldWhitespace 折叠全部空白（含全角空格与 &nbsp;）为单个半角空格并去首尾。
func foldWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
