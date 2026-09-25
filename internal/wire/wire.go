// Package wire 是装配辅助包：与 main.go 同属组合根，集中放编译期接口断言（§9.1 W7）。
// 它不导出业务行为，故不构成对分层的破坏。
//
// Wave 0 先钉住服务层与 app/cli 之间的契约；适配器落地后在同一处追加断言（见文末 TODO）。
package wire

import (
	"caixin2kindle/internal/app"
	"caixin2kindle/internal/cli"
	"caixin2kindle/internal/service/article"
	"caixin2kindle/internal/service/ebook"
	"caixin2kindle/internal/service/issue"
	"caixin2kindle/internal/service/kindle"
)

// 编译期断言：任一处签名漂移都会让 go build 失败，而不是留到运行时。
var (
	_ app.IssueLocator   = (*issue.Locator)(nil)
	_ app.ArticleFetcher = (*article.Fetcher)(nil)
	_ app.EPUBBuilder    = (*ebook.Builder)(nil)
	_ app.KindleSelector = (*kindle.Selector)(nil)
	_ cli.AppRunner      = (*app.App)(nil)
)

// TODO(wave1): 适配器与 cli 实现落地后追加（全部指向 port / app 的消费方接口）：
//
//	var _ port.Navigator        = (*page.Client)(nil)
//	var _ port.PageSource       = (*page.Client)(nil)
//	var _ port.Prompter         = (*cli.Prompter)(nil)
//	var _ port.TextStore        = (*storage.ArticleText)(nil)
//	var _ app.Reporter          = (*cli.Reporter)(nil)
//	var _ cli.DependencyChecker = (*publish.Probe)(nil)
