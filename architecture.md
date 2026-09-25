# caixin2kindle 软件架构文档

版本 1.0 · 对应需求 `spec.md` · 遵循 `go-rules/architecture.md` v0.2

---

## 1. 范围与原则

### 1.1 目标

把《财新周刊》某一期的网页地址，变为 Kindle 里的 EPUB + MOBI：抓取当期全部文章 → 组装 EPUB → 转换 MOBI → 检测 USB Kindle 并拷贝。

### 1.2 不可动摇的约束

| 约束 | 值 | 架构影响 |
|---|---|---|
| 语言 / 平台 | Go / macOS | 路径、挂载点按 macOS 语义 |
| 并发模型 | 单线程，按目录顺序逐篇 | 无 goroutine 编排；`context` 仅用于链路超时 |
| 浏览器 | chromedp，默认有头 | 自动化层允许人工接管 |
| 认证 | 持久化 profile（`~/.caixin/browser-profile`，0700）+ 首次手动登录 | 登录态属状态，必须落存储层 |
| 正文 | 纯文字，无图片 | EPUB 生成器无需资源打包 |
| 配置 | **不加配置文件、不加环境变量** | 与规范"配置外置"冲突，见 §9.1 |
| 合规 | 单账号、低频、不分发、不调内部 API | 所有翻页必须模拟真实点击 |

### 1.3 规范原则的落地方式

- **单一职责**：一包一个变更原因；单文件 ≤ 300 行（预计最大 `service/issue/parse.go` 约 200 行）。
- **分层单向**：`cli → app → service → {model, config}`；适配器只被 `main.go` 装配，见 §3。
- **契约**：全部跨模块调用经 interface，且**接口定义在消费方包内**；适配器只导出结构体构造器，靠 Go 隐式实现满足接口（accept interfaces, return structs）。
- **无状态**：service 层为纯函数/纯对象，随机源、时钟、等待器均由参数注入；状态只存在于 `state.json` 与 `articles/`。
- **配置外置**：所有可调量集中于 `internal/config`，由 CLI 参数覆盖，禁止散落硬编码（[WARN] 见 §9.1）。

---

## 2. 架构决策

| # | 决策 | 理由 | 被否方案 |
|---|---|---|---|
| D1 | 正文抓取拆为**解析（纯） + 导航（浏览器）**两阶段 | 把"点哪个、点几次"变成可单测的纯逻辑；代价是 service 需感知少量 `PageAction` 数据 | 在 chromedp 回调里直接抽文本：不可测、不可回放 |
| D2 | 浏览器接口按**能力**拆分（Navigator / LoginHandler / Publisher），不定义"抓财新"式业务接口 | 接口由消费方定义，适配器保持通用，fake 可整包复用 | 单个 `Crawler` 大接口：任何测试都要实现全部方法 |
| D3 | 导航循环为 `HTML → Plan 出一个 PageAction → 执行 → 重取 HTML`，单篇点击合计上限 50 次 | 单一动作返回值即可表达"展开优先、再翻页"的优先级，两种机制并存也不丢页；点击天然可计数，且与 spec 3.7-5 的合计口径一致 | 一次返回多个动作：顺序语义含糊；在 adapter 内循环：业务逻辑不可测 |
| D4 | 解析一律以 `testdata/` HTML 快照为输入 | spec 3.5/6 明确 selector 不得靠猜；快照即回归 fixture | 真实联网测试：违反测试规范（禁调线上付费接口） |
| D5 | 进度只信 `state.json`，内容哈希基于**提取后纯文本** | spec 4.1 明确"不靠数文章数"；原始 HTML 含动态注入会误判变化 | 以产物文件存在性推断进度：无法表达"抓了一半" |
| D6 | 输出按期号子目录隔离 | spec 4.1：同一 `--out` 不同期互不干扰，天然免冲突处理 | 输出目录加锁文件：引入无用复杂度 |
| D7 | 定时参数与随机源可注入 | 兼顾"确定性/可测"与"拟人化随机等待" | 全局 `rand`/`time.Sleep`：核心逻辑不可复现 |
| D8 | selector 放中立包 `internal/selector`（纯数据） | 解析方与执行方需读同一张表，而分层禁止 `service → adapter`；中立包同时满足两者且只有一处定义（spec 3.5-2） | selector 留在 adapter 由 service 生成"空 selector"指令：同一语义两处维护，易漂移 |

---

## 3. 分层与目录结构

