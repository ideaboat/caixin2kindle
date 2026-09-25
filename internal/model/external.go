package model

// MissingDep 描述一项缺失的外部依赖及其安装提示。
type MissingDep struct {
	Name string // 依赖名，如 "Chromium 系浏览器"、“ebook-convert”
	Hint string // 安装/排查提示
}

// Volume 是一个候选挂载卷（只读探测结果）。
type Volume struct {
	Path         string // 挂载点，如 /Volumes/Kindle
	HasDocuments bool   // 是否含 documents/ 目录
	Writable     bool   // 是否可写
}

// KindleRequest 是设备定位的入参：Mount 为配置中的挂载点，Explicit 表示用户是否显式指定。
type KindleRequest struct {
	Mount    string // 挂载点路径（可能来自默认值）
	Explicit bool   // 是否由 --kindle 显式给出
}

// ConvertPlan 是一次 ebook-convert 调用的完整计划，由 service/ebook 纯函数产出、adapter 执行。
type ConvertPlan struct {
	InputPath     string   // 源 EPUB 路径
	OutputPath    string   // 目标 MOBI 路径
	OutputProfile string   // 传给 --output-profile，默认 "kindle"
	Args          []string // 完整参数列表（不含可执行文件名）
}
