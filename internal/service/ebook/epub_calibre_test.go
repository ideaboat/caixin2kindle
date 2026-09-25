package ebook

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildProducesCalibreReadableEPUB 是可选交叉集成用例：本机装有 calibre 时用 ebook-meta 校验产物，
// 未安装或 -short 时跳过（go-rules/testing.md：集成依赖须显式可用，缺失时跳过而非失败）。
func TestBuildProducesCalibreReadableEPUB(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过外部工具校验")
	}
	tool, err := exec.LookPath("ebook-meta")
	if err != nil {
		t.Skip("未安装 calibre 的 ebook-meta，跳过产物校验")
	}

	// Arrange
	name, data, err := NewBuilder().Build(testIssue(), testTexts())
	if err != nil {
		t.Fatalf("Build 返回错误：%v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("写出 EPUB 失败：%v", err)
	}

	// Act
	output, err := exec.Command(tool, path).CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("ebook-meta 无法读取生成的 EPUB：%v\n%s", err, output)
	}
	for _, want := range []string{"财新周刊第1224期", "财新周刊"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("ebook-meta 输出缺少 %q\n实际输出：\n%s", want, output)
		}
	}
}