```
caixin-download/
├── main.go                      # 唯一组装根：装配适配器 → app → cli，映射退出码
├── internal/
│   ├── cli/                     # 表现层：参数、终端交互、日志、退出码
│   │   ├── cli.go               # Run(args, deps) error
│   │   ├── flags.go             # 参数定义/校验/Usage（含"请勿操作自动化窗口"提示）
│   │   ├── prompt.go            # 登录/验证码/设备提示，读取用户回车
│   │   ├── log.go               # 进度日志“第 3/32 篇：<标题>”，stderr，统一脱敏
│   │   └── exit.go              # 哨兵 error → 退出码 0-5
│   ├── app/                     # 应用层：用例编排（顺序与分支）
│   │   ├── app.go               # Run(ctx, cfg) 主流程
│   │   ├── acquire.go           # 单篇获取 + 重试 + 登录失效恢复
│   │   └── publish.go           # 重建判定 → EPUB → MOBI → Kindle 拷贝
│   ├── service/                 # 业务层：纯逻辑，只依赖 model/config/selector 与消费方接口
│   │   ├── issue/               # 期号与文章列表
│   │   │   ├── parse.go         # 给定 issue 页 HTML → 期号 + 文章列表 + 去重
│   │   │   ├── period.go        # “第 N 期”匹配与目录名 sanitize
│   │   │   └── scroll.go        # 列表稳定的终止条件
│   │   ├── article/             # 正文获取（纯解析）
│   │   │   ├── navigate.go      # 探测机制 → PageAction 序列（expand 优先于 nextpage）
│   │   │   ├── accumulate.go    # 段落级拼接与去重（H5）
│   │   │   ├── extract.go       # 单页 HTML → 标题/作者/正文段落；内容元素处理见 §4.2
│   │   │   └── verify.go        # 文末特征符号 / 付费提示判定
│   │   ├── ebook/               # 纯生成逻辑（不落盘、不起进程）
│   │   │   ├── epub.go          # Issue + []ArticleText → EPUB 字节 + 文件名
│   │   │   └── command.go       # ebook-convert 参数构建（纯函数，不执行）
│   │   ├── kindle/              # 设备定位纯规则
│   │   │   └── rule.go          # 候选卷排序/筛选 + 是否需要创建 documents/ 的判定
│   │   └── state/               # 进度判定（纯函数）
│   │       ├── decide.go        # 待抓列表、是否重建（H6）
│   │       └── hash.go          # 正文纯文本哈希
│   ├── model/                   # 领域数据与哨兵错误，无行为、无依赖
│   │   ├── issue.go             # Issue / Article
│   │   ├── page.go              # PageAction / ActionKind / Step / NavProgress
│   │   ├── text.go              # ArticleText / Status
│   │   ├── state.go             # State / ArticleState / Artifact
│   │   └── errors.go            # ErrUsage / ErrDependency / ErrLogin / ErrFetch / ErrConvert / ErrCopy
│   ├── selector/                # 中立包：selector 纯数据表，service 与 adapter 共享（spec 3.5-2）
│   │   ├── selector.go          # SelectorSet 结构与默认表
│   │   ├── issue.go             # 列表容器、文章链接、期号文案
│   │   └── article.go           # 标题/作者/正文/余下全文/下一页/页脚/付费提示
│   ├── config/                  # 配置外置：默认值、解析、校验（[WARN] §9.1）
│   │   └── config.go
│   ├── wire/                    # 装配辅助：构造各部件 + 集中编译期接口断言（§9.1 W7）
│   │   └── wire.go
│   ├── adapter/                 # 数据/外部世界层
│   │   ├── page/                # chromedp：Navigator / PageSource / LoginHandler 实现
│   │   │   ├── open.go          # 启动 Chrome（有头/无头、持久化 profile、SingletonLock）
│   │   │   ├── navigate.go      # 导航、滚动、点击、等待网络空闲/元素可见
│   │   │   ├── execute.go       # PageAction → chromedp 动作；点击后登出检测（M10）
│   │   │   ├── login.go         # 打开浏览器、登录/验证码探测与提示桥接
│   │   │   ├── prompt.go        # 消费方接口 Prompter（由 cli/prompt 隐式实现）
│   │   │   └── checks.go        # 登录页/验证码的 DOM 特征（adapter 私有，不进 selector 表）
│   │   ├── storage/             # 文件与状态
│   │   │   ├── workspace.go     # 输出目录布局、创建、路径规范化
│   │   │   ├── statestore.go    # state.json 原子读写（唯一进度来源）
│   │   │   └── articletext.go   # articles/ 纯文本读写
│   │   ├── publish/             # 外部发布能力
│   │   │   ├── calibre.go       # 执行 ebook-convert + 退出码/错误分类（非零 → ErrConvert）
│   │   │   ├── kindle_usb.go    # /Volumes 扫描（只读）、documents/ 创建、复制
│   │   │   └── probe.go         # Chrome / ebook-convert 探测（含 macOS 回退路径）
│   │   └── clock/               # 时钟与随机等待的实现（可 fake）
│   │       └── waiter.go
│   └── testutil/                # 测试支撑
│       ├── fakebrowser.go       # 按脚本回放快照的 Navigator/PageSource/LoginHandler
│       ├── fakeconv.go          # Converter / VolumeScanner / DeviceWriter / Waiter / Clock
│       └── goldens/             # 期望产物
└── testdata/                    # 三类样例页面 HTML 快照（issue / 余下全文 / 下一页）
```

**依赖方向**（编译期强制，无环）：

| 包 | 允许导入 |
|---|---|
| `cli` | `app`, `config`, `model`（不得碰 adapter、不得碰浏览器/磁盘） |
| `app` | `service/*`, `model`, `config` |
| `service/*` | `model`, `config`, `selector`, 自身消费方接口 |
| `adapter/*` | `model`, `config`, `selector`, 第三方库（**不得**反向导入 `service`、`app`、`cli`） |
| `main.go` | 全部（唯一装配点；编译期断言集中在 `internal/wire`，`main.go` 只调用） |

> 反向实现靠 Go 隐式接口：`page.Client` 方法签名满足 `service/article.Navigator`，`adapter` 无需 import `service`，依赖箭头不倒流。
> `selector` 是**无依赖的纯数据包**（选项：① 纯数据中立包，已采用；② `PageAction` 只带 `ActionKind`、由 `adapter` 执行时查表填 selector）。选 ① 的原因：`service` 的解析需要读按钮/容器锚点，spec 3.5-2 又要求 selector 只有一处定义，中立包同时满足两者，且不引入"同一语义两处定义"的漂移风险。

---

## 4. 核心数据模型（`internal/model`）

```go
type Issue struct {
    ID       string    // 期号，如 “财新周刊第1234期”；解析失败时为 slug “2026-cw1224”
    DirName  string    // 已 sanitize 的目录名
    URL      string
    Articles []Article // 已去重，保持目录顺序
}

type Article struct {
    Order      int    // 目录顺序，从 1 开始
    Title      string
    Author     string
    URL        string // 原文
    Normalized string // 去尾 “/” 与 query，用于去重与状态匹配
}

type PageAction struct { // service → 浏览器 的唯一指令载体
    Kind     ActionKind    // ActionExpandFullText | ActionNextPage | ActionScrollToBottom
    Selector string        // 取自 internal/selector；service 与 adapter 读取同一张表
    MaxWait  time.Duration // 元素可见/网络空闲的等待上限
}

// NavProgress 是单篇导航的完整可审计记录：状态机每一步都可回放
type NavProgress struct {
    Steps        []Step // 按发生顺序
    PageIndex    int    // 已进入的正文页序号，从 1 开始
    ClicksOnArticle int    // 单篇 Expand+NextPage 合计点击数（上限 50）
    LastArchivedPage int   // 已归档到第几页（0 表示尚未归档任何页）
}

type Step struct { // StepKind: Navigate | Expand | NextPage | Scroll | Snapshot | Done | Fail
    Kind        StepKind
    PageIndex   int
    Selector    string
    BodyHash    string // 终态快照后该页正文哈希，用于诊断
    Err         string
}

type ArticleText struct { // extract/accumulate 的输出，也是 EPUBBuilder 的入参（M8）
    Order   int      // 目录顺序
    Title   string   // 仅从第 1 页取一次
    Author  string   // 仅从第 1 页取一次
    Paragraphs []string // 段落级去重后的有序正文
}

func (t ArticleText) Body() string // 段落以空行连接，供哈希与落盘
```

`State` 落盘结构（`.caixin2kindle/state.json`）：

```json
{
  "schema_version": 1,
  "issue_id": "财新周刊第1234期",
  "articles": [
    {"order":1,"url":"...","normalized_url":"...","title":"...",
     "status":"success","text_path":"articles/001-xxx.txt","hash":"sha256:..."}
  ],
  "artifacts": {"epub":"...epub","mobi":"...mobi","built_issue_id":"财新周刊第1234期"}
}
```

不变式与写盘约定：
- `schema_version` 不匹配 → 视为无状态（走全量），不做迁移；`json.Decoder.DisallowUnknownFields`，损坏 JSON 同样降级为全量并告警。
- **原子写**（M2）：`statestore.Save` 写 `state.json.tmp` → `fsync` → `os.Rename` 覆盖 → `fsync` 目录；任何时刻磁盘上的 `state.json` 要么是旧的完整版本、要么是新的完整版本。
- `artifacts.built_issue_id` 只在 **EPUB 与 MOBI 都成功产出后**写入，是"产物已构建"的唯一标记（见 §7 H6 重建判定）。
- 每篇抓取成功后立即 `Save`，因此进程被杀也只丢当前这一篇的进度。

