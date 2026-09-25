package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

// fakeApp 是实现 AppRunner 的本地假件，记录调用次数与收到的配置。
type fakeApp struct {
	result model.Result
	err    error
	calls  int
	gotCfg config.Config
}

func (f *fakeApp) Run(_ context.Context, cfg config.Config) (model.Result, error) {
	f.calls++
	f.gotCfg = cfg
	return f.result, f.err
}

// fakeChecker 是实现 DependencyChecker 的本地假件。
type fakeChecker struct {
	missing []model.MissingDep
	calls   int
}

func (f *fakeChecker) Check(context.Context) []model.MissingDep {
	f.calls++
	return f.missing
}

// capturedRuntime 记录 NewRuntime 收到的最终配置。
type capturedRuntime struct {
	cfg     *config.Config
	app     *fakeApp
	checker *fakeChecker
}

// depsFor 构造注入假件的 Deps；NewRuntime 捕获最终配置并返回给定 Runtime。
func depsFor(captured *capturedRuntime, stdout, stderr *bytes.Buffer) Deps {
	return Deps{
		NewRuntime: func(cfg config.Config) Runtime {
			captured.cfg = &cfg
			return Runtime{App: captured.app, Checker: captured.checker}
		},
		Stdout: stdout,
		Stderr: stderr,
	}
}

func TestRun_HelpPrintsUsage(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer
	deps := Deps{Stdout: &stdout, Stderr: &stderr}

	// Act
	err := Run([]string{"--help"}, deps)

	// Assert
	if err != nil {
		t.Fatalf("--help 应返回 nil：%v", err)
	}
	if !strings.Contains(stdout.String(), "参数：") || !strings.Contains(stdout.String(), "默认值：") {
		t.Fatalf("--help 应输出完整用法：%q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("--help 不应写 stderr：%q", stderr.String())
	}
}

func TestRun_HelpWithNilStdout(t *testing.T) {
	// Act
	err := Run([]string{"--help"}, Deps{})

	// Assert
	if err != nil {
		t.Fatalf("--help 应返回 nil：%v", err)
	}
}

func TestRun_UsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "缺少位置参数", args: []string{}},
		{name: "多余位置参数", args: []string{testIssueURL, testIssueURL}},
		{name: "未知 flag", args: []string{"--bogus", testIssueURL}},
		{name: "--delay 下界大于上界", args: []string{"--delay", "8-3", testIssueURL}},
		{name: "--delay 非整数", args: []string{"--delay", "abc", testIssueURL}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var stdout, stderr bytes.Buffer
			runtimeCalled := false
			deps := Deps{
				Stdout: &stdout,
				Stderr: &stderr,
				NewRuntime: func(config.Config) Runtime {
					runtimeCalled = true
					return Runtime{}
				},
			}

			// Act
			err := Run(tc.args, deps)

			// Assert
			if !errors.Is(err, model.ErrUsage) {
				t.Fatalf("应返回 ErrUsage，实际：%v", err)
			}
			if ExitCode(err) != 1 {
				t.Fatalf("参数错误退出码应为 1，实际 %d", ExitCode(err))
			}
			if !strings.Contains(stderr.String(), "参数错误") {
				t.Fatalf("stderr 缺少“参数错误”前缀：%q", stderr.String())
			}
			if runtimeCalled {
				t.Fatalf("参数非法时不应构造 Runtime")
			}
		})
	}
}

func TestRun_NilNewRuntime(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer
	deps := Deps{Stdout: &stdout, Stderr: &stderr}

	// Act
	err := Run([]string{testIssueURL}, deps)

	// Assert
	if !errors.Is(err, model.ErrDependency) {
		t.Fatalf("未注入 NewRuntime 应返回 ErrDependency，实际：%v", err)
	}
}

func TestRun_NilRuntimeParts(t *testing.T) {
	cases := []struct {
		name string
		part Runtime
	}{
		{name: "Checker 缺失", part: Runtime{}},
		{name: "App 缺失", part: Runtime{Checker: &fakeChecker{}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			deps := Deps{
				NewRuntime: func(config.Config) Runtime { return tc.part },
				Stdout:     io.Discard,
				Stderr:     io.Discard,
			}

			// Act
			err := Run([]string{testIssueURL}, deps)

			// Assert
			if !errors.Is(err, model.ErrDependency) {
				t.Fatalf("Runtime 缺件应返回 ErrDependency，实际：%v", err)
			}
		})
	}
}

