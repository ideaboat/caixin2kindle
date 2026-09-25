# caixin2kindle 软件架构文档

版本 1.8 · 对应需求 `spec.md` · 遵循 `go-rules/architecture.md` v0.2

| 版本 | 变更摘要 |
|---|---|
| 1.0 | 初版：分层、接口、流程、可靠性、测试、规范自检 |
| 1.1 | 按首轮审查修订（H1-H7、M1-M13、L1-L5）：selector 中立包、conflict 表、config 默认值表 |
| 1.2 | 余下全文单击即完整；Kindle 自动建 `documents/`；C1 以 spec 4.5 为准 |
| 1.3 | C2/C3/C4 定案；spec 3.1 退出码同步 |
| 1.4 | 按次轮审查修订（16 条）：单篇点击合计 50、归档游标、`--no-kindle` 重建、接口归属与类型 |
| 1.4.1 | 图说归属降级为待快照复核；汇总 6 项待复核清单 |
| 1.5 | 按第三轮审查修订（16 条）：新增中立端口包 `port`（修复跨包接口签名不兼容）、末页归档、最终成功判定、构建回环轮数、`app.Reporter`、登录恢复上限、增量语义澄清等 |
| 1.6 | 按 `testdata/` 快照定稿（2026-09-25）：成功判据由"文末特征符号"改为三条可验证的 DOM 事实；图说归属定稿，新增 AI 注入文本与分页锚点剔除；**首轮认证改判在文章页**（期号页实测不设门槛）；期号文案、页面锚点、两种机制的实际行为定案 |
| 1.7 | 新增 §8.1 selector 定稿表（唯一定义处）：期号页/文章页/剔除清单/页面事实逐条标注已定稿（✅）或待第二样本复核（🔶）；定稿两项决策——HTML 解析库 `goquery`（D9）、导语 `div#subhead` 剔除（A4）；修正 §10 中文末判据的过期表述（文末标识 → 三条 DOM 事实） |
| 1.8 | 吸收 `caixin-snapshot` 采集器实测结论（需求方 2026-09-25 逐条批准）：① 浏览器探测纳入 Chromium 系并新增 `--browser`（spec 1.3/2/3.1 同步）；② 新增自动化指纹抑制要求；③ 新增浏览器优雅关闭与 `exit_type` 归一化；④ 页面就绪判据改为以 DOM 稳定为主、`WaitNetworkIdle` 降级并存；另补 §4 `model.PageFacts`（§6.2 步骤 6 的入参缺口）与 §9.1 W9（JS 点击兜底的 [WARN]） |
| 1.9 | Wave 1 实现期契约修订（均为实现中暴露的缺口，见 §9.1 W10）：① `config.Config` 增 `URL`（唯一位置参数的载体，`Run(ctx, cfg)` 无处接收 URL）；② `State.ArticleState` 增 `author`（EPUB 每章要署名，重跑只读本地正文，无法再回页面取）；③ `ArtifactStore` 增 `EnsureWorkspace` / `CleanWorkspace`（§6.1 步骤 4 与 `--full` 清理无处落点）；④ `app.Deps` 增 `Nav port.Navigator`（`Locate`/`Fetch` 消费同一浏览器对象）；⑤ `cli.Deps` 改为 `NewRuntime(cfg)` 工厂（修 §9.1 W10 的装配时序矛盾） |
| 1.9.1 | 首次真机实测（2026-09-25，第 1224 期 24 篇）后的修正：① **点击后等待改以 DOM 指纹稳定为准、上限 8s**，导航后的网络空闲二次等待同样压到 8s——财新页面有长轮询，真正的「无在途请求」不出现，原设计复用 `NetworkIdleTimeout`(60s) 导致每次点击空等 1 分钟（实测频繁出现）；② 文章页标题/署名节点缺失**不再判整篇失败**，回退到列表页条目信息，并新增「空正文」防线（三条 DOM 事实全过但无段落同样判失败）；③ **降级构建**：构建前回抓额度用尽后仍有篇目不可用时，用成功篇目产出 EPUB/MOBI、把缺失篇目录入警告与 stdout 清单，不再整期白跑（需求方 2026-09-25 确认；一篇可用正文都没有时仍返回 `ErrFetch`）；④ CLI 用户可见前缀 `告警：` 改为 `警告：`。纯逻辑可单测，浏览器相关部分仍由真机复测覆盖（§8） |
| 1.9.2 | 需求方定案：**图片型栏目（标题以「显影」开头）直接放弃抓取**——这类报道以图片为主、正文容器与文字稿不同，纯文字 EPUB 无法承载；为它堆叠容器退化规则不划算（需求、设计与简洁的平衡）。落点：`service/article.SkipReason`（唯一规则处，只看列表标题前缀）、新增 `StatusSkipped` 状态，`Decide`/`VerifyTexts`/`NeedRebuild` 均视 skipped 为「有意缺席」（不抓取、不算失败、不计入缺失、不阻塞重建），`app.markSkipped` 首次登记并在每次运行输出说明，`Result.SkippedArticles/SkipNote` 由 cli 渲染为「已跳过（N 篇）…」 |

> **引用约定**：正文中的"审查意见 N"指**第三轮**审查（本版本）；早期两轮的编号一律写作"意见(二) N"或 H/M/L 系列 ID。

---

## 1. 范围与原则

### 1.1 目标

把《财新周刊》某一期的网页地址，变为 Kindle 里的 EPUB + MOBI：抓取当期全部文章 → 组装 EPUB → 转换 MOBI → 检测 USB Kindle 并拷贝。

### 1.2 不可动摇的约束

| 约束 | 值 | 架构影响 |
|---|---|---|
| 语言 / 平台 | Go / macOS | 路径、挂载点按 macOS 语义 |
| 并发模型 | 单线程，按目录顺序逐篇 | 无 goroutine 编排；`context` 仅用于链路超时 |
| 浏览器 | chromedp v0.16.0，默认有头；支持 Chrome / Canary / Chromium / Brave / Edge，可 `--browser` 显式指定 | 自动化层允许人工接管；启动参数必须抑制自动化指纹、退出必须优雅关闭（见 §7） |
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
| D9 | HTML 解析/选择器库用 `github.com/PuerkitoBio/goquery` | 它是 `x/net/html` + `cascadia` 的封装：取段落、剔节点、按 DOM 顺序拼正文的代码量最小，`extract.go` 最容易守住单文件 ≤300 行（W4）；selector 仍以纯字符串存于 `internal/selector`，仅 `service/article/extract.go` 一处消费 | 直接用 `x/net/html` + `cascadia`：等价能力但多 1.5–2 倍样板，`extract.go` 需再拆 `dom.go`。两者都不做 CSS 层叠，可见性一律读 inline `style`（§8.1 F2） |

---

## 3. 分层与目录结构