### 4.1 期号与目录名的 sanitize 规则（M13）

`service/issue/period.go` 的 `Sanitize(name string) string` 按顺序执行，供目录名与文件名共用：

1. Unicode NFC 归一化（macOS 对文件名做 NFD 分解，不归一化会导致同名不同字节、增量匹配失败）。
2. 替换文件系统非法/易歧义字符为 `_`：`/ \ : * ? " < > |`、ASCII 控制字符 `\x00-\x1f\x7f`、换行与制表符。
3. 折叠连续空白与 `_` 为单个，去首尾空白与 `.`。
4. 丢弃尾部 `.`（Windows/macOS 兼容习惯），拒绝空串、`.`、`..` → 回退到 URL slug。
5. 截断到 120 字节（按 rune 边界截断，不切断 UTF-8）。
6. slug 回退规则：取 URL 路径中含年份与期号的段拼连，如 `weekly.caixin.com/2026/cw1224/` → `2026-cw1224`。
7. macOS 默认卷不区分大小写：同卷内出现仅大小写不同的目录名视为同一目录，由"期号唯一"前提保证不冲突，不做额外处理。

### 4.2 正文元素处理（M11，对应"纯文字、不含图片"）

| 元素 | 处理 |
|---|---|
| `<img>` / `<picture>` / `<source>` / `<svg>` | 整节点剔除；图片容器若因此变空，则移除该空容器 |
| 视频/音频/iframe 占位 | 整节点剔除 |
| `<figcaption>` / 图说文本 | 默认**保留为纯文本段落**并冠以 `图：` 前缀（属文字内容，spec 未排除）。**待 `testdata/` 快照复核**：若真实页面正文无图注，本条自然失效；若需求方认为图注属"非正文"，改为整节点剔除只需改 `extract.go` 一处。该待验证项不阻塞编码（详见 §8 末段） |
| 页脚、推荐位、广告、相关阅读 | 按 `internal/selector` 中的剔除锚点整块移除 |
| 付费提示、未展开预览文案 | 不剔除，交给 `verify` 判定该篇为失败（保留原文便于排查） |
| `<br>` | 转段落分隔；连续空段落折叠 |
| 链接 | 保留文字、丢弃 href（纯文字阅读不需要外链） |

---

## 5. 模块契约

接口**全部定义在消费方包**，跨包参数只用 `model` 类型或消费方自定义接口（不得出现 `adapter` 的具体类型）。下表即接口清单与归属。

| 消费方包 | 接口 | 方法签名要点 | 由谁实现 |
|---|---|---|---|
| `cli` | `DependencyChecker` | `Check(ctx) []model.MissingDep` | `adapter/publish` |
| `cli` | `AppRunner` | `Run(ctx, cfg config.Config) error` | `app` |
| `app` | `BrowserOpener` | `Open(ctx, cfg config.Config) error` | `adapter/page` |
| `app` | `LoginHandler` | `EnsureReady(ctx) error`（登录/验证码已就绪；不含启动） | `adapter/page` |
| `app` | `IssueLocator` | `Locate(ctx, page PageSource, url string) (model.Issue, error)` | `service/issue` |
| `app` | `PageSource` | `Navigate`、`HTML`、`ScrollToBottom`、`WaitNetworkIdle`、`WaitVisible` | `adapter/page` |
| `app` | `ArticleFetcher` | `Fetch(ctx, nav Navigator, art model.Article) (model.ArticleText, error)` | `service/article` |
| `app` | `Navigator` | `Navigate`、`HTML`、`Execute(model.PageAction)`、`LogoutDetected` | `adapter/page` |
| `app` | `Waiter` | `BetweenArticles(ctx)`（篇间等待，编排层职责） | `adapter/clock` |
| `app` | `StateRepository` | `Load(ctx, dir) (model.State, error)`；`Save(ctx, dir, s model.State) error` | `adapter/storage` |
| `app` | `ArtifactStore` | `WriteArticleText`、`ReadArticleText`、`Exists`、`WriteEPUB(name, data)`、`Path` | `adapter/storage` |
| `app` | `EPUBBuilder` | `Build(issue model.Issue, texts []model.ArticleText) (name string, data []byte, error)`（**不落盘**） | `service/ebook` |
| `app` | `Converter` | `ToMOBI(ctx, plan model.ConvertPlan) error` | `adapter/publish`（calibre） |
| `app` | `VolumeScanner` | `Volumes(ctx) ([]model.Volume, error)`（只读枚举 `/Volumes/*` 及其 `documents/`） | `adapter/publish` |
| `app` | `DeviceWriter` | `EnsureDocuments(ctx, v model.Volume) error`；`Copy(ctx, v model.Volume, src, name string) error` | `adapter/publish` |
| `app` | `KindleSelector` | `Select(mount string, vols []model.Volume) (model.Volume, bool, error)`（纯规则） | `service/kindle` |
| `service/issue` | `PageSource` | 同 `app.PageSource`（两个消费方各自声明所需子集） | `adapter/page` |
| `service/article` | `Navigator` | 同 `app.Navigator` | `adapter/page` |
| `service/article` | `ClickWaiter` | `BetweenClicks(ctx)`（篇内点击间等待） | `adapter/clock` |
| `adapter/page` | `Prompter` | `WaitForLogin(ctx)`、`WaitForCaptcha(ctx)`（由 adapter 自身消费） | `cli/prompt` |

`model` 中新增的跨包类型（避免 `app` 导入 `adapter`）：`MissingDep{Name, Hint string}`、`Volume{Path string, HasDocuments bool, Writable bool}`、`ConvertPlan{InputPath, OutputPath, OutputProfile string; Args []string}`。

约定：
- 所有方法首参 `context.Context`，由 `cli` 建立根 context（链路超时来自 `config`），库内不得新建 `context.Background()`。
- 返回结构化错误而非日志：`model.ErrUsage/ErrDependency/ErrLogin/ErrFetch/ErrConvert/ErrCopy` 用 `%w` 包装，`cli/exit.go` 用 `errors.Is` 映射退出码（**唯一映射点**，见 §6.1）。
- nil receiver / 未检测到设备等"正常但空"的结果用 `(T, bool)` 表达，不用 error；只有 `--kindle` 指定的挂载点创建/写入失败才用 error。
- 跨包传递只读结构体（`model.*`）；`app` 的进度状态是本地值，每篇后整体落盘。
- 日志统一走 `cli/log.go`（**stderr**，`Warn/Info/Error` 三级 + 计时），`stdout` 只留给 `Prompter` 的必要交互与最终产物路径汇总（M12）。
- 服务层不落盘、不起进程：`EPUBBuilder` 返回字节、`service/ebook.Command` 只产出参数，写文件与执行分别由 `ArtifactStore`、`Converter` 完成。

