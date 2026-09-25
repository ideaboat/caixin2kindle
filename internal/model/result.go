package model

// Result 是 app.Run 的返回值，供 cli 渲染到 stdout。
type Result struct {
	OutputDir       string   // 输出目录 <out>/<期号>/
	EPUBPath        string   // 产出的 EPUB 路径
	MOBIPath        string   // 产出的 MOBI 路径
	KindleCopied    bool     // 是否已拷贝到 Kindle
	Skipped         bool     // 是否因增量而跳过全部抓取与重建
	MissingArticles []string // 重试后仍未抓到的篇目（降级构建时非空，v1.9.1）
	SkippedArticles []string // 按规则有意跳过的篇目，如图片型栏目（v1.9.2）
	SkipNote        string   // 有意跳过的原因说明；无跳过时为空
}
