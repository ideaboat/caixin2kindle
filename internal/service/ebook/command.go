package ebook

import "caixin2kindle/internal/model"

// BuildConvertPlan 构造一次 ebook-convert 调用的完整计划（纯函数，不执行）。
// 命令形如：ebook-convert <期号>.epub <期号>.mobi --output-profile kindle（spec 3.9）。
// 进程执行与错误分类归 adapter/publish，本函数不碰 IO。
func BuildConvertPlan(inputPath, outputPath, outputProfile string) model.ConvertPlan {
	return model.ConvertPlan{
		InputPath:     inputPath,
		OutputPath:    outputPath,
		OutputProfile: outputProfile,
		Args:          []string{inputPath, outputPath, "--output-profile", outputProfile},
	}
}