func TestRun_MissingDependency(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer
	application := &fakeApp{}
	checker := &fakeChecker{missing: []model.MissingDep{
		{Name: "Chromium 系浏览器", Hint: "请安装 Chrome / Chromium / Brave / Edge"},
		{Name: "ebook-convert", Hint: "brew install --cask calibre"},
	}}
	captured := &capturedRuntime{app: application, checker: checker}
	deps := depsFor(captured, &stdout, &stderr)

	// Act
	err := Run([]string{testIssueURL}, deps)

	// Assert
	if !errors.Is(err, model.ErrDependency) {
		t.Fatalf("依赖缺失应返回 ErrDependency，实际：%v", err)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("依赖缺失退出码应为 1，实际 %d", ExitCode(err))
	}
	wantLine := "依赖缺失：ebook-convert：brew install --cask calibre"
	if !strings.Contains(stderr.String(), wantLine) {
		t.Fatalf("stderr 缺少依赖提示 %q：%q", wantLine, stderr.String())
	}
	if checker.calls != 1 {
		t.Fatalf("依赖检查应调用 1 次，实际 %d", checker.calls)
	}
	if application.calls != 0 {
		t.Fatalf("依赖缺失时不应跑主流程，实际调用 %d 次", application.calls)
	}
}

func TestRun_SuccessRendersResult(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer
	application := &fakeApp{result: model.Result{
		OutputDir:    "/tmp/out/财新周刊第1224期",
		EPUBPath:     "/tmp/out/财新周刊第1224期/财新周刊第1224期.epub",
		MOBIPath:     "/tmp/out/财新周刊第1224期/财新周刊第1224期.mobi",
		KindleCopied: true,
	}}
	checker := &fakeChecker{}
	captured := &capturedRuntime{app: application, checker: checker}
	deps := depsFor(captured, &stdout, &stderr)

	// Act
	err := Run([]string{"--kindle", "/Volumes/Kindle", "--out", "/tmp/out", testIssueURL}, deps)

	// Assert
	if err != nil {
		t.Fatalf("成功路径不应报错：%v", err)
	}
	if captured.cfg == nil {
		t.Fatalf("NewRuntime 未收到最终配置")
	}
	if captured.cfg.URL != testIssueURL {
		t.Fatalf("URL = %q", captured.cfg.URL)
	}
	if captured.cfg.OutDir != "/tmp/out" {
		t.Fatalf("OutDir = %q", captured.cfg.OutDir)
	}
	if captured.cfg.KindleMount != "/Volumes/Kindle" || !captured.cfg.KindleMountExplicit {
		t.Fatalf("--kindle 应显式覆盖：mount=%q explicit=%v", captured.cfg.KindleMount, captured.cfg.KindleMountExplicit)
	}
	if application.calls != 1 {
		t.Fatalf("主流程应调用 1 次，实际 %d", application.calls)
	}
	got := stdout.String()
	for _, want := range []string{
		"输出目录：/tmp/out/财新周刊第1224期",
		"EPUB：/tmp/out/财新周刊第1224期/财新周刊第1224期.epub",
		"MOBI：/tmp/out/财新周刊第1224期/财新周刊第1224期.mobi",
		"已拷贝",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout 缺少 %q：%q", want, got)
		}
	}
	if strings.Contains(got, "未检测到设备") {
		t.Fatalf("已拷贝时不应提示未检测到设备：%q", got)
	}
}

func TestRun_NotCopiedPromptsManualCopy(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer
	application := &fakeApp{result: model.Result{
		OutputDir: "/tmp/out/财新周刊第1224期",
		EPUBPath:  "/tmp/out/财新周刊第1224期/a.epub",
		MOBIPath:  "/tmp/out/财新周刊第1224期/a.mobi",
		Skipped:   true,
	}}
	captured := &capturedRuntime{app: application, checker: &fakeChecker{}}
	deps := depsFor(captured, &stdout, &stderr)

	// Act
	err := Run([]string{testIssueURL}, deps)

	// Assert
	if err != nil {
		t.Fatalf("成功路径不应报错：%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "未检测到设备，成品文件位于 /tmp/out/财新周刊第1224期，请自行拷入") {
		t.Fatalf("缺少自行拷入提示：%q", got)
	}
	if !strings.Contains(got, "跳过抓取与重建") {
		t.Fatalf("Skipped 应渲染跳过说明：%q", got)
	}
	if captured.cfg == nil || captured.cfg.KindleMountExplicit {
		t.Fatalf("未给 --kindle 时不应标记显式：%+v", captured.cfg)
	}
	if captured.cfg.KindleMount != "/Volumes/Kindle" {
		t.Fatalf("Kindle 默认挂载点 = %q", captured.cfg.KindleMount)
	}
}

func TestRun_FlagOverridesReachConfig(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		check func(t *testing.T, cfg config.Config)
	}{
		{
			name: "显式 --browser",
			args: []string{"--browser", "/opt/chrome", testIssueURL},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.BrowserPath != "/opt/chrome" {
					t.Fatalf("BrowserPath = %q", cfg.BrowserPath)
				}
			},
		},
		{
			name: "布尔开关",
			args: []string{"--full", "--no-kindle", "--headless", testIssueURL},
			check: func(t *testing.T, cfg config.Config) {
				if !cfg.Full || !cfg.NoKindle || !cfg.Headless {
					t.Fatalf("布尔开关未生效：full=%v no-kindle=%v headless=%v", cfg.Full, cfg.NoKindle, cfg.Headless)
				}
			},
		},
		{
			name: "--delay 单值等价 N-N",
			args: []string{"--delay", "5", testIssueURL},
			check: func(t *testing.T, cfg config.Config) {
				if cfg.DelayMin != 5*time.Second || cfg.DelayMax != 5*time.Second {
					t.Fatalf("delay = %v-%v", cfg.DelayMin, cfg.DelayMax)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var stdout, stderr bytes.Buffer
			captured := &capturedRuntime{app: &fakeApp{}, checker: &fakeChecker{}}
			deps := depsFor(captured, &stdout, &stderr)

			// Act
			err := Run(tc.args, deps)

			// Assert
			if err != nil {
				t.Fatalf("运行失败：%v", err)
			}
			if captured.cfg == nil {
				t.Fatalf("NewRuntime 未收到配置")
			}
			tc.check(t, *captured.cfg)
		})
	}
}

