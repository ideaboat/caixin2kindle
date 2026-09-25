package model

// Result 是 app.Run 的返回值，供 cli 渲染到 stdout。
type Result struct {
	OutputDir    string // 输出目录 <out>/<期号>/
	EPUBPath     string // 产出的 EPUB 路径
	MOBIPath     string // 产出的 MOBI 路径
	KindleCopied bool   // 是否已拷贝到 Kindle
	Skipped      bool   // 是否因增量而跳过全部抓取与重建
}
