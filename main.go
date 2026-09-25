// Command caixin2kindle 是本项目唯一入口：装配适配器 → app → cli，并把错误映射为退出码。
// 组合根只在此处与 internal/wire，业务层互不引用适配器（architecture.md §3 依赖表）。
package main

import (
	"os"

	"caixin2kindle/internal/adapter/clock"
	"caixin2kindle/internal/adapter/page"
	"caixin2kindle/internal/adapter/publish"
	"caixin2kindle/internal/adapter/storage"
	"caixin2kindle/internal/app"
	"caixin2kindle/internal/cli"
	"caixin2kindle/internal/config"
	"caixin2kindle/internal/selector"
	"caixin2kindle/internal/service/article"
	"caixin2kindle/internal/service/ebook"
	"caixin2kindle/internal/service/issue"
	"caixin2kindle/internal/service/kindle"
)

func main() {
	os.Exit(cli.ExitCode(run()))
}

// run 装配全部依赖并执行一次运行，返回哨兵错误供 cli.ExitCode 映射为 0–5。
//
// 装配必须延迟到参数解析之后：适配器（profile 目录、--out 输出根、--delay 等待区间、
// 浏览器候选）都需要最终 config，因此经由 cli.Deps.NewRuntime 闭包构造（§6.1 步骤 0–1）。
//
// 浏览器必须在进程退出前优雅关闭，否则 profile 会留下 Crashed 标记（§7）。
func run() error {
	reporter := cli.NewReporter(os.Stderr)
	prompter := &cli.Prompter{Out: os.Stderr, In: os.Stdin}

	var browser *page.Client

	err := cli.Run(os.Args[1:], cli.Deps{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		NewRuntime: func(cfg config.Config) cli.Runtime {
			set := selector.Default()
			waiter := clock.NewWaiter(cfg)
			workspace := storage.NewWorkspace(cfg.OutDir)
			stateStore := storage.NewStateStore(workspace)
			stateStore.Warn = reporter.Warn

			browser = page.NewClient(cfg, set, prompter)
			browser.Warn = reporter.Warn

			device := publish.NewUSBDevice(cfg)
			return cli.Runtime{
				App: app.New(app.Deps{
					Reporter:  reporter,
					Opener:    browser,
					Login:     browser,
					Nav:       browser,
					Locator:   issue.NewLocator(set),
					Fetcher:   article.NewFetcher(set, cfg, waiter),
					Waiter:    waiter,
					States:    stateStore,
					Artifacts: workspace,
					EPUB:      ebook.NewBuilder(),
					Converter: publish.NewConverter(cfg),
					Volumes:   device,
					Device:    device,
					Kindle:    kindle.NewSelector(),
				}),
				Checker: publish.NewProbe(cfg),
			}
		},
	})

	if browser != nil {
		browser.Close()
	}
	return err
}
