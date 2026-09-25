// Package selector 是 selector 的唯一真相源：纯数据表，不含行为。
// 解析方（service/*）与执行方（adapter/page）读取同一张表，页面改版只需改这一处（spec 3.5-2）。
//
// 取值通用约定（architecture.md §8.1）：
//   - 文本取值一律先折叠空白（strings.Fields + 单空格连接，覆盖全角空格与 &nbsp;）；
//   - class 按空格分隔的 token 匹配，不做整串相等；
//   - selector 子集只用 标签 / #id / .class / [attr] / [attr^=] / 直接子 >；
//     禁止 :visible、:has() 等依赖浏览器计算态的写法；
//   - 可见性一律读 inline style，不做 CSS 层叠计算；
//   - 文章页一切提取限定在 #the_content 之内。
package selector

// Set 是一份完整的 selector 表。默认值见 Default，必须与 architecture.md §8.1 逐字一致。
type Set struct {
	Issue   Issue   // 期号页
	Article Article // 文章页（含剔除清单与页面事实）
	Login   Login   // 登录页与风控
}

// Default 返回与 architecture.md §8.1 定稿表逐字一致的 selector 集合。
func Default() Set {
	return Set{
		Issue:   defaultIssue(),
		Article: defaultArticle(),
		Login:   defaultLogin(),
	}
}

// Issue 是期号页的 selector（§8.1(1)）。
type Issue struct {
	PeriodTitle       string   // I1 期号（首选）
	PeriodHeadTitle   string   // I2 期号（回退 1）
	PeriodBreadcrumb  string   // I3 期号（回退 2），取最后一个元素的文本
	VolumeSource      string   // I4 卷期（仅日志/校验）
	VolumeDate        string   // I4 卷期日期（仅日志/校验）
	VolumeArticleInfo string   // I4 文章页 div#artInfo（仅日志/校验）
	EntryContainers   []string // I5 文章条目容器，按 DOM 顺序
	EntryLink         string   // I6 条目链接
	EntryTitle        string   // I7 条目标题
	EntryByline       string   // I8 条目署名（仅日志）
	EntrySummary      string   // I8 条目摘要（仅日志）
	ColumnName        string   // I9 栏目名
	ExcludeLinkScopes []string // I10 一律排除的非文章链接/容器（文档性约束，解析只认 I6）
}

// Article 是文章页的 selector（§8.1(2) 与 §8.1(3)）。
// Paragraph 与 SectionHead 是相对于 Body 容器的相对 selector，其余为绝对 selector。
type Article struct {
	Title          string   // A1 标题
	TitleStrips    []string // A1 标题内需剔除的节点
	Author         string   // A2 作者（首选）
	AuthorFallback string   // A3 作者（回退）
	Subhead        string   // A4 导语：整块剔除，不入正文
	Body           string   // A5 正文容器
	Paragraph      string   // A6 正文段落：Body 的直接子
	SectionHead    string   // A7 小节标题
	LeadCaption    string   // A8 题图图说：保留为段落
	ImageCaption   string   // A9 正文图图说：保留为段落

	RemoveBlocks      []string // E1–E13 整块剔除；E4 div#pageNext 须在读取页面事实之后才移除
	RemoveMedia       []string // E12 媒体占位：整节点剔除
	RemovePageAnchors []string // E11 分页跳转锚
	UnwrapLinks       []string // E14/E15 保留文字、丢弃 href

	PageBtnContainer string // F1 按钮容器，供生成 :nth-of-type 点击目标
	PageButtons      string // F1 按钮集
	PagePaywall      string // F2 付费墙可见性判据
	PageNavSections  string // F3 小节表

	LabelExpand   string // 「余下全文」按钮文案
	LabelNextPage string // 「下一页」按钮文案
	LabelPrevPage string // 「上一页」按钮文案（仅用于识别，不作为动作）
}

// Login 是登录页与风控的 selector（§8.1(4)，L2/L3 无快照样本、待第二样本复核）。
type Login struct {
	Paywall string   // L1 付费墙可见（首轮认证信号）
	Form    []string // L2 登录页 / 登录表单
	Captcha []string // L3 验证码
}