---

## 6. 关键流程

### 6.1 主流程（`app.Run`）

```
0. cli 解析参数 → config.Load(默认值) → config.Validate
   （URL 非法 / --delay 非法 → ErrUsage → 退出 1，日志明确标注“参数错误”以区别依赖缺失，见 §9.3）
1. cli 调 DependencyChecker：缺 Chrome / ebook-convert → 打印安装提示，退出 1
2. app 调 BrowserOpener.Open（有头 + 持久化 profile）；profile 被占用 → 退出 1
3. app 调 LoginHandler.EnsureReady（首轮登录/验证码）→ IssueLocator.Locate：
   Navigate(issueURL) → 滚动至列表稳定（连续两次滚动数量不增）
   → 解析期号与文章列表 → 按 normalized_url 去重      ← 到此才知道期号（H1）
4. 工作区创建：<out>/<Issue.DirName>/{,.caixin2kindle/,articles/}
   （期号未知前不建目录；此步只依赖 Locate 的产物）
5. state.Decide：读取该目录 state.json，产出待抓列表
   （success 且正文文件存在、hash 未变 → 跳过）
6. 抓取循环：对每篇 ArticleFetcher.Fetch（见 6.2），
   先写正文并 fsync，再原子写 state（顺序见 §7 审查意见 16）；
   每篇后 Save(state)（崩溃可从断点续跑，且不依赖文件计数）
7. 连续 3 篇失败 → ErrFetch，退出 3
8. 产物循环（见 §7「构建前复核」）：
   a. 重建判定：不满足“可跳过”条件 → EPUBBuilder.Build → ArtifactStore.WriteEPUB → Converter.ToMOBI
   b. 若正文复核发现损坏/缺失 → 回步骤 6 只重抓这些篇（上限 2 轮），随后回到 a
   c. 仍不合格 → ErrFetch，退出 3
9. 未指定 --no-kindle → VolumeScanner + KindleSelector 定位设备；
   命中 → EnsureDocuments（必要时创建）→ 覆盖同名文件拷贝（失败退出 5）；
   未命中 → 提示路径，退出 0（EPUB/MOBI 仍在步骤 8 产出，见审查意见 5）
```

**登录/验证码的职责边界（M1）**：DOM 探测与终端交互归 `adapter/page`（它看得见 DOM），编排与恢复归 `app`。

| 环节 | 由谁做 | 内容 |
|---|---|---|
| 打开浏览器 | `app` 调 `BrowserOpener.Open` | 启动 Chrome（有头/无头、持久化 profile、SingletonLock 检查） |
| 首轮认证 | `app` 调 `LoginHandler.EnsureReady` | 探测登录页/验证码 → 经 `Prompter` 提示 → 轮询等待列表元素就绪（带超时） |
| 提示渲染 | `cli/prompt` 实现 `adapter/page.Prompter` | 接口定义在 `adapter/page`（消费方），`cli` 只提供实现并注入 → `adapter` 不导入 `cli`（审查意见 6） |
| issue 页解析 | `app` 调 `IssueLocator.Locate` | 纯解析：只读已就绪页面的 HTML，不感知登录 |
| 运行中被登出（4.4） | `adapter/page` 返回 `model.ErrLogin`，`app` 捕获 | `Navigate` 与每次 `Execute` 之后都探测登出（M10）→ 回到 `EnsureReady` |
| 超时 | `app` 负责整体上限，`adapter` 负责单次等待上限 | 任一层超时 ⇒ `ErrLogin` ⇒ 退出 2 |
| 退出码映射 | `cli/exit.go` 是**唯一**映射点；`main.go` 只调用 `cli.ExitCode(err)` | 避免两处各自映射导致漂移（审查意见 14） |

`--headless` 时（M6）：`EnsureReady` 检测到登录页或验证码**立即**返回 `ErrLogin`，不轮询、不等待人工操作，日志提示“无头模式无法人工介入，请去掉 --headless 后重试”，退出 2。

### 6.2 单篇获取（`article.Fetch`，对应 spec 3.7）

**两种机制的已确认行为（来自需求方一手说明，待 `testdata/` 快照复核）**：

| 机制 | 行为 | 对设计的影响 |
|---|---|---|
| "余下全文" | **点一次即展开整篇正文**；正常情况下一次点击后即可命中文末特征符号 | 展开后立即用 `verify.Complete` 判定，命中即结束，不做无意义连点 |
| "下一页" | 与"余下全文"**并列**；选择翻页路线时必须**一直点，直到没有下一页**，才能读完全文 | 逐页归档终态内容；以"下一页按钮消失"作为结束条件 |

因此 `Plan` 的优先级为：**本页存在"余下全文" → 先展开（一次通常即完整）；展开后若仍存在才继续点；正文已完整则直接结束；仅当"余下全文"不存在（或已点无可点）时，才走"下一页"路线。**

注意这里与"两条路线互斥"的旧表述不同：**并存页面同样被支持**——若展开后正文仍未命中标识，且页面存在"下一页"，会继续翻页处理（见 3f 分支）；只是按已确认行为，这种情况正常不会出现。`Plan` 因此表达的是 spec 3.7 的"对存在的机制分别处理"，而不是二选一。

**核心不变式**：`Plan` 每次只返回**一个**最高优先级动作；**归档只发生在离开某一页时**（翻页前或整篇结束时），且只归档当前终态快照。

```
1. Navigator.Navigate(url)：等待网络空闲 → 快照 pageHTML
   若 LogoutDetected → 返回 ErrLogin
2. Step.Navigate 记入 NavProgress；PageIndex = 1；LastArchivedPage = 0
3. 循环（ClicksOnArticle < 50，TotalSteps < 200）：
   a. 早退检查：verify.Complete(extract.Body(pageHTML)) 为真 → 跳出
      （覆盖“余下全文单击即完整”的正常路径；跳出后由步骤 4 归档）
   b. action, done := Plan(pageHTML)
      优先级 1：存在“余下全文”按钮   → ActionExpandFullText
      优先级 2：存在“下一页”按钮     → ActionNextPage
      无更高优先级动作                → done
   c. done → 跳出
   d. Waiter.BetweenClicks()（1–3s 随机）
   e. Navigator.Execute(action)；Execute 内部保证：
      - 元素可见 → 模拟真实点击 → 等待网络空闲或目标元素就绪（不是固定 sleep）
      - 点击后探测登出 → 返回 ErrLogin
   f. action.Kind == ActionNextPage：
        accumulate.Append(extract.Body(pageHTML))      // 步骤①：归档“当前页”终态
        LastArchivedPage = PageIndex
        if verify.Complete(extract.Body(pageHTML)) → 跳出   // 该页即全文结尾
        重新快照 pageHTML                                // 步骤②：此后 pageHTML 才是下一页
        if Plan(pageHTML) 已无任何动作 → 跳出             // 末页结束
        PageIndex++
      action.Kind == ActionExpandFullText：
        ClicksOnArticle++；重新快照 pageHTML              // 原地展开，不换页、不归档
4. 收尾归档（无条件执行，覆盖早退 / done / 末页三种出口）：
   if PageIndex > LastArchivedPage → accumulate.Append(extract.Body(pageHTML))
5. extract.Title/Author 仅从 PageIndex == 1 的快照取一次
6. 返回 ArticleText{Order, Title, Author, Paragraphs}
```