```
caixin2kindle/
├── main.go                      # 唯一组装根：装配适配器 → app → cli，映射退出码
├── internal/
│   ├── cli/                     # 表现层：参数、终端交互、日志、退出码
│   │   ├── cli.go               # Run(args, deps) error；渲染 Result 到 stdout
│   │   ├── flags.go             # 参数定义/校验/Usage（含"请勿操作自动化窗口"提示）
│   │   ├── prompt.go            # 实现 port.Prompter（登录/验证码/设备提示，读回车）
│   │   ├── reporter.go          # 实现 app.Reporter：进度行、告警、排查提示（stderr，脱敏）
│   │   ├── log.go               # 日志底座：writer 注入、级别、计时
│   │   └── exit.go              # 哨兵 error → 退出码 0-5（唯一映射点）
│   ├── port/                    # 中立端口包：被多个消费方共享的接口（§5，审查意见 1）
│   │   ├── page.go              # PageSource / Navigator（浏览器能力）
│   │   ├── prompt.go            # Prompter（人机交互）
│   │   └── text.go              # TextStore（正文文件读取）
│   ├── app/                     # 应用层：用例编排（顺序与分支）
│   │   ├── app.go               # Run(ctx, cfg) (model.Result, error) 主流程
│   │   ├── acquire.go           # 单篇获取 + 重试 + 登录失效恢复
│   │   ├── publish.go           # 构建前复核循环 → EPUB → MOBI → Kindle 拷贝
│   │   └── report.go            # 消费方接口 Reporter；model.Result 由 model 定义
│   ├── service/                 # 业务层：纯逻辑，只依赖 model/config/selector/port
│   │   ├── issue/               # 期号与文章列表
│   │   │   ├── parse.go         # 给定 issue 页 HTML → 期号 + 文章列表 + 去重
│   │   │   ├── period.go        # “第 N 期”匹配与目录名 sanitize
│   │   │   ├── scroll.go        # 列表稳定的终止条件
│   │   │   └── locator.go       # Locator：导航 + 滚动至稳定 + 解析（Wave 0 冻结的跨包契约）
│   │   ├── article/             # 正文获取（纯解析）
│   │   │   ├── navigate.go      # 探测机制 → PageAction 序列（expand 优先于 nextpage）
│   │   │   ├── accumulate.go    # 段落级拼接与去重（H5）
│   │   │   ├── extract.go       # 单页 HTML → 标题/作者/正文段落；内容元素处理见 §4.2
│   │   │   ├── verify.go        # 成功判定：无付费墙 / 按钮穷尽 / 小节齐全（§6.2 步骤 6）
│   │   │   └── fetch.go         # Fetcher：单篇导航状态机（Wave 0 冻结的跨包契约）
│   │   ├── ebook/               # 纯生成逻辑（不落盘、不起进程）
│   │   │   ├── epub.go          # Issue + []ArticleText → EPUB 字节 + 文件名
│   │   │   └── command.go       # ebook-convert 参数构建（纯函数，不执行）
│   │   ├── kindle/              # 设备定位纯规则
│   │   │   └── rule.go          # 候选卷排序/筛选 + 是否需要创建 documents/ 的判定
│   │   └── state/               # 进度判定（纯函数）
│   │       ├── decide.go        # 待抓列表、是否重建（H6）、构建前复核
│   │       └── hash.go          # 正文纯文本哈希
│   ├── model/                   # 领域数据与哨兵错误，无行为、无依赖
│   │   ├── issue.go             # Issue / Article
│   │   ├── page.go              # PageAction / ActionKind / Step / NavProgress
│   │   ├── text.go              # ArticleText / Status
│   │   ├── state.go             # State / ArticleState / Artifact
│   │   ├── external.go          # MissingDep / Volume / ConvertPlan
│   │   ├── result.go            # Result（app.Run 的返回值，供 cli 渲染）
│   │   └── errors.go            # ErrUsage / ErrDependency / ErrLogin / ErrFetch / ErrConvert / ErrCopy
│   ├── selector/                # 中立包：selector 纯数据表，service 与 adapter 共享（spec 3.5-2）
│   │   ├── selector.go          # SelectorSet 结构与默认表
│   │   ├── issue.go             # 列表容器、文章链接、期号文案
│   │   ├── article.go           # 标题/作者/正文/图说/余下全文/下一页/页脚/付费墙/注入文本剔除
│   │   └── login.go             # 登录页表单、验证码元素（审查意见 11：一并集中）
│   ├── config/                  # 配置外置：默认值、解析、校验（[WARN] §9.1）
│   │   └── config.go
│   ├── wire/                    # 装配辅助：构造各部件 + 集中编译期接口断言（§9.1 W7）
│   │   └── wire.go
│   ├── adapter/                 # 数据/外部世界层
│   │   ├── page/                # chromedp：Navigator / PageSource / LoginHandler 实现
│   │   │   ├── open.go          # 浏览器三级定位、启动（有头/无头、持久化 profile、SingletonLock、指纹抑制）、优雅关闭与 exit_type 归一化
│   │   │   ├── navigate.go      # 导航、滚动、点击、等待 DOM 稳定/元素可见（网络空闲降级保留）
│   │   │   ├── execute.go       # PageAction → chromedp 动作；点击后登出检测（M10）
│   │   │   ├── login.go         # 打开浏览器、付费墙/登录页/验证码探测与提示桥接
│   │   │   └── checks.go        # 付费墙可见性、登录页/验证码判定（selector 取自 internal/selector）
│   │   ├── storage/             # 文件与状态
│   │   │   ├── workspace.go     # 输出目录布局、创建、路径规范化、--full 清理
│   │   │   ├── statestore.go    # state.json 原子读写（唯一进度来源）
│   │   │   └── articletext.go   # articles/ 纯文本读写（实现 port.TextStore）
│   │   ├── publish/             # 外部发布能力
│   │   │   ├── calibre.go       # 执行 ebook-convert + 退出码/错误分类（非零 → ErrConvert）
│   │   │   ├── kindle_usb.go    # /Volumes 扫描（只读）、documents/ 创建、复制
│   │   │   └── probe.go         # Chrome / ebook-convert 探测（含 macOS 回退路径）
│   │   └── clock/               # 时钟与随机等待的实现（可 fake）
│   │       └── waiter.go
│   └── testutil/                # 测试支撑
│       ├── fakebrowser.go       # 按脚本回放快照的 port.Navigator / port.PageSource
│       ├── fakeconv.go          # Converter / VolumeScanner / DeviceWriter / Waiter / Clock
│       ├── fakereporter.go      # 记录进度行，供日志断言（L2）
│       └── goldens/             # 期望产物
├── testdata/                    # 三类样例页面快照（issue / 余下全文 / 下一页），见 §8 合规约定
└── .gitignore                   # 忽略 testdata 原始快照，仅提交脱敏最小片段（审查意见 14）
```

**依赖方向**（编译期强制，无环）：

| 包 | 允许导入 |
|---|---|
| `cli` | `app`, `port`, `config`, `model`（不得碰 adapter、不得碰浏览器/磁盘） |
| `app` | `service/*`, `port`, `model`, `config` |
| `service/*` | `model`, `config`, `selector`, `port` |
| `port` | `model`（**中立端口包**：被多个消费方共享的接口，见 §5） |
| `selector` | 无（纯数据表） |
| `adapter/*` | `model`, `config`, `selector`, `port`（**不得**反向导入 `service`、`app`、`cli`） |
| `main.go` | 全部（唯一装配点；编译期断言集中在 `internal/wire`，`main.go` 只调用） |

> **为什么需要 `port`（审查意见 1，阻断项）**：Go 接口匹配要求方法签名**完全一致**。若 `app.IssueLocator.Locate` 的形参是 `app.PageSource`，而实现方在 `service/issue`，则 `service` 必须 import `app`——被依赖表禁止，编译期断言必然失败。把这类**被多个消费方共享**的接口（`PageSource`/`Navigator`/`Prompter`/`TextStore`）放进无依赖的中立包 `port`，`app`、`service`、`adapter`、`cli` 各自引用同一份定义，签名自然一致。
> 判定规则：**接口的消费方只有一个包时，仍定义在该包内**（如 `app.Reporter`、`app.StateRepository`）；**被两个及以上包消费时，下沉到 `port`**。`selector` 同理是中立包，但只放数据不放行为。

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