func TestRun_AppErrorPassthrough(t *testing.T) {
	// Arrange
	var stdout, stderr bytes.Buffer
	application := &fakeApp{err: fmt.Errorf("第 3 篇失败：%w", model.ErrFetch)}
	captured := &capturedRuntime{app: application, checker: &fakeChecker{}}
	deps := depsFor(captured, &stdout, &stderr)

	// Act
	err := Run([]string{testIssueURL}, deps)

	// Assert
	if !errors.Is(err, model.ErrFetch) {
		t.Fatalf("应透传 ErrFetch，实际：%v", err)
	}
	if ExitCode(err) != 3 {
		t.Fatalf("抓取失败退出码应为 3，实际 %d", ExitCode(err))
	}
	if stdout.Len() != 0 {
		t.Fatalf("失败时不应渲染产物：%q", stdout.String())
	}
}

func TestRenderResult_NilWriter(t *testing.T) {
	// Act / Assert（不 panic 即为通过）
	renderResult(nil, model.Result{OutputDir: "/tmp/out"}, false)
}

// TestRenderResult_NoKindleDoesNotClaimMissingDevice 覆盖 spec 3.9：
// --no-kindle 只是跳过检测与拷贝，输出不得误报「未检测到设备」。
func TestRenderResult_NoKindleDoesNotClaimMissingDevice(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer

	// Act
	renderResult(&stdout, model.Result{OutputDir: "/tmp/out"}, true)

	// Assert
	got := stdout.String()
	if strings.Contains(got, "未检测到设备") {
		t.Fatalf("--no-kindle 时不应提示未检测到设备：%q", got)
	}
	if !strings.Contains(got, "--no-kindle") {
		t.Fatalf("应说明跳过原因：%q", got)
	}
}

// TestRenderResult_ListsMissingArticles 覆盖降级构建：stdout 必须列出未抓到的篇目。
func TestRenderResult_ListsMissingArticles(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	result := model.Result{
		OutputDir:       "/tmp/out/财新周刊第1224期",
		EPUBPath:        "/tmp/out/财新周刊第1224期/a.epub",
		MOBIPath:        "/tmp/out/财新周刊第1224期/a.mobi",
		MissingArticles: []string{"显影｜亲历德国式家庭照护"},
	}

	// Act
	renderResult(&stdout, result, false)

	// Assert
	got := stdout.String()
	if !strings.Contains(got, "未抓取") {
		t.Fatalf("应提示存在未抓取篇目：%q", got)
	}
	if !strings.Contains(got, "显影｜亲历德国式家庭照护") {
		t.Fatalf("应列出未抓取篇目标题：%q", got)
	}
}

// TestRenderResult_ListsSkippedArticles 覆盖有意跳过（图片型栏目）：stdout 必须给出说明。
func TestRenderResult_ListsSkippedArticles(t *testing.T) {
	// Arrange
	var stdout bytes.Buffer
	result := model.Result{
		OutputDir:       "/tmp/out/财新周刊第1224期",
		EPUBPath:        "/tmp/out/财新周刊第1224期/a.epub",
		MOBIPath:        "/tmp/out/财新周刊第1224期/a.mobi",
		SkippedArticles: []string{"第 21 篇 显影｜亲历德国式家庭照护"},
		SkipNote:        "图片型栏目以图片为主，纯文字 EPUB 无法承载",
	}

	// Act
	renderResult(&stdout, result, true)

	// Assert
	got := stdout.String()
	for _, want := range []string{"已跳过", "第 21 篇 显影｜亲历德国式家庭照护", "图片型栏目"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout 缺少 %q：%q", want, got)
		}
	}
	if strings.Contains(got, "未抓取") {
		t.Fatalf("有意跳过不应报成未抓取：%q", got)
	}
}