**归档规则（消除"丢第一页 / 丢展开后正文"两类隐患）**：用 `LastArchivedPage` 记录已归档到第几页，**归档与快照的先后顺序写死在步骤 3f**——先归档当前 `pageHTML`（此时仍是原页终态），再重新快照。每一页的终态内容恰好在"离开该页"时被追加一次；步骤 4 覆盖所有出口，保证最后一页不会漏。

**为什么这样能保证不重不漏（H5）**：

| 风险 | 对策 |
|---|---|
| 点击"下一页"后第一页正文丢失 | 步骤 3f **先归档、后重新快照**：归档的 `pageHTML` 仍是原页终态，快照更新发生在其后 |
| 原地展开的中间态被反复拼接 | 展开动作不换页、不推进 `LastArchivedPage`，只替换 `pageHTML`；同一页只会在离开时归档一次 |
| "余下全文"单击即完整时正文丢失 | 早退出口同样经过步骤 4 的归档（3a→4；`PageIndex > LastArchivedPage` 成立，故会归档） |
| 末页正文丢失 | 3f 的"下一页之后无动作 → 跳出"与 `done` 出口都经步骤 4 归档 |
| 同一页被归档两次 | 归档后立即置 `LastArchivedPage = PageIndex`，步骤 4 只归档"尚未归档的当前页"；`accumulate` 的段落级去重是第二道保险 |
| 误在已完整的正文上继续点击 | 步骤 3a 每轮先做完成判定，命中即结束 |
| 段落级重复（页脚、连载重复段） | `accumulate` 对段落归一化（折叠空白、去首尾）后哈希去重，保持首次出现顺序，重复归档也不会产生重复段落 |
| 按钮探测不稳（点击后按钮不消失） | 不依赖"按钮消失"作为唯一信号：`verify.Complete` 优先，点击上限兜底 |
| 死循环 | 全篇点击上限 50 次（spec 3.7-5），另设总步数上限 200 兜底 |

若某页已满足 `verify.Complete`（文末符号出现、无付费提示），直接判为完整，剩余按钮不再点击。

**点击计数（H7 + 审查意见 1）**：`ActionExpandFullText` 与 `ActionNextPage` **合计**上限 **50 次**，与 spec 3.7-5 一致（此前按页 50 次是错的，会允许单篇累计 200 次）；`ActionScrollToBottom` 不计入；`TotalSteps < 200` 仅作为兜底闸，不得放宽 50 次硬上限。`NavProgress.Steps` 记录实际步数以便审计与复现。

### 6.3 重试与中断（`app/acquire.go`）

```
attempt 1..3：
  失败 → 记录 → 若 attempt < 3 则固定退避 2s、4s 后重试
  若错误是 ErrLogin → 不计入重试次数，直接进入登录恢复流程（恢复成功后重试当前篇）
任一篇成功后连续失败计数归零；连续 3 篇最终失败 → 停止并输出排查提示
（登录态是否失效 / 网络是否正常 / 页面结构是否变更）
```

---

## 7. 可靠性设计

| 需求 | 设计 | 落点 |
|---|---|---|
| 增量（4.1） | 唯一进度依据 `state.json`；success **且** `text_path` 存在 **且** 内容哈希匹配才跳过；哈希对**提取后正文**计算 | `service/state/decide.go`、`adapter/storage/*` |
| 正文文件校验（M2） | `Decide` 阶段做存在性/可读性检查（保持快）；**构建 EPUB 前**逐篇重读并复核哈希，不符即回到抓取阶段重抓（流程回环见 §6.1 步骤 8，审查意见 12），绝不用损坏正文生成 EPUB | `service/state/decide.go`、`app/publish.go` |
| 状态写盘（M2） | 临时文件 + `fsync` + `rename` + 目录 `fsync`；解码失败或 schema 不匹配 → 全量并告警 | `adapter/storage/statestore.go` |
| 写入顺序（审查意见 16） | 单篇成功时**先写 `articles/*.txt` 并 fsync，再原子写 state**；顺序反了会在崩溃后出现"state 说 success、正文缺失" | `app/acquire.go`、`adapter/storage/*` |
| 期号隔离（4.1） | 输出目录按期号命名，state 与产物同目录，天然隔离 | `adapter/storage/workspace.go` |
| 重建判定（4.1，修订 H6 + 审查意见 5） | 仅当 **① 全部文章 success ② `artifacts.built_issue_id == Issue.ID` ③ EPUB 与 MOBI 均存在** 三者同时成立才跳过。`--no-kindle` **不参与**该判定：该模式仍须产出 EPUB + MOBI，只是不拷贝（spec 2/3.9），原条件"`--no-kindle` 时只查 EPUB"是错的 | `service/state/decide.go` |
| `--full`（4.2 + 审查意见 17） | 删除 `.caixin2kindle/state.json` 与 `articles/*.txt`、旧的 `<期号>.epub`/`.mobi`（**只删本工具在 `<输出目录>` 内生成的已知产物，不做目录级 `RemoveAll`**），然后按全量待抓列表执行 | `app/app.go`、`adapter/storage/workspace.go` |
| 失败重试（4.3） | 单篇最多 3 次尝试，固定退避 2s/4s；`ErrLogin` 不计入尝试次数 | `app/acquire.go` |
| 连败熔断（4.3） | 连续 3 篇失败即停，附排查提示 | `app/acquire.go` |
| 中途登出（4.4） | `Navigate` 与每次 `Execute`（含点击、翻页、滚动）后探测登出特征（M10）；命中即返回 `ErrLogin`，由 `app` 走登录恢复 | `adapter/page/execute.go`、`app/acquire.go` |
| 超时兜底（3.2-3） | 所有等待均带 `config` 超时，默认值见下；超时按失败处理 | `config`、`adapter/page/navigate.go` |
| 拟人化（3.4） | 篇间 `--delay` 区间（默认 3–8s）、点击间 1–3s 随机；等待条件为"网络空闲/元素就绪"而非固定 sleep | `adapter/clock/waiter.go` |
| 依赖检查（3.1） | 启动即检 Chrome 与 `ebook-convert`，缺失退出 1 | `adapter/publish/probe.go` |
| 依赖探测路径（M9） | Chrome：`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome` → `exec.LookPath("google-chrome")`；`ebook-convert`：`exec.LookPath` → `/Applications/calibre.app/Contents/MacOS/ebook-convert` | `adapter/publish/probe.go` |
| 凭据安全（3.1） | 日志只输出标题/进度/路径，**stderr** 唯一出口，统一脱敏 | `cli/log.go` |
| 复制语义（3.9） | 同名覆盖；指定挂载点缺 `documents/` 时自动创建（C2）；失败非 0 退出 | `adapter/publish/kindle_usb.go` |
| 服务层不碰 IO（审查意见 8/9） | `service/ebook` 只产出 EPUB 字节与转换参数；写文件归 `ArtifactStore`，起进程与错误分类归 `Converter` | `service/ebook/*`、`adapter/storage`、`adapter/publish` |

