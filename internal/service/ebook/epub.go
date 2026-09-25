// Package ebook 是纯生成逻辑：不落盘、不起进程（§7 审查意见 8/9）。
// EPUB 字节由 Builder 产出，ebook-convert 的参数由 BuildConvertPlan 产出，两者都可在单测中断言。
package ebook

import (
	"archive/zip"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"html"
	"io"
	"slices"
	"strings"
	"time"

	"caixin2kindle/internal/model"
)

// fixedModifiedTime 固定所有 zip 条目的时间戳，使同一输入产出逐字节一致的 EPUB，
// 便于产物回归与哈希比对（adapter 与判定层都不依赖构建时间）。
var fixedModifiedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Builder 实现 app.EPUBBuilder：把一期文章组装为 EPUB 3 字节。
type Builder struct{}

// NewBuilder 构造 EPUB 生成器。
func NewBuilder() *Builder {
	return &Builder{}
}

// Build 按当期目录顺序生成 EPUB 3（纯文字、无图片），返回文件名与完整字节。
// 元数据：语言 zh-CN、书名 = 期号、作者 = 财新周刊；每章含作者署名（spec 3.8）。
func (b *Builder) Build(issue model.Issue, texts []model.ArticleText) (string, []byte, error) {
	if len(texts) == 0 {
		return "", nil, fmt.Errorf("%w：没有可写入 EPUB 的文章", model.ErrConvert)
	}
	ordered := slices.Clone(texts)
	slices.SortStableFunc(ordered, func(left, right model.ArticleText) int {
		return cmp.Compare(left.Order, right.Order)
	})

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	if err := writeEntries(archive, issue, ordered); err != nil {
		// 写入已失败：仍然关闭归档器并合并其错误，避免丢弃 Close 的失败信息。
		return "", nil, errors.Join(err, archive.Close())
	}
	if err := archive.Close(); err != nil {
		return "", nil, fmt.Errorf("%w：生成 EPUB 失败：%w", model.ErrConvert, err)
	}
	return issue.DirName + ".epub", buffer.Bytes(), nil
}

// writeEntries 按固定顺序写入全部条目：mimetype 必须首个且不压缩（EPUB 规范硬要求）。
func writeEntries(archive *zip.Writer, issue model.Issue, texts []model.ArticleText) error {
	if err := writeStored(archive, "mimetype", "application/epub+zip"); err != nil {
		return err
	}
	documents := []struct {
		name    string
		content string
	}{
		{name: "META-INF/container.xml", content: containerDocument},
		{name: "OEBPS/content.opf", content: packageDocument(issue, texts)},
		{name: "OEBPS/nav.xhtml", content: navigationDocument(texts)},
	}
	for _, document := range documents {
		if err := writeDeflated(archive, document.name, document.content); err != nil {
			return err
		}
	}
	for index, text := range texts {
		if err := writeDeflated(archive, chapterPath(index), chapterDocument(text)); err != nil {
			return err
		}
	}
	return nil
}

// packageDocument 生成 OEBPS/content.opf：EPUB 3 元数据 + 清单 + 阅读顺序。
func packageDocument(issue model.Issue, texts []model.ArticleText) string {
	var manifest, spine strings.Builder
	for index := range texts {
		identifier := fmt.Sprintf("chapter-%03d", index+1)
		fmt.Fprintf(&manifest, "    <item id=%q href=%q media-type=\"application/xhtml+xml\"/>\n",
			identifier, chapterHref(index))
		fmt.Fprintf(&spine, "    <itemref idref=%q/>\n", identifier)
	}
	escapedID := escapeText(issue.ID)
	return fmt.Sprintf(packageTemplate, escapedID, escapedID, modifiedTimestamp, manifest.String(), spine.String())
}

// navigationDocument 生成 OEBPS/nav.xhtml：EPUB 3 导航文档，章节顺序与目录一致。
func navigationDocument(texts []model.ArticleText) string {
	var items strings.Builder
	for index, text := range texts {
		fmt.Fprintf(&items, "      <li><a href=%q>%s</a></li>\n", chapterHref(index), escapeText(text.Title))
	}
	return fmt.Sprintf(navigationTemplate, items.String())
}

// chapterDocument 生成单章 XHTML：标题、署名与段落；全部文本经转义，正文不可能注入标签。
func chapterDocument(text model.ArticleText) string {
	var body strings.Builder
	for _, paragraph := range text.Paragraphs {
		if paragraph == "" {
			continue
		}
		fmt.Fprintf(&body, "    <p>%s</p>\n", escapeText(paragraph))
	}

	byline := ""
	if text.Author != "" {
		byline = fmt.Sprintf("    <p class=\"byline\">%s</p>\n", escapeText(bylineText(text.Author)))
	}

	escapedTitle := escapeText(text.Title)
	return fmt.Sprintf(chapterTemplate, escapedTitle, escapedTitle, byline, body.String())
}

// bylineText 规范署名：已含「文｜」标记的原样保留，否则补「作者：」前缀。
func bylineText(author string) string {
	if strings.HasPrefix(author, "文｜") || strings.HasPrefix(author, "文|") {
		return author
	}
	return "作者：" + author
}

// chapterPath 返回第 index（0 起算）章的包内路径。
func chapterPath(index int) string {
	return fmt.Sprintf("OEBPS/text/chapter-%03d.xhtml", index+1)
}

// chapterHref 返回第 index（0 起算）章在包内的相对链接。
func chapterHref(index int) string {
	return fmt.Sprintf("text/chapter-%03d.xhtml", index+1)
}

// escapeText 转义 XML 文本，防止正文内容改变文档结构。
func escapeText(text string) string {
	return html.EscapeString(text)
}

// writeStored 写入不压缩条目（仅 mimetype 使用）。
func writeStored(archive *zip.Writer, name, content string) error {
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetModTime(fixedModifiedTime)
	entry, err := archive.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("%w：写入 EPUB 条目 %s 失败：%w", model.ErrConvert, name, err)
	}
	if _, err := io.WriteString(entry, content); err != nil {
		return fmt.Errorf("%w：写入 EPUB 条目 %s 失败：%w", model.ErrConvert, name, err)
	}
	return nil
}

// writeDeflated 写入默认压缩条目。
func writeDeflated(archive *zip.Writer, name, content string) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(fixedModifiedTime)
	entry, err := archive.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("%w：写入 EPUB 条目 %s 失败：%w", model.ErrConvert, name, err)
	}
	if _, err := io.WriteString(entry, content); err != nil {
		return fmt.Errorf("%w：写入 EPUB 条目 %s 失败：%w", model.ErrConvert, name, err)
	}
	return nil
}
