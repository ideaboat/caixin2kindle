package cli

import (
	"bytes"
	"strings"
	"testing"

	"caixin2kindle/internal/port"
)

// 编译期契约断言：实现方签名漂移会让本包测试直接编译失败（架构 §5、§8）。
// app.Reporter 的断言集中在组合根 internal/wire，避免测试依赖并发开发中的 app 包。
var _ port.Prompter = (*Prompter)(nil)

func TestReporter_ProgressFormat(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	reporter := &Reporter{Log: &Logger{Out: &buf}}

	// Act
	reporter.Progress(3, 32, "封面报道")

	// Assert
	got := strings.TrimRight(buf.String(), "\n")
	if got != "第 3/32 篇：封面报道" {
		t.Fatalf("进度行 = %q，期望 %q", got, "第 3/32 篇：封面报道")
	}
}

func TestReporter_WarnAndHintPrefixPlusRedact(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// Act
	reporter.Warn("token=abc123 已失效")
	reporter.Hint("请检查网络")

	// Assert
	got := buf.String()
	if !strings.Contains(got, "警告：token=*** 已失效") {
		t.Fatalf("警告行格式错误：%q", got)
	}
	if !strings.Contains(got, "提示：请检查网络") {
		t.Fatalf("提示行格式错误：%q", got)
	}
	if strings.Contains(got, "abc123") {
		t.Fatalf("警告未脱敏：%q", got)
	}
}

func TestReporter_NilSafety(t *testing.T) {
	// Arrange
	var reporter *Reporter

	// Act / Assert（不 panic 即为通过）
	reporter.Progress(1, 1, "标题")
	reporter.Warn("警告")
	reporter.Hint("提示")
	(&Reporter{}).Progress(1, 1, "标题")
}