**构建前复核的流程回环（审查意见 12）**：读取与哈希复核是纯判断（`service/state`），执行重抓需要浏览器，因此回环由 `app/publish.go` 编排：

```
for buildRound in 1..2:
    bad := state.VerifyTexts(issue, state, store)   // 纯判定：缺失/哈希不符的篇
    if len(bad) > 0:
        app.acquire(bad)                            // 回到抓取流程，复用 §6.3 的重试策略
        continue
    epub := epubBuilder.Build(issue, texts)         // 纯生成，返回字节
    store.WriteEPUB(issue.DirName, epub)
    converter.ToMOBI(ctx, plan)                     // 唯一的进程执行点
    state.MarkBuilt(issue.ID); store.Save(state)
    break
否则 → ErrFetch，退出 3
```

**Kindle 定位的组合方式（审查意见 7）**：`service/kindle` 只做纯规则（给定 `--kindle` 与候选卷列表，选出目标卷），不读盘；读盘由 `adapter/publish.VolumeScanner` 负责，创建目录与复制由 `DeviceWriter` 负责。三者都由 `app` 装配调用：

```
vols := scanner.Volumes(ctx)                    // model.Volume{Path, HasDocuments, Writable}
vol, found, err := selector.Select(cfg.KindleMount, vols)   // 纯规则，含“指定挂载点”优先
if err != nil → ErrCopy，退出 5                  // 指定挂载点不可写等
if !found → 提示路径，退出 0
app 调 writer.EnsureDocuments(ctx, vol)         // 仅在“显式指定”的卷上创建（C2）
app 调 writer.Copy(ctx, vol, mobiPath, name)    // 覆盖同名；失败 → ErrCopy，退出 5
```

因此 `adapter/publish` **不需要导入 `service/kindle`**，依赖表不被破坏；`service/kindle` 的消费方是 `app`。

**`config` 默认值表（M4/M5/L3/L4，全部可被 CLI 参数覆盖，仅 `--out/--delay/--kindle/--headless/--no-kindle/--full` 对外暴露）**：

| 项 | 默认 | 校验规则 |
|---|---|---|
| `DelayMin/Max` | 3s / 8s | 解析 `"M-N"`；`M`、`N` 为非负整数且 `0 ≤ M ≤ N`；单值 `"5"` 合法等价 `[5,5]`；`"8-3"`、负数、非整数、缺上界 → `ErrUsage` 退出 1 |
| `ClickDelayMin/Max`（篇内点击间） | 1s / 3s | 同上（当前不对 CLI 暴露） |
| `LoginPollTimeout` | 300s | 轮询等待文章列表元素 |
| `ElementVisibleTimeout` | 30s | 单次元素可见等待 |
| `NetworkIdleTimeout` | 60s | 单次网络空闲等待 |
| `NavigateTimeout` | 90s | 单次页面导航 |
| `ArticleClickLimit` | 50 | **单篇 Expand + NextPage 合计**上限，spec 3.7-5（审查意见 1：此前误写为按页 50） |
| `ArticleTotalSteps` | 200 | 单篇动作总步数兜底闸，不得替代 50 次硬上限 |
| `RetryBackoff` | 2s, 4s | spec 4.3 |
| `ConsecutiveFailureLimit` | 3 | spec 4.3 |
| `BuildRetryRounds` | 2 | 构建前复核发现损坏时，最多回抓轮数（审查意见 12） |
| `OutputProfile` | `kindle` | 传给 `ebook-convert --output-profile`；待 KF7/KF8 确认后只需改此处（L4） |
| `BrowserProfileDir` | `~/.caixin/browser-profile` | 创建时 `0700` |
| `KindleMount` | `/Volumes/Kindle` | 见下 |
| `DocumentsDirName` | `documents` | 仅在显式指定的挂载点上缺失时创建（C2） |
| `EnableRemoteDebug` | `false` | chromedp 调试端口仅在排查时开启；默认不暴露端口 |
| `OutDir` | `~/Downloads/caixin` | 路径规范化：先 `~` 展开，相对路径相对**当前工作目录**，再 `filepath.Clean`（L3） |
| 人工确认（验证码/登录） | **无超时** | 唯一例外，理由见 §9.2 |

> `--delay "5"`（单值）为架构扩展，允许但属 spec 未列出的超集：`--help` 中显式写明"单值等价 `N-N`"，并建议同步补入 spec（审查意见 19）。

**Kindle 挂载点判定（M7 / C2 已定案）**：

1. `--kindle` 给出且存在：`documents/` 不存在则**创建**该目录（`MkdirAll`，0755），随后照常拷贝；创建失败或后续拷贝失败 → 退出 5。
2. `--kindle` 未给出：扫描 `/Volumes/*`，取第一个含 `documents/` 的卷；**扫描路径不创建目录**，无 `documents/` 的卷直接跳过（避免在任意 U 盘上新建目录）。
3. 均无 → 未检测到设备：提示成品路径，退出 0。
4. 中途挂载点消失、复制失败 → 退出 5。

> 即：**只在用户显式指定的挂载点上创建 `documents/`**；自动扫描阶段保持只读探测。

**退出码**（`cli/exit.go` 单一映射表）：

| 码 | 含义 | 触发源 |
|---|---|---|
| 0 | 成功（含未检测到 Kindle 的自行拷入提示） | 正常结束 |
| 1 | 参数错误 / 依赖缺失 / 浏览器启动失败（含 profile 占用） | `ErrUsage`、`ErrDependency` |
| 2 | 登录失败或超时（含 `--headless` 下遇到登录/验证码） | `ErrLogin` |
| 3 | 抓取失败（含连败熔断、单篇点击超限、全篇步数超限） | `ErrFetch` |
| 4 | EPUB/MOBI 转换失败 | `ErrConvert` |
| 5 | Kindle 复制失败（含指定挂载点创建/写入失败） | `ErrCopy` |

> C1 已定案：profile 被占用归 **1**（以 spec 4.5 为准；spec 3.1 已同步修订为"退出码 1"）。
> C4 已定案：参数错误与依赖缺失**共用 1**，spec 4.5 表格不动；实现侧以日志首行前缀强制区分（约定见 §6.1 步骤 0 与 §5 约定）。

---

## 8. 测试策略（对齐 `go-rules/testing.md`）

