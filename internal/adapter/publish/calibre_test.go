package publish

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

// convertPlanFixture 返回一份典型的 ebook-convert 调用计划（spec 3.9）。
func convertPlanFixture() model.ConvertPlan {
	return model.ConvertPlan{
		InputPath:     "/tmp/issue.epub",
		OutputPath:    "/tmp/issue.mobi",
		OutputProfile: "kindle",
		Args:          []string{"/tmp/issue.epub", "/tmp/issue.mobi", "--output-profile", "kindle"},
	}
}

func TestConverterToMOBI_MissingPath(t *testing.T) {
	// Arrange
	runCalled := false
	converter := &Converter{Run: func(context.Context, string, ...string) ([]byte, error) {
		runCalled = true
		return nil, nil
	}}

	// Act
	err := converter.ToMOBI(context.Background(), convertPlanFixture())

	// Assert
	if !errors.Is(err, model.ErrConvert) {
		t.Fatalf("未找到 ebook-convert 应包装 model.ErrConvert，实际：%v", err)
	}
	if !strings.Contains(err.Error(), "ebook-convert") {
		t.Errorf("错误应点明 ebook-convert，实际：%v", err)
	}
	if runCalled {
		t.Error("可执行文件缺失时不得启动进程")
	}
}

func TestConverterToMOBI_EmptyPlan(t *testing.T) {
	cases := []struct {
		name string
		plan model.ConvertPlan
	}{
		{name: "完全没有参数", plan: model.ConvertPlan{}},
		{name: "只有路径没有参数表", plan: model.ConvertPlan{InputPath: "/tmp/a.epub", OutputPath: "/tmp/a.mobi"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			runCalled := false
			converter := &Converter{Path: "/fake/ebook-convert", Run: func(context.Context, string, ...string) ([]byte, error) {
				runCalled = true
				return nil, nil
			}}

			// Act
			err := converter.ToMOBI(context.Background(), tc.plan)

			// Assert
			if !errors.Is(err, model.ErrConvert) {
				t.Fatalf("空计划应包装 model.ErrConvert，实际：%v", err)
			}
			if runCalled {
				t.Error("空计划不得启动进程")
			}
		})
	}
}

func TestConverterToMOBI_Success(t *testing.T) {
	// Arrange
	var gotName string
	var gotArgs []string
	var hasDeadline bool
	converter := &Converter{
		Path:    "/fake/ebook-convert",
		Timeout: time.Minute,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = append([]string(nil), args...)
			_, hasDeadline = ctx.Deadline()
			return []byte("converted"), nil
		},
	}

	// Act
	err := converter.ToMOBI(context.Background(), convertPlanFixture())

	// Assert
	if err != nil {
		t.Fatalf("成功路径不应返回错误：%v", err)
	}
	if gotName != "/fake/ebook-convert" {
		t.Errorf("可执行文件 = %q，期望 %q", gotName, "/fake/ebook-convert")
	}
	if !equalStrings(gotArgs, convertPlanFixture().Args) {
		t.Errorf("参数 = %v，期望 %v", gotArgs, convertPlanFixture().Args)
	}
	if !hasDeadline {
		t.Error("执行上下文应带超时上限")
	}
}

func TestConverterToMOBI_FailureIncludesOutputTail(t *testing.T) {
	// Arrange
	converter := &Converter{
		Path: "/fake/ebook-convert",
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("Traceback\ncalibre 版本过低，无法转换"), errors.New("exit status 1")
		},
	}

	// Act
	err := converter.ToMOBI(context.Background(), convertPlanFixture())

	// Assert
	if !errors.Is(err, model.ErrConvert) {
		t.Fatalf("非零退出应包装 model.ErrConvert，实际：%v", err)
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("错误应保留原始失败原因，实际：%v", err)
	}
	if !strings.Contains(err.Error(), "calibre 版本过低，无法转换") {
		t.Errorf("错误应附带工具输出尾部，实际：%v", err)
	}
}

func TestConverterToMOBI_FailureWithoutOutput(t *testing.T) {
	// Arrange
	converter := &Converter{
		Path: "/fake/ebook-convert",
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("exec: file does not exist")
		},
	}

	// Act
	err := converter.ToMOBI(context.Background(), convertPlanFixture())

	// Assert
	if !errors.Is(err, model.ErrConvert) {
		t.Fatalf("启动失败应包装 model.ErrConvert，实际：%v", err)
	}
	if !strings.Contains(err.Error(), "无输出") {
		t.Errorf("无输出时应给出占位说明，实际：%v", err)
	}
}