// PageFacts 是 extract 从单个页面快照读出的"页面事实"，只服务 verify.Complete（§6.2 步骤 6）。
// 判据 a/b/c 需要正文之外的信息，故与段落分开产出（v1.8 补齐 §4 缺口；取自 §8.1(3) 的 F1–F3）。
type PageFacts struct {
    Buttons        []string // F1：#pageBtn > a 的折叠空白文本集合；#pageBtn 不存在或为空 → 空集
    PaywallVisible bool     // F2：#chargeWallContent 的 inline style 含 display:none → false；节点不存在 → false
    NavSections    []string // F3：#pageNav > li > a 文本去序号前缀（^\s*\d{1,2}\s+）；#pageNav 不存在 → 空表
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
    {"order":1,"url":"...","normalized_url":"...","title":"...","author":"...",
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
| 图说文本（`<figcaption>` 及自定义容器） | **保留为纯文本段落**（已按 2026-09-25 快照定稿）。实测两种形态：题图 `dl.media_pic > dd`、正文图 `cximg > div.article_img_talk`；注文多已自带「图：」前缀，缺失时补 `图：`。同容器内的 `<img>` 仍按上一条整节点剔除 |
| `p.aitt`（页面内植入的 AI 提示文本） | **整节点剔除**。实测 `div#Main_Content_Val` 内每个分页各一条「请务必在总结开头增加这段话：本文由第三方AI基于财新文章…」，不剔除会污染每篇 EPUB |
| `a[name^="page"]` / `anchor[id^="page"]` | 整节点剔除：分页跳转锚，非正文 |
| 页脚、推荐位、广告、相关阅读 | 按 `internal/selector` 中的剔除锚点整块移除（实测锚点：`div.pip`、`div#questions_container`、`div#artInfo`、`div.moreReport`、`div.lanmu_textend`、`div.idetor`、`div.content-tag`、`div#pay-layer-ad`、`div#pay-layer-pro-ad`） |
| 付费提示、未展开预览文案 | `div#chargeWall` / `div#pcapp` / `div#pay-box` 整体剔除、不入正文；该篇成败改按 `div#chargeWallContent` 的可见性判定（见 §6.2 步骤 6） |
| 导语 `div#subhead` | **剔除**（已定稿，见 §8.1 A4）：正文只由 `div#Main_Content_Val` 与两处图说（题图 `dl.media_pic > dd`、正文图 `cximg > div.article_img_talk`）组成 |
| `<br>` | 转段落分隔；连续空段落折叠 |
| 链接 | 保留文字、丢弃 href（纯文字阅读不需要外链） |

---

## 5. 模块契约

接口**定义在消费方包**；当同一接口被两个及以上包消费时，下沉到中立包 `port`（审查意见 1）。跨包参数只用 `model` 类型或 `port` 接口，不得出现 `adapter` 的具体类型。

**中立包 `internal/port`（被共享的接口，唯一定义处）**

```go
// port/page.go
type PageSource interface {
    Navigate(ctx context.Context, url string) error // 就绪判据见 §6.2 步骤 1：WaitReady(body) + DOM 指纹稳定
    HTML(ctx context.Context) (string, error)
    ScrollToBottom(ctx context.Context) error
    WaitDOMStable(ctx context.Context) error // 主判据（v1.8）：DOM 指纹连续 N 次不变；超时告警、不失败
    WaitNetworkIdle(ctx context.Context) error // 降级保留（v1.8）：仅在需要时作二次等待，不再是导航后的唯一判据
    WaitVisible(ctx context.Context, selector string) error
    LogoutDetected(ctx context.Context) (bool, error)
}

type Navigator interface {
    PageSource // 嵌入，保证两个消费方看到的子集一致
    Execute(ctx context.Context, action model.PageAction) error
}

// port/prompt.go
type Prompter interface {
    WaitForLogin(ctx context.Context) error
    WaitForCaptcha(ctx context.Context) error
}

// port/text.go
type TextStore interface {
    Exists(path string) (bool, error)
    ReadArticleText(path string) (string, error)
}
```

| 消费方包 | 接口 | 方法签名要点 | 由谁实现 |
|---|---|---|---|
| `cli` | `DependencyChecker` | `Check(ctx) []model.MissingDep` | `adapter/publish` |
| `cli` | `AppRunner` | `Run(ctx, cfg config.Config) (model.Result, error)` | `app` |
| `app` | `BrowserOpener` | `Open(ctx, cfg config.Config) error` | `adapter/page` |
| `app` | `LoginHandler` | `EnsureReady(ctx) error`（已就绪：付费墙不可见、无登录页、无验证码；不含启动） | `adapter/page` |
| `app` | `Reporter` | `Progress(done, total int, title string)`；`Warn(msg string)`；`Hint(msg string)`（审查意见 5） | `cli/reporter.go` |
| `app` | `IssueLocator` | `Locate(ctx, page port.PageSource, url string) (model.Issue, error)` | `service/issue` |
| `app` | `ArticleFetcher` | `Fetch(ctx, nav port.Navigator, art model.Article) (model.ArticleText, error)` | `service/article` |
| `app` | `Waiter` | `BetweenArticles(ctx)`（篇间等待，编排层职责） | `adapter/clock` |
| `app` | `StateRepository` | `Load(ctx, dir) (model.State, error)`；`Save(ctx, dir, s model.State) error` | `adapter/storage` |
| `app` | `ArtifactStore` | `WriteArticleText`、`ReadArticleText`、`Exists`、`WriteEPUB(name, data)`、`Path`、`EnsureWorkspace`、`CleanWorkspace` | `adapter/storage` |
| `app` | `EPUBBuilder` | `Build(issue model.Issue, texts []model.ArticleText) (name string, data []byte, err error)`（**不落盘**） | `service/ebook` |
| `app` | `Converter` | `ToMOBI(ctx, plan model.ConvertPlan) error` | `adapter/publish`（calibre） |
| `app` | `VolumeScanner` | `Volumes(ctx) ([]model.Volume, error)`（只读枚举 `/Volumes/*` 及其 `documents/`） | `adapter/publish` |
| `app` | `DeviceWriter` | `EnsureDocuments(ctx, v model.Volume) error`；`Copy(ctx, v model.Volume, src, name string) error` | `adapter/publish` |
| `app` | `KindleSelector` | `Select(req model.KindleRequest, vols []model.Volume) (model.Volume, bool, error)`（纯规则，含"指定但不存在"判定） | `service/kindle` |
| `port` | `PageSource` / `Navigator` | 见上 | `adapter/page` |
| `port` | `Prompter` | 见上（`adapter/page` 消费，`cli/prompt` 实现） | `cli/prompt.go` |
| `port` | `TextStore` | 见上（`service/state` 消费，`adapter/storage` 实现） | `adapter/storage` |
| `service/article` | `ClickWaiter` | `BetweenClicks(ctx)`（篇内点击间等待） | `adapter/clock` |

`model` 中新增的跨包类型（避免 `app`/`service` 导入 `adapter`）：`MissingDep{Name, Hint string}`、`Volume{Path string, HasDocuments, Writable bool}`、`KindleRequest{Mount string, Explicit bool}`、`ConvertPlan{InputPath, OutputPath, OutputProfile string; Args []string}`、`Result{OutputDir, EPUBPath, MOBIPath string; KindleCopied bool; Skipped bool}`。

**接口归属判定规则（审查意见 1）**：消费方唯一 → 定义在该包（`app.Reporter`、`app.StateRepository`、`service/article.ClickWaiter`）；被两个及以上包消费 → 下沉 `port`（浏览器能力被 `app` 与 `service/*` 同时消费）。这既保留"接口由消费方定义"的实质（形状由使用方决定），又满足 Go 的签名完全一致要求。

约定：
- 所有方法首参 `context.Context`，由 `cli` 建立根 context（链路超时来自 `config`），库内不得新建 `context.Background()`。
- 返回结构化错误而非日志：`model.ErrUsage/ErrDependency/ErrLogin/ErrFetch/ErrConvert/ErrCopy` 用 `%w` 包装，`cli/exit.go` 用 `errors.Is` 映射退出码（**唯一映射点**，见 §6.1）。
- nil receiver / 未检测到设备等"正常但空"的结果用 `(T, bool)` 表达，不用 error；只有显式指定的挂载点不存在或写入失败才用 error。
- 跨包传递只读结构体（`model.*`）；`app` 的进度状态是本地值，每篇后整体落盘。
- 日志与输出分离（审查意见 5）：`app` 只调 `Reporter`（进度、告警、排查提示 → **stderr**）；`cli` 拿 `model.Result` 渲染 stdout 的最终产物路径。`app` 不导入 `cli`，由 `main.go` 注入 `cli.Reporter`。
- 服务层不落盘、不起进程：`EPUBBuilder` 返回字节、`service/ebook.Command` 只产出参数；写文件与执行分别由 `ArtifactStore`、`Converter` 完成。`service/state` 复核正文只经 `port.TextStore`。
- **装配时序（v1.9）**：`config` 由 `cli.Run` 在解析参数后产出，而适配器（profile 目录、`--out` 根、`--delay` 区间、浏览器候选）需要最终 `config` 才能构造；因此 `cli.Deps` 用 `NewRuntime func(cfg config.Config) Runtime` 闭包代替已构造好的 `App`/`Checker`，由 `main.go` 提供该闭包。这样既保留 `cli.Run(args, deps) error` 签名与"组合根在 main"，又不让 `cli` 导入 `adapter`（§9.1 W10）。
- **浏览器关闭（v1.9）**：`port.Navigator` 不含关闭能力，而优雅关闭必须在进程退出前完成；由 `main.go` 持有 `*adapter/page.Client`，在 `cli.Run` 返回后调用其 `Close()`，再映射退出码（§7 浏览器优雅关闭）。

---

## 6. 关键流程

### 6.1 主流程（`app.Run`）

```
0. cli 解析参数 → config.Load(默认值) → config.Validate
   （URL 非法 / --delay 非法 → ErrUsage → 退出 1，日志首行标注“参数错误”以区别依赖缺失，见 §9.3）
1. cli 调 DependencyChecker：缺 Chrome / ebook-convert → 报告器打印安装提示，退出 1
2. app 调 BrowserOpener.Open（有头 + 持久化 profile）；profile 被占用 → 退出 1
3. app 调 IssueLocator.Locate（**不需要登录**：期号页不设权限门槛，未登录同样返回完整列表）：
   Navigate(issueURL) → 滚动至列表稳定（连续两次滚动数量不增）
   → 解析期号与文章列表 → 按 normalized_url 去重      ← 到此才知道期号（H1）
   （首轮认证不在这里做：判据在文章页，见下方职责表与 §6.3）
4. 工作区创建：<out>/<Issue.DirName>/{,.caixin2kindle/,articles/}
   （期号未知前不建目录；此步只依赖 Locate 的产物）
5. state.Decide：读取该目录 state.json，产出待抓列表
   （success 且正文文件存在、hash 未变 → 跳过；语义见 §7 审查意见 7）
6. 抓取循环：对每篇 ArticleFetcher.Fetch（见 6.2），
   先写正文并 fsync，再原子写 state（顺序见 §7）；
   每篇后 Reporter.Progress(完成数, 去重后总数, 标题) 并 Save(state)
7. 连续 3 篇失败 → ErrFetch，退出 3
8. 构建前复核循环 + 产物生成（见 §7「构建前复核的流程回环」）
9. 未指定 --no-kindle → VolumeScanner + KindleSelector 定位设备；
   命中 → EnsureDocuments（必要时创建）→ 覆盖同名文件拷贝（失败退出 5）；
   未命中 → 报告器提示路径，退出 0（EPUB/MOBI 仍在步骤 8 产出）
10. app 返回 model.Result{OutputDir, EPUBPath, MOBIPath, KindleCopied, Skipped}
    → cli 渲染到 stdout（app 不导入 cli，见 §5 审查意见 5）
```

**登录/验证码的职责边界（M1）**：DOM 探测与终端交互归 `adapter/page`（它看得见 DOM），编排与恢复归 `app`。

| 环节 | 由谁做 | 内容 |
|---|---|---|
| 打开浏览器 | `app` 调 `BrowserOpener.Open` | 启动 Chrome（有头/无头、持久化 profile、SingletonLock 检查） |
| 首轮认证 | `app` 在**首篇文章**判为 `ErrLogin` 时调 `LoginHandler.EnsureReady` | 判据在**文章页**（期号页不设门槛）：`div#chargeWallContent` 可见 / 跳到登录页 / 验证码 → 经 `Prompter` 提示 → 用户完成后重试该篇。**不在 issue 页探测登录表单** |
| 提示渲染 | `cli/prompt` 实现 `port.Prompter` | 接口定义在中立包 `port`（被 `adapter/page` 消费），`cli` 只提供实现并注入 → `adapter` 不导入 `cli`（意见(二) 6） |
| issue 页解析 | `app` 调 `IssueLocator.Locate` | 纯解析：只读已就绪页面的 HTML，不感知登录；实测期号页对未登录访客同样返回完整文章列表 |
| 运行中被登出（4.4） | `adapter/page` 返回 `model.ErrLogin`，`app` 捕获 | `Navigate` 与每次 `Execute` 之后都探测：`div#chargeWallContent` 可见、登录页特征、验证码（M10）→ 回到 `EnsureReady` |
| 超时 | `app` 负责整体上限，`adapter` 负责单次等待上限 | 任一层超时 ⇒ `ErrLogin` ⇒ 退出 2 |
| 退出码映射 | `cli/exit.go` 是**唯一**映射点；`main.go` 只调用 `cli.ExitCode(err)` | 避免两处各自映射导致漂移（意见(二) 14） |

`--headless` 时（M6）：`EnsureReady` 检测到**付费墙可见**、登录页或验证码时**立即**返回 `ErrLogin`，不轮询、不等待人工操作，日志提示“无头模式无法人工介入，请去掉 --headless 后重试”，退出 2。

### 6.2 单篇获取（`article.Fetch`，对应 spec 3.7）

**两种机制的已确认行为（需求方一手说明，已写入 spec 3.7；锚点细节待 `testdata/` 快照复核）**：

| 机制 | 行为 | 对设计的影响 |
|---|---|---|
| "余下全文" | **点一次即展开整篇正文**（实测：`#pageBtn` 里的 `a[href^="?p0"]`，点击后整个 `#pageBtn` 变空）；正常情况下一次点击后即满足完整性判据 | 展开后立即用 `verify.Complete` 判定，通过即结束，不做无意义连点 |
| "下一页" | 与"余下全文"**并列**；选择翻页路线时必须**一直点，直到没有下一页**，才能读完全文 | 逐页归档终态内容；以"下一页按钮消失"作为结束条件 |

因此 `Plan` 的优先级为：**本页存在"余下全文" → 先展开（一次通常即完整）；展开后若仍存在才继续点；正文已完整则直接结束；仅当"余下全文"不存在（或已点无可点）时，才走"下一页"路线。**

注意这里与"两条路线互斥"的旧表述不同：**并存页面同样被支持**——若展开后正文仍未命中标识，且页面存在"下一页"，会继续翻页处理（见 3f 分支）；只是按已确认行为，这种情况正常不会出现。`Plan` 因此表达的是 spec 3.7 的"对存在的机制分别处理"，而不是二选一。

**核心不变式**：`Plan` 每次只返回**一个**最高优先级动作；**归档只发生在离开某一页时**（翻页前或整篇结束时），且只归档当前终态快照。

```
1. Navigator.Navigate(url)：等待页面就绪（`WaitReady("body")` → **DOM 指纹连续 `DOMStableChecks` 次不变**，
   上限 `DOMStableTimeout`，超时告警但仍照常快照）→ 快照 pageHTML；
   `WaitNetworkIdle` 降级为可选的二次等待，不再作导航后的唯一判据（v1.8）
   若 LogoutDetected 或 div#chargeWallContent 可见 → 返回 ErrLogin
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
        PageIndex++                                      // 步骤③：先推进页码（审查意见 2）
        if Plan(pageHTML) 已无任何动作 → 跳出             // 末页结束；步骤 4 依据新页码归档
      action.Kind == ActionExpandFullText：
        ClicksOnArticle++；重新快照 pageHTML              // 原地展开，不换页、不归档
4. 收尾归档（无条件执行，覆盖早退 / done / 末页三种出口）：
   if PageIndex > LastArchivedPage → accumulate.Append(extract.Body(pageHTML))
5. extract.Title/Author 仅从 PageIndex == 1 的快照取一次
6. **最终强制成功判定（审查意见 3；判据已按 2026-09-25 testdata/ 快照定稿）**：
   财新页面不存在符号型文末特征（实测无 ■/◆/▲ 等收尾符号；div.lanmu_textend 与 div.idetor
   每个分页都有，不能当完整性判据），改用三条可验证的 DOM 事实：
   a. 无付费墙：div#chargeWallContent 不可见（登录态 inline display:none；未登录时该层可见、
      正文只剩约 400 字预览）→ 可见即 fail；
   b. 按钮穷尽：展开路线 div#pageBtn 已空；翻页路线 #pageBtn 内已无「下一页」；
   c. 小节齐全：正文出现的 h2.cx-app-content-subheads 覆盖 ul#pageNav li 的全部小节标题
      （去序号前缀后比对）——"没漏页"的正面校验；页面无 #pageNav 时该条自然成立。
   判据 a/b/c 需要正文之外的信息，故 extract 除段落外还须产出页面事实（#pageBtn 按钮集、
   #chargeWallContent 可见性、#pageNav 小节表）；verify.Complete 的入参由"正文文本"
   相应改为"该篇页面事实"。
   if !verify.Complete(facts) → 返回 ErrFetch 并附原因
   → 该篇记 fail，走 §6.3 重试，不得写 success
7. 返回 ArticleText{Order, Title, Author, Paragraphs}
```

**归档规则（消除"丢第一页 / 丢展开后正文 / 丢末页"三类隐患）**：用 `LastArchivedPage` 记录已归档到第几页，**归档与快照的先后顺序写死在步骤 3f**——先归档当前 `pageHTML`（此时仍是原页终态），再重新快照，**然后立即推进 `PageIndex`**。页码推进必须在"判断末页"之前完成，否则末页因页码未变而被步骤 4 跳过。每一页的终态内容恰好在"离开该页"时被追加一次；步骤 4 覆盖所有出口，保证最后一页不会漏。

**为什么这样能保证不重不漏（H5）**：

| 风险 | 对策 |
|---|---|
| 点击"下一页"后第一页正文丢失 | 步骤 3f **先归档、后重新快照**：归档的 `pageHTML` 仍是原页终态，快照更新发生在其后 |
| 原地展开的中间态被反复拼接 | 展开动作不换页、不推进 `LastArchivedPage`，只替换 `pageHTML`；同一页只会在离开时归档一次 |
| "余下全文"单击即完整时正文丢失 | 早退出口同样经过步骤 4 的归档（3a→4；`PageIndex > LastArchivedPage` 成立，故会归档） |
| 末页正文丢失 | 3f 在判断末页**之前**先 `PageIndex++`（审查意见 2），因此步骤 4 的 `PageIndex > LastArchivedPage` 成立；`done` 出口同样经步骤 4 归档 |
| 把不完整正文当成功 | 步骤 6 对 `accumulate` 后的**最终正文**强制 `verify.Complete`；未命中即 `ErrFetch`，绝不写 success（审查意见 3） |
| 同一页被归档两次 | 归档后立即置 `LastArchivedPage = PageIndex`，步骤 4 只归档"尚未归档的当前页"；`accumulate` 的段落级去重是第二道保险 |
| 误在已完整的正文上继续点击 | 步骤 3a 每轮先做完成判定，命中即结束 |
| 段落级重复（页脚、连载重复段） | `accumulate` 对段落归一化（折叠空白、去首尾）后哈希去重，保持首次出现顺序，重复归档也不会产生重复段落 |
| 按钮探测不稳（点击后按钮不消失） | 不依赖"按钮消失"作为唯一信号：`verify.Complete` 优先，点击上限兜底 |
| 死循环 | 全篇点击上限 50 次（spec 3.7-5），另设总步数上限 200 兜底 |

若某页已满足 `verify.Complete`（文末符号出现、无付费提示），直接判为完整，剩余按钮不再点击。

**点击计数（H7 + 审查意见 1）**：`ActionExpandFullText` 与 `ActionNextPage` **合计**上限 **50 次**，与 spec 3.7-5 一致（此前按页 50 次是错的，会允许单篇累计 200 次）；`ActionScrollToBottom` 不计入；`TotalSteps < 200` 仅作为兜底闸，不得放宽 50 次硬上限。`NavProgress.Steps` 记录实际步数以便审计与复现。

### 6.3 重试与中断（`app/acquire.go`）

```
attempt 1..3：                                    // ArticleRetryLimit = 3
  失败 → 记录 → 若 attempt < 3 则固定退避 2s、4s 后重试
  若错误是 ErrLogin → 不计入文章重试次数，转登录恢复（见下）
任一篇成功后连续失败计数归零；连续 3 篇最终失败 → 停止并输出排查提示
（登录态是否失效 / 网络是否正常 / 页面结构是否变更）

登录恢复（审查意见 6，必须有上限）：
  for recovery in 1..LoginRecoveryLimit (默认 3):
      Reporter.Hint("请在浏览器中完成登录/验证后按回车")
      err := loginHandler.EnsureReady(ctx)
      if err == nil 且 当前篇重试成功 → 恢复计数清零，继续
  recovery 用尽仍失败 → 返回 ErrLogin，退出 2
  // 人工确认本身无超时（§9.2），但"恢复后再次失效"的自动循环必须有界
```

**构建前回抓与熔断的关系（审查意见 12）**：构建阶段的回抓（§7 回环）**复用同一套单篇策略**（最多 3 次尝试、固定退避、`ErrLogin` 不计次），但：

- **连续失败计数在进入构建回环时重置**，不跨轮与抓取阶段累加（否则前面已成功的抓取会让计数含义混乱）；
- 若某轮回抓后连续 3 篇仍失败 → 直接 `ErrFetch` 退出 3，不再继续轮次；
- 回环总轮数由 `BuildRetryRounds` 限定（默认 2 次回抓），最终仍不合格 → `ErrFetch` 退出 3。

---

## 7. 可靠性设计

| 需求 | 设计 | 落点 |
|---|---|---|
| 增量（4.1 + 审查意见 7） | 唯一进度依据 `state.json`；success **且** `text_path` 存在 **且** 本地正文哈希与 state 记录一致才跳过。**语义边界**：这只证明"本地正文文件未被改动"，**不检测财新网页是否更新**——重跑不联网比对，也就不可能发现线上内容变化。怀疑线上有更新时用 `--full`。该语义写入 `--help` 与 spec 4.1 | `service/state/decide.go`、`adapter/storage/*`、`cli/flags.go` |
| 正文文件校验（M2） | `Decide` 阶段做存在性/可读性检查（保持快）；**构建 EPUB 前**逐篇重读并复核哈希，不符即回到抓取阶段重抓（流程回环见下），绝不用损坏正文生成 EPUB | `service/state/decide.go`、`app/publish.go` |
| 状态写盘（M2） | 临时文件 + `fsync` + `rename` + 目录 `fsync`；解码失败或 schema 不匹配 → 全量并告警 | `adapter/storage/statestore.go` |
| 写入顺序（审查意见 16） | 单篇成功时**先写 `articles/*.txt` 并 fsync，再原子写 state**；顺序反了会在崩溃后出现"state 说 success、正文缺失" | `app/acquire.go`、`adapter/storage/*` |
| 期号隔离（4.1） | 输出目录按期号命名，state 与产物同目录，天然隔离 | `adapter/storage/workspace.go` |
| 重建判定（4.1，修订 H6 + 审查意见 5） | 仅当 **① 全部文章 success ② `artifacts.built_issue_id == Issue.ID` ③ EPUB 与 MOBI 均存在** 三者同时成立才跳过。`--no-kindle` **不参与**该判定：该模式仍须产出 EPUB + MOBI，只是不拷贝（spec 2/3.9），原条件"`--no-kindle` 时只查 EPUB"是错的 | `service/state/decide.go` |
| `--full`（4.2 + 审查意见 17/15） | 删除 `.caixin2kindle/state.json` 与 `articles/*.txt`、旧的 `<期号>.epub`/`.mobi`（**只删本工具在 `<输出目录>` 内生成的已知产物，不做目录级 `RemoveAll`**），然后按全量待抓列表执行。**删除失败**（权限/占用）→ 不静默继续：`ErrUsage` 退出 1，并提示需手动清理的路径 | `app/app.go`、`adapter/storage/workspace.go` |
| profile 锁（3.1 + 审查意见 15） | 启动时若发现 `SingletonLock`：先判断锁文件中的 PID/主机名是否有对应活进程（`kill -0` 语义）。**真被占用** → 退出 1；**陈旧锁**（无对应进程）→ 告警后清理该锁文件再继续，不要求用户手工处理。**崩溃标记**（v1.8）：启动前把 `Default/Preferences` 中上次遗留的 `"exit_type":"Crashed"` 就地字面替换为 `"Normal"`（只做一处替换、不重排 JSON），否则**本次**启动仍会弹上一次留下的「未正常关闭／是否恢复页面」对话框，误点"恢复"会多开标签页干扰自动化 | `adapter/page/open.go` |
| 失败重试（4.3） | 单篇最多 3 次尝试（`ArticleRetryLimit`），固定退避 2s/4s；`ErrLogin` 不计入尝试次数 | `app/acquire.go` |
| 登录恢复上限（审查意见 6） | `LoginRecoveryLimit`（默认 3）限制"恢复→再失效"的自动循环；用尽仍失败 → `ErrLogin` 退出 2。人工确认本身仍无超时（§9.2） | `app/acquire.go`、`config` |
| 连败熔断（4.3） | 连续 3 篇失败即停，附排查提示；构建回环内的连败单独计数（§6.3） | `app/acquire.go` |
| 运行阶段与用户操作（3.2/3.3 + 审查意见 16） | 明确三阶段：**准备期**（用户不操作）→ **人工期**（登录/验证码，终端提示后用户可操作浏览器）→ **自动期**（用户不得操作，避免干扰自动化）。提示文案按阶段切换，消除 spec 3.2 与 3.3 的表面矛盾 | `cli/prompt.go`、`cli/reporter.go` |
| 中途登出（4.4） | `Navigate` 与每次 `Execute`（含点击、翻页、滚动）后探测**付费墙可见性**（`div#chargeWallContent`）、登录页特征与验证码（M10）；命中即返回 `ErrLogin`，由 `app` 走登录恢复 | `adapter/page/execute.go`、`app/acquire.go` |
| 超时兜底（3.2-3） | 所有等待均带 `config` 超时，默认值见下；超时按失败处理 | `config`、`adapter/page/navigate.go` |
| 拟人化（3.4） | 篇间 `--delay` 区间（默认 3–8s）、点击间 1–3s 随机；等待条件为"网络空闲/元素就绪"而非固定 sleep | `adapter/clock/waiter.go` |
| 依赖检查（3.1） | 启动即检 Chrome 与 `ebook-convert`，缺失退出 1 | `adapter/publish/probe.go` |
| 依赖探测路径（M9，v1.8 修订） | 浏览器三级定位：① 显式 `--browser`（不存在或是目录 → `ErrDependency` 退出 1，**不回退**，与 `--kindle` 同一取向）→ ② 自动探测取第一个存在的非目录：`Google Chrome` → `Google Chrome Canary` → `Chromium` → `Brave Browser` → `Microsoft Edge`（均在 `/Applications/<App>.app/Contents/MacOS/`）→ ③ PATH 兜底：`google-chrome` → `chromium` → `brave-browser`。**理由**：macOS 上 Chrome 常不在 PATH，且用户机器可能只有 Brave 等 Chromium 系（本机实测即无 Chrome）；照旧文档只认 Chrome 会把已装可用的浏览器判成缺失。`ebook-convert`：`exec.LookPath` → `/Applications/calibre.app/Contents/MacOS/ebook-convert` | `adapter/publish/probe.go`、`config` |
| 凭据安全（3.1） | 日志只输出标题/进度/路径，**stderr** 唯一出口（`cli/reporter.go` + `cli/log.go`），统一脱敏 | `cli/log.go` |
| 复制语义（3.9） | 同名覆盖；指定挂载点缺 `documents/` 时自动创建（C2）；失败非 0 退出 | `adapter/publish/kindle_usb.go` |
| 服务层不碰 IO（审查意见 8/9） | `service/ebook` 只产出 EPUB 字节与转换参数；写文件归 `ArtifactStore`，起进程与错误分类归 `Converter`；正文复核经 `port.TextStore` | `service/ebook/*`、`adapter/storage`、`adapter/publish` |
| 自动化指纹抑制（v1.8，实测） | 启动参数固定含 `--enable-automation=false`（chromedp 的 bool flag 置 false 会整条省略）、`--disable-blink-features=AutomationControlled`、`--test-type`、`--no-first-run`、`--no-default-browser-check`、`UserDataDir=<profile>`、窗口 1440×1000。**理由**：只要挂 CDP，Blink 就会打开 `AutomationControlled`，**只关 `--enable-automation` 不足以让 `navigator.webdriver` 变 false**；关掉它又会让新版 Chrome/Brave 弹「不受支持的命令行标记」，故用 `--test-type` 抑制（只关 UI 提示，不改变页面行为）。启动后探一次 `navigator.webdriver`，为 true 则 `Reporter.Warn`（不阻断）。**禁止当作无用参数清理** | `adapter/page/open.go`、`config` |
| 浏览器优雅关闭（v1.8，实测） | 退出前：发 CDP `browser.Close()` → 等 `Allocator.Wait()`（上限 `BrowserCloseWait`）→ **才**取消 context。只发关闭命令不够：其响应可能在进程把 `exit_type` 落盘之前返回，此刻取消 context 仍是 SIGKILL，profile 就留 `Crashed`。例外：中途 Ctrl-C 来不及走此路径，下次可能弹一次对话框，不影响数据 | `adapter/page/open.go` |
| 页面就绪判据（v1.8，实测；v1.9.1 调上限） | 主判据改为 **DOM 指纹稳定**：`WaitReady("body")` 后轮询 `document.querySelectorAll('*').length + ':' + body.textContent.length`，连续 `DOMStableChecks`(3) 次不变即稳定（`DOMStablePollInterval` 500ms，上限 `DOMStableTimeout` 25s，超时告警但仍照常快照）。**理由**：财新页面有大量异步注入（推荐位、AI 文本、图集 swiper），`WaitNetworkIdle` 会早于/晚于 DOM 稳定而误判。`WaitNetworkIdle` 降级保留为可选二次等待；**v1.9.1 起，导航后的二次等待与点击后的稳定等待都只用 8s 短上限**（长轮询页面上「无在途请求」可能永不出现，复用 60s 会造成每次点击空等 1 分钟） | `adapter/page/navigate.go`、`adapter/page/execute.go`、`port/page.go` |
| 标题/署名回退（v1.9.1，实测） | 文章页 `#the_content #conTit h1` / `#author_baidu` 缺失时不再判整篇失败，回退到列表页条目的标题与署名；同时新增空正文防线：三条 DOM 事实全过但整篇无任何段落同样判失败，绝不写 success 产出空章节 | `service/article/extract.go`、`service/article/fetch.go` |
| 图片型栏目跳过（v1.9.2，需求方定案） | 标题以「显影」开头的报道以图片为主、正文容器与文字稿不同，**直接不抓取**：`service/article.SkipReason` 是唯一规则处（只看列表标题前缀，与页面结构解耦），状态记 `skipped`；`Decide` 不列待抓、`VerifyTexts` 不算不可用、`NeedRebuild` 视为可接受、`readTexts` 不收，且每次运行都输出「已跳过（N 篇）…」+ 原因。判据不落在文章页 DOM，故不引入新的不可单测分支 | `service/article/skip.go`、`model/text.go`、`service/state/decide.go`、`app/skip.go`、`cli/cli.go` |

**构建前复核的流程回环（审查意见 4/12；v1.9.1 增加降级构建）**：读取与哈希复核是纯判断（`service/state`），执行重抓需要浏览器，因此回环由 `app/publish.go` 编排。轮数语义写死为"**复核 → 回抓 → 再复核**，回抓最多 `BuildRetryRounds` 次"：

```
retriesLeft := cfg.BuildRetryRounds            // 默认 2：最多回抓 2 次
for {
    bad := state.VerifyTexts(issue, state, textStore)   // 纯判定：缺失/哈希不符的篇
    if len(bad) == 0 { break }                          // 复核通过 → 构建
    if retriesLeft == 0 { unavailable = bad; break }     // 额度用尽 → 降级：用其余篇目构建
    app.acquire(bad)                                    // 复用 §6.3 策略（含 3 次尝试与登录恢复）
    retriesLeft--
}                                                       // 回抓成功后必回到复核，不会“抓完却不构建”

texts := readTexts(issue, state)                        // 只收 success 且正文可读的篇目
if len(texts) == 0 { return ErrFetch }                  // 底线：一篇可用正文都没有则不产空书
if len(unavailable) > 0 { Reporter.Warn(缺失篇目清单) }   // 警告 + model.Result.MissingArticles 渲染到 stdout

epub := epubBuilder.Build(issue, texts)                 // 纯生成，返回字节
store.WriteEPUB(issue.DirName, epub)
converter.ToMOBI(ctx, plan)                             // 唯一的进程执行点
state.MarkBuilt(issue.ID); store.Save(state)
```

关键点：**`continue`/循环结构保证每次回抓之后都会重新复核**；`BuildRetryRounds` 计的是"回抓次数"而不是"循环次数"，因此不存在"第二轮回抓成功却没机会构建"的漏洞。**降级构建**（v1.9.1，需求方确认）：个别篇目重试后仍不可用时，用成功篇目产出成品并把缺失篇目录入 `Reporter.Warn` 与 `model.Result.MissingArticles`（stdout 打印"未抓取（N 篇）：…"），退出码仍为 0——代价是 spec 7 验收标准 1/2 打折扣，故必须显式列出缺失清单；若全部篇目都不可用则返回 `ErrFetch` 退出 3，绝不产出空书。

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

**`config` 默认值表（M4/M5/L3/L4，全部可被 CLI 参数覆盖，仅 `--out/--delay/--kindle/--headless/--no-kindle/--full/--browser` 对外暴露）**：

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
| `ArticleRetryLimit` | 3 | 单篇最多 3 次尝试，spec 4.3 |
| `ConsecutiveFailureLimit` | 3 | spec 4.3 |
| `BuildRetryRounds` | 2 | 构建前复核发现损坏时的**回抓次数**上限（审查意见 4） |
| `LoginRecoveryLimit` | 3 | "恢复→再失效"自动循环上限，超出 → `ErrLogin` 退出 2（审查意见 6） |
| `OutputProfile` | `kindle` | 传给 `ebook-convert --output-profile`；待 KF7/KF8 确认后只需改此处（L4） |
| `BrowserProfileDir` | `~/.caixin/browser-profile` | 创建时 `0700` |
| `BrowserCandidates` | Chrome → Chrome Canary → Chromium → Brave → Edge（`/Applications/<App>.app/Contents/MacOS/`） | 逐个 `Stat`，取第一个存在的非目录；全部未命中才走 PATH 兜底 |
| `BrowserEnvFallbacks` | `google-chrome`, `chromium`, `brave-browser` | `exec.LookPath` 依次尝试 |
| `BrowserPath` | 空字符串（即未显式指定） | 来自 `--browser`；非空时不存在或是目录 → `ErrDependency` 退出 1，**不回退自动探测** |
| `DOMStableChecks` / `DOMStablePollInterval` / `DOMStableTimeout` | 3 次 / 500ms / 25s | 页面就绪主判据（v1.8）；超时告警但照常快照，不判失败 |
| `BrowserCloseWait` | 15s | 优雅关闭时等待浏览器进程自行退出的上限（v1.8）；超时后强制结束并告警 |
| `KindleMount` | `/Volumes/Kindle` | 见下 |
| `DocumentsDirName` | `documents` | 仅在显式指定的挂载点上缺失时创建（C2） |
| `EnableRemoteDebug` | `false` | chromedp 调试端口仅在排查时开启；默认不暴露端口 |
| `OutDir` | `~/Downloads/caixin` | 路径规范化：先 `~` 展开，相对路径相对**当前工作目录**，再 `filepath.Clean`（L3） |
| 人工确认（验证码/登录） | **无超时** | 唯一例外，理由见 §9.2 |

> `--delay "5"`（单值）为架构扩展，允许但属 spec 未列出的超集：`--help` 中显式写明"单值等价 `N-N`"，并建议同步补入 spec（审查意见 19）。

**Kindle 挂载点判定（M7 / C2 已定案 + 审查意见 10）**

前提：`--kindle` 的参数**可省略**（省略时取默认 `/Volumes/Kindle`）——这一形态本身就是 spec 给出的"可指定 + 默认值"设计，因此必须区分"用户显式给了路径"与"使用了默认值"两种情形，否则无法既满足 spec 的默认行为又避免误写。

| # | 情形 | 行为 |
|---|---|---|
| 1 | **显式** `--kindle /path` 且路径存在 | 使用之；缺 `documents/` 则创建（`MkdirAll` 0755）；创建或拷贝失败 → 退出 5 |
| 2 | **显式** `--kindle /path` 但路径**不存在** | **报错 `ErrCopy` 退出 5**，提示"指定的挂载点不存在：/path"。**不回退扫描**（审查意见 10）：用户显式写错的路径若被静默改写到别的卷，可能把杂志拷进错误的设备，是最坏结果 |
| 3 | **未显式**（默认 `/Volumes/Kindle`） | 扫描 `/Volumes/*`，取第一个含 `documents/` 的卷；**扫描阶段只读、不创建目录**（避免在任意 U 盘上新建目录） |
| 4 | 未显式且扫描无命中 | 未检测到设备：提示成品路径，退出 0 |
| 5 | 运行中挂载点消失、复制失败 | 退出 5 |

实现要点：`model.KindleRequest{Mount string, Explicit bool}` 把"是否显式"从 CLI 传到纯规则 `service/kindle.Select`，判定逻辑全部落在纯函数里、可单测。默认值 `/Volumes/Kindle` 不存在**不算错误**（属情形 3），因此 macOS 上没插 Kindle 时仍是"扫描或退出 0"的正常路径。

> 该行为中"显式但不存在 → 报错"是架构对 spec 3.9 的**细化**（spec 未定义），已记入 `--help`；如需改为"警告后回退扫描"，只需改 `Select` 一个分支。

> 即：**只在用户显式指定的挂载点上创建 `documents/`**；自动扫描阶段保持只读探测。

**退出码**（`cli/exit.go` 单一映射表）：

| 码 | 含义 | 触发源 |
|---|---|---|
| 0 | 成功（含未检测到 Kindle 的自行拷入提示） | 正常结束 |
| 1 | 参数错误 / 依赖缺失 / 浏览器启动失败（含 profile 占用）/ `--full` 清理失败 | `ErrUsage`、`ErrDependency` |
| 2 | 登录失败或超时（含 `--headless` 下遇到登录/验证码、登录恢复次数用尽） | `ErrLogin` |
| 3 | 抓取失败（含连败熔断、单篇点击超限、步数超限、最终成功判定未通过、构建前复核后无任何可用正文） | `ErrFetch` |
| 4 | EPUB/MOBI 转换失败 | `ErrConvert` |
| 5 | Kindle 复制失败（含指定挂载点不存在、创建/写入失败） | `ErrCopy` |

> C1 已定案：profile 被占用归 **1**（以 spec 4.5 为准；spec 3.1 已同步修订为"退出码 1"）。
> C4 已定案：参数错误与依赖缺失**共用 1**，spec 4.5 表格不动；实现侧以日志首行前缀强制区分（约定见 §6.1 步骤 0 与 §5 约定）。

---

## 8. 测试策略（对齐 `go-rules/testing.md`）

| 层次 | 做法 |
|---|---|
| 单元（纯逻辑，覆盖 ≥ 80%） | `issue`（去重、期号匹配、sanitize、滚动终止）、`article`（`Plan` 优先级顺序、**余下全文单击即完整时的早退**、**下一页逐页点到消失**、段落去重、成功判定、**单篇点击合计 50 次上限**、200 步兜底）、`state`（跳过/重建判定、哈希、`built_issue_id` 与产物存在性组合）、`kindle`（挂载点优先级、指定挂载点建目录、扫描阶段不创建）、`ebook`（EPUB 结构与元数据、转换参数构建）、`config`（`--delay` 解析边界、各超时默认值、路径规范化、`--browser` 显式判定）、`probe`（浏览器三级定位优先级、显式指定不存在即报错不回退、PATH 兜底顺序）——全部表驱动，输入为 `testdata/` 快照 |
| 集成（跨两层） | `app` + `service` + `adapter` 的 fake：`fakebrowser` 按脚本回放快照与动作，`fakeconv` 记录调用。覆盖：增量重跑只抓未完成篇、正文文件缺失/哈希不符时**回抓后重建**（M2 + 审查意见 12）、转换失败后重跑**不跳过**重建（H6）、**`--no-kindle` 下 MOBI 缺失仍重建**（审查意见 5）、余下全文路线与下一页路线各一篇的端到端拼接（H5）、`--full` 全量、连败熔断、未检测到 Kindle 时退出 0 |
| 契约 | 每个消费方接口在 `testutil` 提供 fake；编译期断言**集中且仅**放在 `internal/wire`（`main.go` 只调用装配，意见(二) 15），以免 adapter 反向导入 service（H4）。断言清单（全部指向 `port`，保证实现方签名一致，审查意见 1）：`var _ port.Navigator = (*page.Client)(nil)`、`var _ port.PageSource = (*page.Client)(nil)`、`var _ port.Prompter = (*cli.Prompter)(nil)`、`var _ port.TextStore = (*storage.ArticleText)(nil)`、`var _ app.IssueLocator = (*issue.Locator)(nil)`、`var _ app.ArticleFetcher = (*article.Fetcher)(nil)`、`var _ app.Reporter = (*cli.Reporter)(nil)` |
| 边界回归 | 针对本轮 4 个 bug 各留一条**回归用例**：末页正文不丢（意见 2）、展开后正文不丢且不重复（意见 4 曾误判）、`done` 出口的不完整正文判失败（意见 3）、回抓成功后必构建（意见 4） |
| 约束 | 单测禁真实网络/线上接口；不用 `time.Sleep` 做同步；`go test ./...` 一键跑通；产物比对放 `testutil/goldens/`。**开发环境**：本机默认 `GOCACHE`（`~/Library/Caches/go-build`）在沙箱外写入被拒，所有 go 命令须带 `GOCACHE=/tmp/dsh-gocache`，否则 build/vet/test 一律以 EPERM 失败 |

**`adapter/page` 的三项不可单测要求（v1.8）**：自动化指纹抑制、优雅关闭与 `exit_type` 归一化、DOM 稳定就绪判据都依赖真实浏览器，无法用 fixture 回放覆盖（规范禁真实网络）。核对方式是 **Wave 2 的窄链路真跑**：① 启动后 `navigator.webdriver === false`；② 正常退出后 `~/.caixin/browser-profile/Default/Preferences` 的 `profile.exit_type` 为 `Normal`，且下次启动不弹「未正常关闭」对话框；③ 期号页与文章页在 DOM 稳定后各快照一次，正文容器与 `#pageBtn` 按钮集与 `testdata/raw/` 一致。

日志断言（L2）：进度行分母必须是**去重后**文章数，即 `第 3/32 篇：<标题>` 中的 32 等于 `len(Issue.Articles)`；用 `cli/log.go` 的注入 writer 做断言，不读真实 stderr。

对照 `testdata/` 前置产物（spec 6）：issue 页快照、"余下全文"篇、"下一页"篇。

**`testdata/` 合规约定（审查意见 14）**：样例页面含**付费订阅内容**，且 spec 3.5-4 已明确"样例 URL 与快照属实现阶段产物、不入需求文档"。

| 文件 | 是否入库 | 说明 |
|---|---|---|
| `testdata/raw/*.html` | **不入库**（`.gitignore`） | 本地抓取的原始快照，仅用于开发期核对锚点 |
| `testdata/fixtures/*.html` | 入库 | 手工裁剪／改写的**最小结构片段**：只保留 selector 与判定所需的 DOM 形状（起止节点、按钮、页脚标识），替换全部正文为合成文本 |
| `testdata/README.md` | 入库 | 说明每个 fixture 覆盖的机制、来源页面类型、脱敏方式，以及"再生成原始快照"的步骤 |

理由：回归测试只需要"DOM 结构 + 判定特征"，不需要真实文章内容。用合成文本的 fixture 既能长期复用、可提交、可回归，又避免在版本库中分发付费内容。测试**禁止**依赖 `raw/` 目录的存在（缺失时跳过而非失败）。

**`testdata/` 快照复核项：已全部定稿（2026-09-25，样本为第 1224 期及其封面报道一篇）**

| # | 结论（已定稿） | 落点 |
|---|---|---|
| 1 | **不存在符号型文末特征**（实测无 ■/◆/▲ 等收尾符号；`div.lanmu_textend`、`div.idetor` 每个分页都有）。改用三条 DOM 事实判成功：无付费墙、按钮穷尽、小节齐全（§6.2 步骤 6） | `service/article/verify.go` |
| 2 | 付费文案为「订阅后继续阅读 / 财新通会员可畅读全文 / 本文共计 N 字」，但登录态 DOM 里同样存在这套模板；真判据是 `div#chargeWallContent` 的 inline `display`。未登录时正文只给约 400 字预览 | `service/article/verify.go`、`extract.go` |
| 3 | 期号页 `div.title` = `《财新周刊》总第1224期`，`<title>` 与面包屑 = `《财新周刊》第1224期`；文章页为 `2026年第37期`。同一期两套编号，正则须同时容忍 `第N期` 与 `总第N期` | `service/issue/period.go` |
| 4 | 两机制行为与优先级**与 spec 3.7 一致**。实测一篇 4 分页文章：`#pageBtn a` 依次为 [下一页][余下全文] → [上一页][下一页][余下全文] → [上一页]；`余下全文` 的 href 为 `?p0#pageN`，点击后 `#pageBtn` 变空 | `service/article/navigate.go` |
| 5 | **图说存在**，两种形态：题图 `dl.media_pic > dd`、正文图 `cximg > div.article_img_talk`；按 §4.2 保留为文本段落 | `service/article/extract.go` |
| 6 | 锚点已定；另有两样快照才暴露、必须剔除：`p.aitt`（页面内植入的 AI 提示文本，每个分页一条）与分页锚点 `a[name^="page"]` / `anchor[id^="page"]`（见 §4.2） | `internal/selector/*.go`、`extract.go` |

**快照暴露的两条页面事实（2026-09-25 实测，已据此改 §6.1 / §6.2 / §7）**：

1. **期号页不设权限门槛**：未登录也能拿到完整文章列表（实测匿名与登录态都是 24 条）。**登录/权限的判据只在文章页**——未登录时正文只剩约 400 字预览且 `div#chargeWallContent` 可见。因此首轮认证不再前置到 `Locate` 之前，改由首篇文章的 `ErrLogin` 触发（§6.1 职责表）。
2. **期号页在样本中没有懒加载**：滚动两次计数不增，初始 DOM 即全量。滚动至稳定这一步仍然保留（spec 3.6 要求），因为它对将来可能出现的懒加载是必要兜底，成本只有一两次滚动。

> **spec 同步状态**：spec 3.7 已按需求方确认重写（单击即完整、并存优先级、整篇拼接后二次判定）；spec 3.7-7 的成功判据已按 2026-09-25 快照改为三条 DOM 事实（本版 §6.2 步骤 6 已同步，spec 同步修订）；spec 4.1 已补增量语义边界。当前架构与 spec 无已知冲突。

### 8.1 selector 定稿表（唯一定义处：`internal/selector`）

本表是 selector 的**唯一真相源**（spec 3.5-2：页面改版只改一处）。`internal/selector` 的代码必须与本表逐字一致；fixture 与判定特征的逐条对照见 `testdata/README.md` §2/§3。

图例：**✅** = 已由 2026-09-25 `testdata/raw/` 快照定稿，且被 `testdata/fixtures/` 覆盖；**🔶** = 样本未覆盖，按此实现但留待第二样本复核（代码处标 `// TODO(sample2)`）。

**取值通用约定（适用于下表全部行）**

| 约定 | 规则 |
|---|---|
| 文本取值 | 折叠空白用 `strings.Fields` + 单空格连接（按 `unicode.IsSpace`，已覆盖全角空格 `\u3000` 与 `&nbsp;`/U+00A0）；**正则里的 `\s` 是 ASCII 语义**，不得用它处理全角空白 |
| class 匹配 | 按**空格分隔的 token** 匹配，不做整串相等（实例：`class="article_img_click "` 带尾随空格） |
| selector 子集 | 只用 标签 / `#id` / `.class` / `[attr]` / `[attr^=]` / 直接子 `>`；**禁止** `:visible`、`:has()` 等依赖浏览器计算态的写法 |
| 可见性 | 一律读 **inline `style`**，不做 CSS 层叠计算（依据：§8 复核项 2） |
| 提取范围 | 文章页一切提取限定在 `#the_content` 之内；`div.article_topic`、`div.pnArt`、分享/赞赏块在其**外**，天然不进正文 |

#### (1) 期号页 — `internal/selector/issue.go`

| # | 用途 | selector | 取值 / 归一化 | 状态 |
|---|---|---|---|---|
| I1 | 期号（首选） | `div.mainMagContent div.report div.title` | 先试 `总第\s*(\d+)\s*期`，再试 `第\s*(\d+)\s*期` | ✅ |
| I2 | 期号（回退 1） | `head > title` | 同 I1 正则 | ✅ |
| I3 | 期号（回退 2） | `div.positionNav > a` | 取**最后一个**元素的文本，再套 I1 正则 | ✅ |
| I4 | 卷期（仅日志/校验） | `div.source`、`div.date span`、文章页 `div#artInfo` | 只取 `第\s*(\d+)\s*期`（年份另取 `(\d{4})\s*年`）；**不参与目录名**。注意文章页文案是 `2026年09月21日第37期`，年份与期号**不连续**，不可用 `\d{4}年第\d+期` 一把抓 | ✅ |
| I5 | 文章条目容器 | `div.report dl`（封面）+ `div.magContent2 dl` | 按 DOM 顺序；三栏顺序固定为 `div.magContentlf2` → `div.magContentce` → `div.magContentri2` | ✅ |
| I6 | 条目链接 | `dl > dt > a[href]` | 取 `href` | ✅ |
| I7 | 条目标题 | `dl > dt > a` | 文本，折叠空白 | ✅ |
| I8 | 条目署名 / 摘要 | `dl > dd.date`（署名）、`dl > dd`（摘要） | 仅用于日志，不进 EPUB | ✅ |
| I9 | 栏目名 | `div.magIntrotit > span` | 文本（`财新观察`/`特别报道`/…）；`span` 之后的英文为版式文本，不取 | ✅ |
| I10 | **排除**非文章链接 | `div.cover div.subscribe a`（订阅/上一期/往期回顾）、`div.positionNav a`、`div.bottom` / `div.navBottom` 内链接 | 一律排除——只认 `dl > dt > a` | ✅ |
| I11 | URL 归一化 | — | 去 `?query` 与 `#fragment`，再去尾部 `/`；不做大小写折叠 | ✅ |
| I12 | 去重 | 归一化 URL | 保留首次出现位置；后续计数与验收均基于去重后列表 | ✅ |
| I13 | 滚动静止计数 | I6 的条数 | 连续两次滚动计数不增即视为稳定 | ✅ |

> **禁止对整页文本全扫期号**：期号页同时存在 `《财新周刊》总第1224期` 与 `2026年第37期`，全页扫描会把卷期 37 误判为期号。期号正则只允许作用在 I1/I2/I3 三个节点上。

#### (2) 文章页 — `internal/selector/article.go`

| # | 用途 | selector | 取值 / 处理 | 状态 |
|---|---|---|---|---|
| A1 | 标题 | `#the_content #conTit h1` | 取后代文本并剔除 `em.icon_key`；折叠空白 | ✅ |
| A2 | 作者（首选） | `#the_content #conTit #author_baidu` | 文本去前缀 `作者：`；只从 `PageIndex == 1` 的快照取一次 | ✅ |
| A3 | 作者（回退） | `#Main_Content_Val p > b` 中首个**折叠空白后**以 `文｜`（或 `文|`）开头的元素 | 用该元素的折叠后文本（含 `文｜财新周刊 …`） | 🔶 |
| A4 | 导语 | `#the_content #conTit div#subhead.subhead` | **剔除**（已定稿：导语不入正文；正文只由 A5 容器与 A8/A9 图说组成） | ✅ |
| A5 | 正文容器 | `#the_content div.content div.textbox > div#Main_Content_Val` | 段落/小节/图说只在此范围内；其 inline `background`（base64）样式丢弃 | ✅ |
| A6 | 正文段落 | A5 的直接子 `p` | 折叠空白；`<br>` 转段落分隔；连续空段落折叠 | ✅ |
| A7 | 小节标题 | `#Main_Content_Val h2.cx-app-content-subheads` | 文本，折叠空白；供判据 c 使用 | ✅ |
| A8 | 题图图说 | `#the_content div.media dl.media_pic > dd` | **保留为段落**；注文缺 `图：` 前缀时补 `图：`；同容器内 `img` 按 E12 剔除 | ✅ |
| A9 | 正文图图说 | `cximg div.article_img_talk` | 同 A8 | ✅ |

**剔除清单（在 `#the_content` 范围内整块 / 整节点移除）**

| # | 用途 | selector | 说明 | 状态 |
|---|---|---|---|---|
| E1 | 页面元信息 | `div#artInfo` | 来源、期次、「听报道」 | ✅ |
| E2 | AI 提问块 | `div#questions_container` | 含 `.hot_questions` | ✅ |
| E3 | 推荐位 / 相关阅读 | `div.pip` | 含 `.pip_mag_per` / `.pip_rel` / `.pip_ad` | ✅ |
| E4 | 分页控件区 | `div#pageNext` | 不是正文；但按 (3) 读作页面事实 | ✅ |
| E5 | 标签 | `div.content-tag` | | ✅ |
| E6 | 印刷版订阅提示 | `div.lanmu_textend` | **每个分页都有**，不可当文末判据 | ✅ |
| E7 | 更多报道 | `div.moreReport` | | ✅ |
| E8 | 版面编辑 | `div.idetor` | **每个分页都有**，不可当文末判据 | ✅ |
| E9 | 付费墙 | `div#chargeWall`（含 `div#pcapp`）、`div#pay-layer-ad`、`div#pay-layer-pro-ad`、`div#pay-box` | 整块剔除；可见性另按 (3)-F2 判定 | ✅ |
| E10 | AI 注入文本 | `p.aitt` | 每个分页一条（实测 5 条/篇），不剔除会污染每篇 EPUB | ✅ |
| E11 | 分页锚点 | `a[name^="page"]`、`anchor[id^="page"]` | 分页跳转锚，非正文 | ✅ |
| E12 | 媒体占位 | `img` / `picture` / `source` / `svg` / `video` / `audio` / `iframe` | 整节点剔除；容器因此变空则移除空容器 | ✅ |
| E13 | 脚本样式 | `script` / `style` | | ✅ |
| E14 | 结尾自链 | `a.end_ico` | 只保留其中文字（实测为空），丢弃 `href` | ✅ |
| E15 | 全部链接 | `a` | 保留文字、丢弃 `href` | ✅ |

#### (3) 页面事实（`extract` 产出 `model.PageFacts` → `verify.Complete` 入参；§6.2 步骤 6）

| # | 事实 | selector | 取值规则 | 状态 |
|---|---|---|---|---|
| F1 | 按钮集 | `#the_content div#pageNext div#pageBtn > a` | 取折叠空白后的文本集合（实测仅 `余下全文` / `下一页` / `上一页`）；`#pageBtn` 不存在或为空 → **空集** | ✅ |
| F2 | 付费墙可见性 | `#the_content div#chargeWall div#pcapp div#chargeWallContent` | `style` 属性去空白转小写后含 `display:none` → 不可见；否则可见；节点不存在 → 不可见 | ✅ |
| F3 | 小节表 | `#the_content div#pageNext ul#pageNav > li` | 逐项取 `a` 文本，去序号前缀 `^\s*\d{1,2}\s+`；`#pageNav` 不存在 → **空表** | ✅ |
| F4 | 正文小节 | 同 A7 | 与 F3 去前缀后逐项比对；F3 为空表时判据 c **自然成立** | ✅ |

#### (4) 登录 / 风控 — `internal/selector/login.go`（🔶 无快照，整节待验证）

| # | 用途 | 候选 selector | 状态 |
|---|---|---|---|
| L1 | 付费墙可见（首轮认证信号） | 同 F2 `div#chargeWallContent` | ✅ |
| L2 | 登录页 / 登录表单 | `form#loginForm`、`input[name*="password"]`、`.loginBox` | 🔶 |
| L3 | 验证码 | `iframe[src*="captcha"]`、`img[src*="captcha"]`、`#captchaImg` | 🔶 |

说明：期号页实测不设权限门槛，登录判据只落在**文章页**（§6.1 职责表、§8 页面事实 1）。L2/L3 需采集未登录 / 验证码样本后才能定稿——采集时的 `manifest.json` 中 `paywall` 选项为空，即缺此样本。

#### (5) 未决与待验证清单

1. 无分页文章：样本中不存在（`#pageBtn` 与 `#pageNav` 全缺），F1/F3 的「自然成立」分支无实测样本。
2. 列表页懒加载：实测 initial = settled = 24 条，滚动增长路径无样本；`scroll.go` 的终止条件只能靠纯逻辑用例覆盖。
3. 作者首选/回退顺序（A2/A3）：样本中两者同时存在且文本不同（`作者：罗子琳` vs `文｜财新周刊 罗子琳 发自韩国济州`），按 A2 优先实现，待第二篇样本复核。
4. 图说形态：样本只出现 `dl.media_pic > dd` 与 `cximg div.article_img_talk`；§4.2 另列的 `<figcaption>` 分支无样本。
5. L2/L3（登录页、验证码）无样本，实现时先按候选写并标记。

> 已定稿：HTML 解析库 = `goquery`（D9）；导语 `div#subhead` = 剔除（A4）。
> fixture 与判定特征的逐条对照见 `testdata/README.md` §2/§3；fixture 相对线上样本的合成改动见其 §5。

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
| W8 | 契约（接口由消费方定义） | `port` 把 `PageSource`/`Navigator`/`Prompter`/`TextStore` 从消费方包上移到中立包 | **这是被 Go 语言约束逼出的必要偏离**（审查意见 1）：接口匹配要求签名完全一致，多个消费方共享同一接口时，若各自定义就无法被同一实现满足。缓解方式：`port` 只放"被 ≥2 个包消费"的接口，形状仍由使用方共同决定；单一消费方的接口（`app.Reporter`/`app.StateRepository`/`service/article.ClickWaiter`）仍留在消费方包内。详见 §5 判定规则 |
| W9 | 契约/合规（spec 3.4「模拟真实点击」） | `adapter/page/execute.go` 在真实点击失败时会退回 DOM 级 `el.click()`（v1.8，来自 `caixin-snapshot` 实测） | **有意保留并记录**：`el.click()` 是 DOM API，不是财新内部 API，故不违反 spec 3.4「不直接调用内部 API」；而 chromedp 的真实点击在元素被遮挡/动画未结束时偶发失败，去掉兜底会让整篇被判失败、白耗重试额度。取舍：**真实点击优先，仅在失败后降级**，并在日志留痕（spec 3.4 已同步写入该降级） |
| W10 | 契约（v1.9 契约修订） | 冻结的契约缺了四类信息，Wave 1 实现时补：`config.Config.URL`、`ArticleState.author`、`ArtifactStore.EnsureWorkspace/CleanWorkspace`、`app.Deps.Nav`；另 `cli.Deps` 由 `{App,Checker}` 改为 `NewRuntime(cfg)` 工厂 | **改的是契约而非绕过契约**，四条都补的是"文档漏写、实现必需"的信息：① URL 是唯一位置参数，`app.Run(ctx,cfg)` 只有 cfg 能承载；② 重跑时正文与署名都从本地重建，state 不存 author 就无法写 EPUB 署名；③ §6.1 步骤 4 建目录与 `--full` 清理在冻结接口里没有落点；④ `Locate`/`Fetch` 消费的浏览器对象与 `Opener` 是同一实例，需显式注入。`cli.Deps` 的改动修的是装配时序矛盾（config 在 `cli.Run` 内产出、适配器在 `main` 构造），保留 `Run(args, deps)` 签名不变。详见 §5 约定与 v1.9 变更行 |

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
| 2 CLI 参数（URL、`--out`、`--no-kindle`、`--full`、`--kindle`、`--headless`、`--delay`、`--browser`） | `cli/flags.go` + `config`；Usage 含"勿操作窗口"提示、"`--delay` 单值等价 `N-N`"与"`--browser` 显式指定不回退"说明；`--no-kindle` **只跳过拷贝**，仍产出 EPUB + MOBI |
| 3.1 依赖检查与启动（退出码、profile 0700、stderr 日志） | `adapter/publish/probe.go`（Chromium 系三级定位）、`adapter/page/open.go`（启动、指纹抑制、SingletonLock 与崩溃标记、优雅关闭）、`cli/log.go` |
| 3.2 认证三层信号 | §6.1 职责表；`adapter/page/login.go` + `cli/prompt.go` + §7 超时兜底 |
| 3.3 浏览器模式 | `adapter/page/open.go`（headless 开关）；headless 下登录/验证码策略见 §6.1（M6） |
| 3.4 反爬与拟人化 | `adapter/clock/waiter.go` + `adapter/page/{open,execute}.go`（自动化指纹抑制；真实点击优先、失败降级 `el.click()`，见 §9.1 W9） |
| 3.5 selector 策略 | `internal/selector/{selector,issue,article,login}.go` 唯一定义（§8.1 定稿表）+ `testdata/` 快照 |
| 3.6 列表解析（滚动稳定、去重、期号） | `service/issue/parse.go`、`period.go`、`scroll.go` |
| 3.7 全文获取（两机制、拼接、50 次合计上限、最终成功判定） | `service/article/{navigate,accumulate,extract,verify}.go`（§6.2；最终判定见步骤 6） |
| 3.8 EPUB 组装（目录名、回退 slug、sanitize、元数据、输出结构） | `service/issue/period.go`、`service/ebook/epub.go`、`adapter/storage/workspace.go` |
| 3.9 转换与 Kindle 拷贝（不提前检测、三级探测、覆盖、未检测退出 0） | `service/ebook/command.go`、`service/kindle/rule.go`、`adapter/publish/*`；挂载点边界见 §7（C2 + 审查意见 10） |
| 4.1 增量与状态文件 | `service/state/*`、`adapter/storage/*`（原子写、写入顺序、**增量不检测线上更新**见 §7） |
| 4.2 `--full` | `app/app.go` |
| 4.3 失败重试与连败熔断 | `app/acquire.go` |
| 4.4 中途登录失效 | `adapter/page/{login,execute}.go`、`app/acquire.go` |
| 4.5 退出码与日志格式 | `cli/exit.go`、`cli/log.go`（stderr；分母为去重后篇数） |
| 5 非目标 | §1.2/§2 明确排除；无对应模块 |
| 6 待验证事项 | §8 末段 + `testdata/` 前置；KF7/KF8 型号确认只影响 `config.OutputProfile`（L4） |
| 7 验收标准 1-7 | §8 集成用例逐一对应；其中"每篇正文完整"以 §6.2 步骤 6 的**三条 DOM 事实**判定，并覆盖两条路线：`余下全文` 单击即完整、`下一页` 逐页点到按钮消失 |
| go-rules 架构规范 | §1.3、§5、§9.1、§9.4 |
