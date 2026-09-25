package ebook

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
)

// testIssue 构造一期用于 EPUB 结构断言。
func testIssue() model.Issue {
	return model.Issue{
		ID:      "财新周刊第1224期",
		DirName: "财新周刊第1224期",
		Articles: []model.Article{
			{Order: 1, Title: "甲篇", Normalized: "https://x/1.html"},
			{Order: 2, Title: "乙篇", Normalized: "https://x/2.html"},
		},
	}
}

// testTexts 构造两篇正文。
func testTexts() []model.ArticleText {
	return []model.ArticleText{
		{Order: 1, Title: "甲篇", Author: "示例记者甲", Paragraphs: []string{"甲段一", "甲段二"}},
		{Order: 2, Title: "乙篇", Author: "文｜财新周刊 示例记者乙", Paragraphs: []string{"乙段一"}},
	}
}

// openArchive 解压 EPUB 字节，返回 zip.Reader 与各条目内容。
func openArchive(t *testing.T, data []byte) (*zip.Reader, map[string]string) {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("EPUB 不是合法 zip：%v", err)
	}
	entries := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatalf("打开条目 %s 失败：%v", file.Name, err)
		}
		content, err := io.ReadAll(handle)
		handle.Close()
		if err != nil {
			t.Fatalf("读取条目 %s 失败：%v", file.Name, err)
		}
		entries[file.Name] = string(content)
	}
	return reader, entries
}

func TestBuildArchiveLayout(t *testing.T) {
	// Arrange
	issue := testIssue()

	// Act
	name, data, err := NewBuilder().Build(issue, testTexts())

	// Assert
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}
	if name != "财新周刊第1224期.epub" {
		t.Errorf("文件名 = %q，期望 %q", name, "财新周刊第1224期.epub")
	}

	reader, entries := openArchive(t, data)
	required := []string{
		"mimetype",
		"META-INF/container.xml",
		"OEBPS/content.opf",
		"OEBPS/nav.xhtml",
		"OEBPS/text/chapter-001.xhtml",
		"OEBPS/text/chapter-002.xhtml",
	}
	for _, path := range required {
		if _, found := entries[path]; !found {
			t.Errorf("EPUB 缺少条目 %s", path)
		}
	}

	// mimetype 必须是首个条目且不压缩（EPUB 规范硬要求）
	if reader.File[0].Name != "mimetype" {
		t.Errorf("首个条目 = %q，期望 mimetype", reader.File[0].Name)
	}
	if reader.File[0].Method != zip.Store {
		t.Errorf("mimetype 压缩方式 = %d，期望 Store(0)", reader.File[0].Method)
	}
	if entries["mimetype"] != "application/epub+zip" {
		t.Errorf("mimetype 内容 = %q", entries["mimetype"])
	}
	if !strings.Contains(entries["META-INF/container.xml"], "OEBPS/content.opf") {
		t.Error("container.xml 未指向 OEBPS/content.opf")
	}
}

func TestBuildPackageMetadataAndSpine(t *testing.T) {
	// Arrange
	issue := testIssue()

	// Act
	_, data, err := NewBuilder().Build(issue, testTexts())

	// Assert
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}
	_, entries := openArchive(t, data)
	opf := entries["OEBPS/content.opf"]

	for _, want := range []string{
		`version="3.0"`,
		"<dc:title>财新周刊第1224期</dc:title>",
		"<dc:language>zh-CN</dc:language>",
		"<dc:creator>财新周刊</dc:creator>",
		`id="chapter-001"`,
		`id="chapter-002"`,
		`<itemref idref="chapter-001"/>`,
		`<itemref idref="chapter-002"/>`,
		`properties="nav"`,
	} {
		if !strings.Contains(opf, want) {
			t.Errorf("content.opf 缺少 %q\n实际内容：\n%s", want, opf)
		}
	}

	if strings.Index(opf, "chapter-001") > strings.Index(opf, `id="chapter-002"`) {
		t.Error("清单项应按目录顺序出现")
	}
}

