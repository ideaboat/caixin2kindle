// Package cli 是表现层：参数、终端交互、日志与退出码；不得碰浏览器与磁盘（架构 §3 依赖表）。
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

// AppRunner 由 app.App 实现：跑主流程并返回结果。
type AppRunner interface {
	Run(ctx context.Context, cfg config.Config) (model.Result, error)
}

// DependencyChecker 由 adapter/publish 实现：检查 Chromium 系浏览器与 ebook-convert。
type DependencyChecker interface {
	Check(ctx context.Context) []model.MissingDep
}

// Runtime 汇聚一次运行所需的 App 与依赖检查器，按最终配置构造。
// 适配器需要已解析的 config，因此二者只能在参数解析之后由 NewRuntime 创建。
type Runtime struct {
	App     AppRunner
	Checker DependencyChecker
}

// Deps 是 cli 的依赖，由 main.go 注入。
type Deps struct {
	// NewRuntime 在参数解析后按最终配置构造 App 与依赖检查器；由 main.go 提供闭包，
	// 保证组合根仍在 main（cli 不导入 adapter）。
	NewRuntime func(cfg config.Config) Runtime
	Stdout     io.Writer // 最终产物路径渲染目标
	Stderr     io.Writer // 日志唯一出口（脱敏）
}

// Run 解析参数、按最终配置构造依赖、跑主流程，并把 model.Result 渲染到 stdout。
// 返回值以哨兵错误包装，供 main 经 ExitCode 映射退出码（架构 §6.1 步骤 0–1）。
func Run(args []string, deps Deps) error {
	overrides, help, err := parseFlags(args, deps.Stderr)
	if err != nil {
		printErrorLine(deps.Stderr, err)
		return err
	}
	if help {
		if deps.Stdout != nil {
			fmt.Fprint(deps.Stdout, usageText)
		}
		return nil
	}

	cfg, err := config.Load(overrides, processEnv())
	if err != nil {
		printErrorLine(deps.Stderr, err)
		return err
	}
	if err := cfg.Validate(); err != nil {
		printErrorLine(deps.Stderr, err)
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// NewRuntime 由组合根注入；为空属编程错误，归入依赖缺失以便退出码 1（架构 §9.3 C4）。
	if deps.NewRuntime == nil {
		return fmt.Errorf("%w：cli.Deps.NewRuntime 未注入（编程错误）", model.ErrDependency)
	}
	runtime := deps.NewRuntime(cfg)
	if runtime.Checker == nil {
		return fmt.Errorf("%w：Runtime.Checker 未注入（编程错误）", model.ErrDependency)
	}

	missing := runtime.Checker.Check(ctx)
	if len(missing) > 0 {
		for _, dependency := range missing {
			writeRedacted(deps.Stderr, "依赖缺失：%s：%s", dependency.Name, dependency.Hint)
		}
		return fmt.Errorf("%w：缺少 %d 项外部依赖", model.ErrDependency, len(missing))
	}

	if runtime.App == nil {
		return fmt.Errorf("%w：Runtime.App 未注入（编程错误）", model.ErrDependency)
	}
	result, err := runtime.App.Run(ctx, cfg)
	if err != nil {
		// app 已用哨兵错误包装，直接透传供 ExitCode 映射，不做二次包装。
		return err
	}
	renderResult(deps.Stdout, result, cfg.NoKindle)
	return nil
}

// processEnv 采集 Load 归一化路径所需的进程环境。
// 两个取值失败都只影响 ~ 展开/相对路径解析（由 config.ResolvePath 自行回退），不阻断启动，故置空处理。
func processEnv() config.Env {
	homeDir, homeErr := os.UserHomeDir()
	if homeErr != nil {
		homeDir = ""
	}
	workDir, workErr := os.Getwd()
	if workErr != nil {
		workDir = ""
	}
	return config.Env{HomeDir: homeDir, WorkDir: workDir}
}

// renderResult 把 model.Result 渲染到 stdout：只输出输出目录、EPUB/MOBI 路径与拷贝结论，绝不输出凭据。
// noKindle 为真时说明用户已显式跳过设备检测，不得误报「未检测到设备」。
func renderResult(w io.Writer, result model.Result, noKindle bool) {
	if w == nil {
		return
	}
	if result.Skipped {
		fmt.Fprintln(w, "本期已是最新，跳过抓取与重建。")
	}
	fmt.Fprintf(w, "输出目录：%s\n", result.OutputDir)
	fmt.Fprintf(w, "EPUB：%s\n", result.EPUBPath)
	fmt.Fprintf(w, "MOBI：%s\n", result.MOBIPath)
	renderMissing(w, result)
	renderSkipped(w, result)
	if result.KindleCopied {
		fmt.Fprintln(w, "已拷贝到 Kindle 设备。")
		return
	}
	if noKindle {
		fmt.Fprintln(w, "已按 --no-kindle 跳过设备检测与拷贝。")
		return
	}
	fmt.Fprintf(w, "未检测到设备，成品文件位于 %s，请自行拷入。\n", result.OutputDir)
}

// renderMissing 在降级构建后列出未抓到的篇目，避免用户误以为全书齐全（v1.9.1）。
func renderMissing(w io.Writer, result model.Result) {
	if len(result.MissingArticles) == 0 {
		return
	}
	fmt.Fprintf(w, "未抓取（%d 篇）：%s\n", len(result.MissingArticles), strings.Join(result.MissingArticles, "、"))
}

// renderSkipped 列出按规则有意跳过的篇目（图片型栏目等）并给出说明（v1.9.2）。
func renderSkipped(w io.Writer, result model.Result) {
	if len(result.SkippedArticles) == 0 {
		return
	}
	fmt.Fprintf(w, "已跳过（%d 篇）：%s\n", len(result.SkippedArticles), strings.Join(result.SkippedArticles, "、"))
	if result.SkipNote != "" {
		fmt.Fprintf(w, "说明：%s。\n", result.SkipNote)
	}
}
