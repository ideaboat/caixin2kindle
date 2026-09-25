// Package ebook 是纯生成逻辑：不落盘、不起进程（§7 审查意见 8/9）。
// EPUB 字节由 Builder 产出，ebook-convert 的参数由 BuildConvertPlan 产出，两者都可在单测中断言。
//
// Wave 0 只冻结本包的跨包契约（Builder / BuildConvertPlan）。
package ebook

import "caixin2kindle/internal/model"

// Builder 实现 app.EPUBBuilder：把一期文章组装为 EPUB 3 字节。
type Builder struct{}

// NewBuilder 构造 EPUB 生成器。
func NewBuilder() *Builder {
	return &Builder{}
}

// Build 按当期目录顺序生成 EPUB 3（纯文字、无图片），返回文件名与完整字节。
// 元数据：语言 zh-CN、书名 = 期号、作者 = 财新周刊；每章含作者署名（spec 3.8）。
func (b *Builder) Build(issue model.Issue, texts []model.ArticleText) (string, []byte, error) {
	panic("TODO(wave1-C): 见 spec 3.8")
}

// BuildConvertPlan 构造一次 ebook-convert 调用的完整计划（纯函数，不执行）。
// 命令形如：ebook-convert <期号>.epub <期号>.mobi --output-profile kindle（spec 3.9）。
func BuildConvertPlan(inputPath, outputPath, outputProfile string) model.ConvertPlan {
	panic("TODO(wave1-C): 见 spec 3.9")
}