func TestBuildChapterContent(t *testing.T) {
	// Arrange
	issue := testIssue()

	// Act
	_, data, err := NewBuilder().Build(issue, testTexts())

	// Assert
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}
	_, entries := openArchive(t, data)

	first := entries["OEBPS/text/chapter-001.xhtml"]
	for _, want := range []string{"<h1>甲篇</h1>", "作者：示例记者甲", "<p>甲段一</p>", "<p>甲段二</p>", `lang="zh-CN"`} {
		if !strings.Contains(first, want) {
			t.Errorf("第一章缺少 %q\n实际内容：\n%s", want, first)
		}
	}

	second := entries["OEBPS/text/chapter-002.xhtml"]
	if !strings.Contains(second, "文｜财新周刊 示例记者乙") {
		t.Errorf("回退署名应原样保留，实际：\n%s", second)
	}
	if strings.Contains(second, "作者：文｜") {
		t.Error("署名已含「文｜」时不应再加「作者：」前缀")
	}
}

func TestBuildNavOrderAndTitles(t *testing.T) {
	// Arrange
	issue := testIssue()

	// Act
	_, data, err := NewBuilder().Build(issue, testTexts())

	// Assert
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}
	_, entries := openArchive(t, data)
	nav := entries["OEBPS/nav.xhtml"]

	firstIndex := strings.Index(nav, "甲篇")
	secondIndex := strings.Index(nav, "乙篇")
	if firstIndex < 0 || secondIndex < 0 {
		t.Fatalf("nav.xhtml 缺少章节标题：\n%s", nav)
	}
	if firstIndex > secondIndex {
		t.Error("nav.xhtml 章节顺序与目录顺序不一致")
	}
	if !strings.Contains(nav, `epub:type="toc"`) {
		t.Error("nav.xhtml 缺少 EPUB3 导航属性 epub:type=\"toc\"")
	}
}

// TestBuildSortsByOrder 覆盖入参乱序：输出必须按 ArticleText.Order 排序。
func TestBuildSortsByOrder(t *testing.T) {
	// Arrange
	texts := []model.ArticleText{
		{Order: 2, Title: "乙篇", Paragraphs: []string{"乙段"}},
		{Order: 1, Title: "甲篇", Paragraphs: []string{"甲段"}},
	}

	// Act
	_, data, err := NewBuilder().Build(testIssue(), texts)

	// Assert
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}
	_, entries := openArchive(t, data)
	if !strings.Contains(entries["OEBPS/text/chapter-001.xhtml"], "甲段") {
		t.Error("第一章应为 Order=1 的文章")
	}
	if !strings.Contains(entries["OEBPS/text/chapter-002.xhtml"], "乙段") {
		t.Error("第二章应为 Order=2 的文章")
	}
}

// TestBuildEscapesMarkup 覆盖注入边界：正文里的尖括号与 & 必须被转义，不得成为标签。
func TestBuildEscapesMarkup(t *testing.T) {
	// Arrange
	texts := []model.ArticleText{
		{Order: 1, Title: "标题<脚本>", Paragraphs: []string{"危险<script>alert(1)</script>&文本"}},
	}

	// Act
	_, data, err := NewBuilder().Build(testIssue(), texts)

	// Assert
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}
	_, entries := openArchive(t, data)
	chapter := entries["OEBPS/text/chapter-001.xhtml"]
	if strings.Contains(chapter, "<script>") {
		t.Errorf("正文不得引入可执行标签：\n%s", chapter)
	}
	for _, want := range []string{"&lt;script&gt;", "&amp;"} {
		if !strings.Contains(chapter, want) {
			t.Errorf("缺少转义结果 %q\n实际：\n%s", want, chapter)
		}
	}
}

func TestBuildRejectsEmptyTexts(t *testing.T) {
	// Act
	_, _, err := NewBuilder().Build(testIssue(), nil)

	// Assert
	if !errors.Is(err, model.ErrConvert) {
		t.Fatalf("错误应包装 model.ErrConvert，实际：%v", err)
	}
}

// TestBuildIsDeterministic 保证同一输入产出相同字节，便于产物回归与哈希比对。
func TestBuildIsDeterministic(t *testing.T) {
	// Arrange
	issue := testIssue()
	builder := NewBuilder()

	// Act
	_, first, err := builder.Build(issue, testTexts())
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}
	_, second, err := builder.Build(issue, testTexts())
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}

	// Assert
	if !bytes.Equal(first, second) {
		t.Error("同一输入两次构建的字节不一致")
	}
}
