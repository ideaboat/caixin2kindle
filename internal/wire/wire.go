// Package wire 是装配辅助包：与 main.go 同属组合根，集中放编译期接口断言（§9.1 W7）。
// 它不导出业务行为，故不构成对分层的破坏。
//
// 断言覆盖每个消费方接口与其唯一实现：任一处签名漂移都会让 go build 失败，而不是留到运行时。
package wire

import (
	"caixin2kindle/internal/adapter/clock"
	"caixin2kindle/internal/adapter/page"
	"caixin2kindle/internal/adapter/publish"
	"caixin2kindle/internal/adapter/storage"
	"caixin2kindle/internal/app"
	"caixin2kindle/internal/cli"
	"caixin2kindle/internal/port"
	"caixin2kindle/internal/service/article"
	"caixin2kindle/internal/service/ebook"
	"caixin2kindle/internal/service/issue"
	"caixin2kindle/internal/service/kindle"
)

var (
	// 服务层契约
	_ app.IssueLocator   = (*issue.Locator)(nil)
	_ app.ArticleFetcher = (*article.Fetcher)(nil)
	_ app.EPUBBuilder    = (*ebook.Builder)(nil)
	_ app.KindleSelector = (*kindle.Selector)(nil)
	_ cli.AppRunner      = (*app.App)(nil)

	// 浏览器适配器（同一客户端实现三种能力，注入 app 的 Opener/Login/Nav）
	_ app.BrowserOpener = (*page.Client)(nil)
	_ app.LoginHandler  = (*page.Client)(nil)
	_ port.Navigator    = (*page.Client)(nil)
	_ port.PageSource   = (*page.Client)(nil)

	// 存储与时钟适配器
	_ app.StateRepository = (*storage.StateStore)(nil)
	_ app.ArtifactStore   = (*storage.Workspace)(nil)
	_ port.TextStore      = (*storage.Workspace)(nil)
	_ app.Waiter          = (*clock.Waiter)(nil)
	_ article.ClickWaiter = (*clock.Waiter)(nil)

	// 发布与依赖探测适配器
	_ cli.DependencyChecker = (*publish.Probe)(nil)
	_ app.Converter         = (*publish.Converter)(nil)
	_ app.VolumeScanner     = (*publish.USBDevice)(nil)
	_ app.DeviceWriter      = (*publish.USBDevice)(nil)

	// 表现层实现的中立端口
	_ app.Reporter  = (*cli.Reporter)(nil)
	_ port.Prompter = (*cli.Prompter)(nil)
)