| 层次 | 做法 |
|---|---|
| 单元（纯逻辑，覆盖 ≥ 80%） | `issue`（去重、期号匹配、sanitize、滚动终止）、`article`（`Plan` 优先级顺序、**余下全文单击即完整时的早退**、**下一页逐页点到消失**、段落去重、成功判定、**单篇点击合计 50 次上限**、200 步兜底）、`state`（跳过/重建判定、哈希、`built_issue_id` 与产物存在性组合）、`kindle`（挂载点优先级、指定挂载点建目录、扫描阶段不创建）、`ebook`（EPUB 结构与元数据、转换参数构建）、`config`（`--delay` 解析边界、各超时默认值、路径规范化）——全部表驱动，输入为 `testdata/` 快照 |
| 集成（跨两层） | `app` + `service` + `adapter` 的 fake：`fakebrowser` 按脚本回放快照与动作，`fakeconv` 记录调用。覆盖：增量重跑只抓未完成篇、正文文件缺失/哈希不符时**回抓后重建**（M2 + 审查意见 12）、转换失败后重跑**不跳过**重建（H6）、**`--no-kindle` 下 MOBI 缺失仍重建**（审查意见 5）、余下全文路线与下一页路线各一篇的端到端拼接（H5）、`--full` 全量、连败熔断、未检测到 Kindle 时退出 0 |
| 契约 | 每个消费方接口在 `testutil` 提供 fake；编译期断言**集中且仅**放在 `internal/wire`（`main.go` 只调用装配，不再自称断言处，审查意见 15），以免 adapter 反向导入 service（H4）：`var _ article.Navigator = (*page.Client)(nil)`、`var _ app.PageSource = (*page.Client)(nil)`、`var _ page.Prompter = cli.Prompter{}` |
| 约束 | 单测禁真实网络/线上接口；不用 `time.Sleep` 做同步；`go test ./...` 一键跑通；产物比对放 `testutil/goldens/` |

日志断言（L2）：进度行分母必须是**去重后**文章数，即 `第 3/32 篇：<标题>` 中的 32 等于 `len(Issue.Articles)`；用 `cli/log.go` 的注入 writer 做断言，不读真实 stderr。

对照 `testdata/` 前置产物（spec 6）：issue 页快照、"余下全文"篇、"下一页"篇。快照同时用于确认**文末特征符号、付费提示文案、期号真实格式**——三者以 `verify.go`/`period.go` 中的可替换规则表达，便于快照到位后一次校正。

**待 `testdata/` 快照复核项汇总（均为"看一眼真实页面即可定"，不阻塞编码）**：

| # | 待确认内容 | 现状（默认选择） | 复核后可能要改的地方 |
|---|---|---|---|
| 1 | 文末特征符号的具体形态 | 《待快照》以规则表达 | `service/article/verify.go` |
| 2 | 付费提示 / 未展开预览文案特征 | 同上 | `service/article/verify.go` |
| 3 | 期号文案真实格式（"第 N 期"变体） | 宽松正则匹配 | `service/issue/period.go` |
| 4 | "余下全文"单击即完整、"下一页"需点到消失 | 已按需求方说明实现（§6.2） | `service/article/navigate.go` |
| 5 | 正文是否存在图说（`<figcaption>`）及其归属 | 保留为文本并加 `图：` 前缀（§4.2） | `service/article/extract.go` |
| 6 | 列表/标题/作者/按钮的具体锚点 | 集中在 `internal/selector` | `internal/selector/*.go` |

以上 6 项都在 `testdata/` 快照到位后一次性定稿，其中 1–3、6 是 spec 6 已列出的前置任务，4–5 是本架构在缺乏快照时所作的默认选择。

---

## 9. 规范自检

### 9.1 [WARN] 违反项与理由

| # | 规范条款 | 冲突点 | 处理与理由 |
|---|---|---|---|
| W1 | 配置外置（环境变量/配置文件） | spec 明确"不加配置文件、不加环境变量" | 以 CLI 参数 + `internal/config` 内集中默认值实现"注入"：调用方仍是唯一配置来源，可调量不散落。若规范优先，改动面仅 `cli/flags.go` 与 `config.go` |
| W2 | 配置外置 | 超时、退避 2s/4s、点击上限 50、delay 区间等为代码常量 | 全部集中在 `internal/config` 单点定义，可测试覆盖；不上环境变量是 spec 的明确取舍 |
| W3 | 契约 | `app` 从 `main.go` 接收具体服务对象（构造注入），未为纯计算服务再套接口；适配器导出的结构体（`page.Client` 等）本身不是接口 | 返回结构体、由消费方在自己的包内定义所需接口，是规范要求的方向；`app` 对纯计算服务无替换需求，不再套壳（YAGNI）。跨层与外部依赖处均已用接口 |
| W4 | 单文件 ≤ 300 行 | 以下文件有超限风险，实现时须盯住（审查意见 21） | 拆分方案：`internal/selector` 按机制分片且只放纯数据；`adapter/page/navigate.go` 按"导航/等待/执行"拆；`app/app.go` 把抓取与发布分别下沉到 `acquire.go`/`publish.go`（目录结构已按此预留）；`service/article/navigate.go` 把 `Plan` 与步进状态机分开；`service/issue/parse.go` 把列表解析与期号解析分开。超限者在文件头标注理由 |
| W5 | 无状态 | 等待时长由随机源决定，非确定性 | 随机源与时钟以参数注入，`service` 层对给定输入仍确定；非确定性仅存在于 `adapter/clock` |
| W6 | 分层单向 | 当前仅 CLI 一个入口，表现层与组装职责边界较薄 | 分层仍显式保留：`cli` 只做参数/渲染，`main.go` 独占装配，便于后续加入其他入口 |
| W7 | 分层单向 | `internal/wire` 为了放编译期断言，需同时导入 `service/*` 与 `adapter/*` | `wire` 是**装配包**，与 `main.go` 同性质（组合根），不在业务分层链上；它不导出业务行为，故不构成对分层的破坏 |

### 9.2 人工确认不设超时的例外

`spec 3.2-3` 要求"所有等待必须设超时上限"，而验证码/登录的**人工确认**按定义需要人参与，无法预估时长。处理：**机器等待全部设超时**（浏览器元素、网络空闲、导航、登录轮询），**只有"等待用户按回车"这一处不设超时**，且该处输出明确提示；`--headless` 下直接跳过该路径并失败退出 2（§6.1）。这是对 spec 3.2-3 的有意细化，不是遗漏。

### 9.3 需求文档内部冲突与取舍（C1-C4）

以下冲突来自 `spec.md` 自身，架构必须选边并记录来源；**建议需求方后续统一 spec 文案**。