func TestConverterToMOBI_DefaultTimeout(t *testing.T) {
	cases := []struct {
		name    string
		timeout time.Duration
	}{
		{name: "零值取默认", timeout: 0},
		{name: "负值取默认", timeout: -time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var observed time.Duration
			converter := &Converter{
				Path:    "/fake/ebook-convert",
				Timeout: tc.timeout,
				Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
					if deadline, ok := ctx.Deadline(); ok {
						observed = time.Until(deadline)
					}
					return nil, nil
				},
			}

			// Act
			err := converter.ToMOBI(context.Background(), convertPlanFixture())

			// Assert
			if err != nil {
				t.Fatalf("成功路径不应返回错误：%v", err)
			}
			if observed < 9*time.Minute || observed > 11*time.Minute {
				t.Errorf("默认超时应约为 10m，实际约 %v", observed)
			}
		})
	}
}

func TestConverterToMOBI_TimeoutCancelsRun(t *testing.T) {
	// Arrange
	converter := &Converter{
		Path:    "/fake/ebook-convert",
		Timeout: 30 * time.Millisecond,
		Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			<-ctx.Done()
			return []byte("killed"), ctx.Err()
		},
	}

	// Act
	err := converter.ToMOBI(context.Background(), convertPlanFixture())

	// Assert
	if !errors.Is(err, model.ErrConvert) {
		t.Fatalf("超时应包装 model.ErrConvert，实际：%v", err)
	}
	if !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "context") {
		t.Errorf("错误应说明超时上下文，实际：%v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("应保留 context.DeadlineExceeded 因果链，实际：%v", err)
	}
}

// TestConverterRun_Default 覆盖生产默认执行器（exec.CommandContext + CombinedOutput）。
// 只运行本机 /bin/echo 这类无副作用命令，不涉及网络或外部服务。
func TestConverterRun_Default(t *testing.T) {
	t.Run("运行本地命令并返回 CombinedOutput", func(t *testing.T) {
		// Arrange
		converter := &Converter{}

		// Act
		output, err := converter.run()(context.Background(), "/bin/echo", "hello")

		// Assert
		if err != nil {
			t.Fatalf("默认执行器应能运行本地命令：%v", err)
		}
		if strings.TrimSpace(string(output)) != "hello" {
			t.Errorf("CombinedOutput = %q，期望 %q", output, "hello")
		}
	})

	t.Run("可执行文件不存在时返回错误", func(t *testing.T) {
		// Arrange
		converter := &Converter{}

		// Act
		_, err := converter.run()(context.Background(), "/nonexistent/caixin2kindle-binary")

		// Assert
		if err == nil {
			t.Error("不存在的可执行文件应返回错误")
		}
	})
}

func TestNewConverter(t *testing.T) {
	// Act
	converter := NewConverter(config.Config{OutputProfile: "kindle"})

	// Assert
	if converter == nil {
		t.Fatal("NewConverter 不应返回 nil")
	}
	if converter.Timeout != defaultConvertTimeout {
		t.Errorf("Timeout = %v，期望 %v", converter.Timeout, defaultConvertTimeout)
	}
}

func TestTailOutput(t *testing.T) {
	multiByte := strings.Repeat("中", 2000) // 6000 字节，2000 处落在 rune 中间
	cases := []struct {
		name          string
		output        []byte
		limit         int
		want          string
		wantTruncated bool
	}{
		{name: "空输出", output: nil, limit: 10, want: ""},
		{name: "零上限", output: []byte("abc"), limit: 0, want: ""},
		{name: "短于上限原样返回", output: []byte("hello"), limit: 10, want: "hello"},
		{name: "等于上限原样返回", output: []byte("0123456789"), limit: 10, want: "0123456789"},
		{name: "去除首尾空白", output: []byte("  out  "), limit: 10, want: "out"},
		{name: "超上限保留末尾并标注截断", output: []byte(strings.Repeat("A", 3000) + "TAIL"), limit: 2000, wantTruncated: true},
		{name: "多字节截断不产生非法 UTF-8", output: []byte(multiByte), limit: 2000, wantTruncated: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := tailOutput(tc.output, tc.limit)

			// Assert
			if !tc.wantTruncated {
				if got != tc.want {
					t.Fatalf("tailOutput = %q，期望 %q", got, tc.want)
				}
				return
			}
			if !strings.Contains(got, "截断") {
				t.Errorf("截断结果应含提示字样，实际：%q", got)
			}
			if !utf8.ValidString(got) {
				t.Errorf("截断结果必须是合法 UTF-8，实际：%q", got)
			}
			parts := strings.SplitN(got, "\n", 2)
			if len(parts) != 2 {
				t.Fatalf("截断结果应为“提示行 + 尾部”，实际：%q", got)
			}
			if len(parts[1]) > tc.limit || len(parts[1]) < tc.limit-3 {
				t.Errorf("尾部长度 = %d，期望接近上限 %d", len(parts[1]), tc.limit)
			}
		})
	}
}

func TestTailOutput_KeepsTailMarker(t *testing.T) {
	// Arrange
	output := []byte(strings.Repeat("B", 2500) + "TAILMARKER")

	// Act
	got := tailOutput(output, convertOutputTailLimit)

	// Assert
	if !strings.HasSuffix(got, "TAILMARKER") {
		t.Errorf("应保留输出末尾内容，实际结尾：%q", got[len(got)-20:])
	}
}
