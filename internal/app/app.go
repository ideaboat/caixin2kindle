// Package app 是应用层：编排用例的顺序与分支，不解析 DOM、不直接碰磁盘（§3 依赖表）。
//
// Wave 0 冻结本包对外的契约：Deps 的字段类型与 App.Run 的签名。
// 主流程实现由 Wave 1 的 F 路负责（先写测试，再写实现）。
package app

import (
	"context"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
	"caixin2kindle/internal/port"
)

// BrowserOpener 启动浏览器（有头/无头、持久化 profile、SingletonLock 与指纹抑制）；由 adapter/page 实现。
type BrowserOpener interface {
	Open(ctx context.Context, cfg config.Config) error
}

// LoginHandler 确保登录态就绪（付费墙不可见、无登录页、无验证码）；不含启动；由 adapter/page 实现。
type LoginHandler interface {
	EnsureReady(ctx context.Context) error
}

// IssueLocator 定位期号与去重后的文章列表；由 issue.Locator 实现。
type IssueLocator interface {
	Locate(ctx context.Context, page port.PageSource, url string) (model.Issue, error)
}

// ArticleFetcher 获取单篇正文；由 article.Fetcher 实现。
type ArticleFetcher interface {
	Fetch(ctx context.Context, nav port.Navigator, art model.Article) (model.ArticleText, error)
}

// Waiter 提供篇间随机等待（编排层职责，spec 3.4）；由 adapter/clock 实现。
type Waiter interface {
	BetweenArticles(ctx context.Context) error
}

// StateRepository 读写 state.json，是进度的唯一来源；由 adapter/storage 实现。
type StateRepository interface {
	Load(ctx context.Context, dir string) (model.State, error)
	Save(ctx context.Context, dir string, s model.State) error
}

// ArtifactStore 读写输出目录内的正文与产物；由 adapter/storage 实现。
type ArtifactStore interface {
	WriteArticleText(path, body string) error
	ReadArticleText(path string) (string, error)
	Exists(path string) (bool, error)
	WriteEPUB(issueDirName string, data []byte) (string, error)
	Path(elem ...string) string
}

// EPUBBuilder 生成 EPUB 字节（不落盘）；由 ebook.Builder 实现。
type EPUBBuilder interface {
	Build(issue model.Issue, texts []model.ArticleText) (name string, data []byte, err error)
}

// Converter 执行 ebook-convert，并把非零退出归类为 model.ErrConvert；由 adapter/publish 实现。
type Converter interface {
	ToMOBI(ctx context.Context, plan model.ConvertPlan) error
}

// VolumeScanner 只读枚举候选挂载卷；由 adapter/publish 实现。
type VolumeScanner interface {
	Volumes(ctx context.Context) ([]model.Volume, error)
}

// DeviceWriter 在目标卷上按需创建 documents/ 并复制文件；由 adapter/publish 实现。
type DeviceWriter interface {
	EnsureDocuments(ctx context.Context, v model.Volume) error
	Copy(ctx context.Context, v model.Volume, src, name string) error
}

// KindleSelector 是 Kindle 挂载点的纯规则；由 kindle.Selector 实现。
type KindleSelector interface {
	Select(req model.KindleRequest, vols []model.Volume) (model.Volume, bool, error)
}

// Deps 汇聚 App 的全部依赖，由 main.go（唯一组合根）注入。
// 适配器以接口注入；纯计算服务（issue/article/ebook/state/kindle）以具体类型满足接口后注入。
type Deps struct {
	Reporter  Reporter
	Opener    BrowserOpener
	Login     LoginHandler
	Locator   IssueLocator
	Fetcher   ArticleFetcher
	Waiter    Waiter
	States    StateRepository
	Artifacts ArtifactStore
	EPUB      EPUBBuilder
	Converter Converter
	Volumes   VolumeScanner
	Device    DeviceWriter
	Kindle    KindleSelector
}

// App 承载一次运行的依赖，实现 cli.AppRunner。
type App struct {
	deps Deps
}

// New 用给定依赖构造 App。
func New(deps Deps) *App {
	return &App{deps: deps}
}

// Run 执行主流程 §6.1 步骤 2–10：开浏览器 → 定位期号 → 增量判定 → 抓取循环
// → 构建前复核与产物生成 → Kindle 定位与拷贝，返回 model.Result 供 cli 渲染。
func (a *App) Run(ctx context.Context, cfg config.Config) (model.Result, error) {
	panic("TODO(wave1-F): 见 architecture.md §6.1")
}