| # | 冲突 | 采用 | 来源与理由 |
|---|---|---|---|
| C1 | profile 被占用：spec 3.1 说退出码 **2**，spec 4.5 把"浏览器启动失败（含 profile 被占用）"归为退出码 **1** | **1（已定案并已修 spec）** | 需求方确认以 4.5 为准；spec 3.1 已同步改为"退出码 1"，原"2"作废 |
| C2 | spec 3.9 说"--kindle 指定挂载点存在 → 使用之"，未定义缺 `documents/` 的行为 | **自动创建 `documents/` 后拷贝（已定案）** | 需求方确认；仅在显式指定的挂载点上创建，自动扫描阶段不创建（见 §7） |
| C3 | spec 3.1 要求检 Chrome 与 `ebook-convert`，未定义 macOS 上不在 PATH 时的行为 | **PATH + macOS 绝对路径回退（已定案）** | 需求方确认方案 A；理由：只查 PATH 会把已安装的 Chrome 判为缺失（macOS 上 Chrome 通常不在 PATH）；全盘扫描因慢且不确定被否 |
| C4 | 参数错误无退出码 | **沿用 1，日志前缀区分（已定案）** | 需求方确认方案 A：零 spec 改动，符合 4.5 既有口径；代价是脚本不能仅凭退出码区分参数错误与依赖缺失 |

**spec 待改条目**：C1 对应的一处已修订完成（spec 3.1："退出码 2" → "退出码 1"）。C2/C3 属 spec 未定义的行为补齐，不改 spec 原文；C4 定案为沿用 1，spec 4.5 表格无需改动。

**C3 决策依据（依赖探测路径）**

事实：macOS 上 Chrome 装在 `/Applications/Google Chrome.app`，其可执行文件**不在 PATH**；calibre 的 `ebook-convert` 视安装方式可能写入 `/usr/local/bin`（在 PATH）也可能只在 app bundle 内。因此"只用 `exec.LookPath`"会把**已安装**判成缺失，直接违反 spec 3.1 的初衷（给出安装提示）。

已采纳方案 A，其余方案记录备查：

- **A（已采纳）**：`Chrome`：先试 `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`，再 `LookPath("google-chrome")`；`ebook-convert`：先 `LookPath("ebook-convert")`，再 `/Applications/calibre.app/Contents/MacOS/ebook-convert`。命中任一即视为已安装。代价：`probe.go` 多两个候选路径常量（进 `config`），无新增依赖、无安全面。
- **B（未采纳）**：只查 PATH。优点最简；代价是绝大多数 macOS 用户会被判"未安装 Chrome"，需要手动改 PATH 或建软链，可用性明显变差。
- **C（未采纳）**：PATH + 全盘扫描（如 `mdfind`/`locate`）。代价：启动变慢、结果不确定，且引入了新的外部命令依赖——与"参数最少、开箱即用"相悖。

**C4 决策依据（参数错误退出码）**

两种诉求不同：人看日志，用 1 足够；脚本判退出码，才需要区分。当前 spec 4.5 只定义 0–5，扩码需要动 spec。

已采纳方案 A，其余方案记录备查：

- **A（已采纳）**：沿用 **1**，但日志首行强制前缀区分：`参数错误：…` / `依赖缺失：…` / `浏览器启动失败：…`。优点：零 spec 改动，符合 spec 4.5 既有口径；代价：脚本无法仅凭退出码区分"用户敲错参数"与"环境缺依赖"。
- **B（未采纳）**：新增退出码 **6**（仅参数错误），并同步改 spec 4.5 表与本架构 §7 表。优点：退出码语义完整，对自动化更友好；代价：spec 与实现的映射表都要改，且 0–5 的既有验收用例需补一条。
- **C（未采纳）**：把参数错误并入 2（登录）。语义完全不相关，会让排查更混乱。

### 9.4 已满足项

单一职责（一包一职责）、分层单向（依赖表见 §3，无环）、接口由消费方定义（§5）、`accept interfaces, return structs`（适配器只导出构造器）、状态集中于存储层（`state.json` + `articles/`）、错误用 `%w` + `errors.Is` 判断、上下文贯穿链路并带超时、日志不输出凭据。

---

## 10. 需求覆盖对照

| spec 章节 | 架构落点 |
|---|---|
| 1.1 目的（5 步流程） | §6.1 主流程步骤 3-9 |
| 1.2 用户场景 | §6.1（一次运行完成） |
| 1.3 约束与前提 | §1.2、§2、§3 |
| 1.4 合规边界 | §1.2 合规行；D3 禁内部 API（仅模拟点击） |
| 2 CLI 参数（URL、`--out`、`--no-kindle`、`--full`、`--kindle`、`--headless`、`--delay`） | `cli/flags.go` + `config`；Usage 含"勿操作窗口"提示与"`--delay` 单值等价 `N-N`"说明；`--no-kindle` **只跳过拷贝**，仍产出 EPUB + MOBI |
| 3.1 依赖检查与启动（退出码、profile 0700、stderr 日志） | `adapter/publish/probe.go`、`adapter/page/open.go`、`cli/log.go` |
| 3.2 认证三层信号 | §6.1 职责表；`adapter/page/login.go` + `cli/prompt.go` + §7 超时兜底 |
| 3.3 浏览器模式 | `adapter/page/open.go`（headless 开关）；headless 下登录/验证码策略见 §6.1（M6） |
| 3.4 反爬与拟人化 | `adapter/clock/waiter.go` + `adapter/page/execute.go` |
| 3.5 selector 策略 | `internal/selector/{selector,issue,article}.go` 唯一定义 + `testdata/` 快照 |
| 3.6 列表解析（滚动稳定、去重、期号） | `service/issue/parse.go`、`period.go`、`scroll.go` |
| 3.7 全文获取（两机制、拼接、50 次上限、成功判定） | `service/article/{navigate,accumulate,extract,verify}.go`（§6.2） |
| 3.8 EPUB 组装（目录名、回退 slug、sanitize、元数据、输出结构） | `service/issue/period.go`、`service/ebook/epub.go`、`adapter/storage/workspace.go` |
| 3.9 转换与 Kindle 拷贝（不提前检测、三级探测、覆盖、未检测退出 0） | `service/ebook/command.go`、`service/kindle/rule.go`、`adapter/publish/*`；挂载点边界见 §7（C2） |
| 4.1 增量与状态文件 | `service/state/*`、`adapter/storage/*`（原子写与正文校验见 §7） |
| 4.2 `--full` | `app/app.go` |
| 4.3 失败重试与连败熔断 | `app/acquire.go` |
| 4.4 中途登录失效 | `adapter/page/{login,execute}.go`、`app/acquire.go` |
| 4.5 退出码与日志格式 | `cli/exit.go`、`cli/log.go`（stderr；分母为去重后篇数） |
| 5 非目标 | §1.2/§2 明确排除；无对应模块 |
| 6 待验证事项 | §8 末段 + `testdata/` 前置；KF7/KF8 型号确认只影响 `config.OutputProfile`（L4） |
| 7 验收标准 1-7 | §8 集成用例逐一对应；其中"每篇正文完整"以**文末固定标识**判定，并覆盖两条路线：`余下全文` 单击即完整、`下一页` 逐页点到按钮消失 |
| go-rules 架构规范 | §1.3、§5、§9.1、§9.4 |
