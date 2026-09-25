package app

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"caixin2kindle/internal/model"
	"caixin2kindle/internal/service/ebook"
)

// TestRunWithRealEPUBBuilder 是跨层集成用例：把真实的 service/ebook 生成器接入 app，
// 验证「抓取 → 组装入参 → 生成 EPUB → 落盘」整条链路（落盘目标仍是内存 store）。
func TestRunWithRealEPUBBuilder(t *testing.T) {
	// Arrange
	h := newHarness(t)
	h.scriptHappyArticles()
	h.scanner.List = []model.Volume{kindleVolume()}
	h.epubBuilder = ebook.NewBuilder()
	h.rebuild()

	// Act
	result, err := h.app.Run(context.Background(), h.cfg)

	// Assert
	if err != nil {
		t.Fatalf("Run 返回错误：%v", err)
	}
	data, found := h.artifacts.Files[result.EPUBPath]
	if !found {
		t.Fatalf("EPUB 未写入 %s", result.EPUBPath)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("产物不是合法 EPUB（zip）：%v", err)
	}
	names := make(map[string]bool, len(reader.File))
	for _, file := range reader.File {
		names[file.Name] = true
	}
	required := []string{
		"mimetype",
		"META-INF/container.xml",
		"OEBPS/content.opf",
		"OEBPS/nav.xhtml",
		"OEBPS/text/chapter-001.xhtml",
		"OEBPS/text/chapter-006.xhtml",
	}
	for _, want := range required {
		if !names[want] {
			t.Errorf("EPUB 缺少条目 %s", want)
		}
	}
	if reader.File[0].Name != "mimetype" {
		t.Errorf("首个条目 = %q，期望 mimetype", reader.File[0].Name)
	}
	if reader.File[0].Method != zip.Store {
		t.Errorf("mimetype 压缩方式 = %d，期望 Store(0)", reader.File[0].Method)
	}
}
