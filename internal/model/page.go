package model

import "time"

// ActionKind 是 PageAction 的动作种类，也是单篇点击计数的口径（Expand 与 NextPage 合计）。
type ActionKind int

const (
	// ActionScrollToBottom 滚动到页面底部，用于列表懒加载；不计入单篇点击上限。
	ActionScrollToBottom ActionKind = iota + 1
	// ActionExpandFullText 点击「余下全文」；原地展开，不换页。
	ActionExpandFullText
	// ActionNextPage 点击「下一页」；换页，离开当前页前须归档其终态。
	ActionNextPage
)

// PageAction 是 service → 浏览器 的唯一指令载体。
type PageAction struct {
	Kind     ActionKind    // 动作种类
	Selector string        // 取自 internal/selector；service 与 adapter 读取同一张表
	MaxWait  time.Duration // 元素可见/网络空闲的等待上限
}

// PageFacts 是 extract 从单个页面快照读出的“页面事实”，只服务 verify.Complete（§6.2 步骤 6）。
// 判据 a/b/c 需要正文之外的信息，故与段落分开产出（取自 §8.1(3) 的 F1–F4）。
type PageFacts struct {
	Buttons        []string // F1：#pageBtn > a 的折叠空白文本集合；#pageBtn 不存在或为空 → 空集
	PaywallVisible bool     // F2：#chargeWallContent 的 inline style 含 display:none → false；节点不存在 → false
	NavSections    []string // F3：#pageNav > li > a 文本去序号前缀（^\s*\d{1,2}\s+）；#pageNav 不存在 → 空表
	BodySections   []string // F4：正文中的小节标题（同 A7）；判据 c 用累积后的全集与之比对
}

// StepKind 是 NavProgress.Steps 中一步的种类。
type StepKind int

const (
	// StepNavigate 导航到目标 URL。
	StepNavigate StepKind = iota + 1
	// StepExpand 点击「余下全文」。
	StepExpand
	// StepNextPage 点击「下一页」。
	StepNextPage
	// StepScroll 滚动页面。
	StepScroll
	// StepSnapshot 抓取页面快照。
	StepSnapshot
	// StepDone 导航正常结束。
	StepDone
	// StepFail 导航失败结束。
	StepFail
)

// Step 是单篇导航状态机中的一步，供审计与复现。
type Step struct {
	Kind      StepKind // 步骤种类
	PageIndex int      // 该步发生时的正文页序号，从 1 开始
	Selector  string   // 该步使用的 selector（导航步为空）
	BodyHash  string   // 终态快照后该页正文哈希，用于诊断
	Err       string   // 失败原因；成功步为空
}

// NavProgress 是单篇导航的完整可审计记录：状态机每一步都可回放。
type NavProgress struct {
	Steps            []Step // 按发生顺序
	PageIndex        int    // 已进入的正文页序号，从 1 开始
	ClicksOnArticle  int    // 单篇 Expand+NextPage 合计点击数（上限 ArticleClickLimit）
	LastArchivedPage int    // 已归档到第几页（0 表示尚未归档任何页）
}
