// Package cli 是表现层：参数、终端交互、日志与退出码；不得碰浏览器与磁盘（§3 依赖表）。
//
// Wave 0 冻结本包与外部的契约：AppRunner / DependencyChecker / Deps / Run / ExitCode，
// 以及 §8 断言清单点名的 Reporter 与 Prompter 类型。其余实现由 Wave 1 的 E 路负责。
package cli

import (
	"context"
	"io"

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

// Deps 是 cli 的依赖，由 main.go 注入。
type Deps struct {
	App     AppRunner
	Checker DependencyChecker
	Stdout  io.Writer // 最终产物路径的渲染目标
	Stderr  io.Writer // 日志唯一出口（脱敏）
}

// Run 解析参数、跑主流程、把 model.Result 渲染到 stdout，返回哨兵错误供 main 映射退出码。
func Run(args []string, deps Deps) error {
	panic("TODO(wave1-E): 见 architecture.md §6.1 步骤 0–1 与 §7")
}
