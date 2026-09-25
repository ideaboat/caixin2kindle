package model

import "errors"

// 哨兵错误：全部用 %w 包装，cli/exit.go 用 errors.Is 做唯一退出码映射（§6.1、§7）。
var (
	// ErrUsage 表示参数错误或清理失败，退出码 1。
	ErrUsage = errors.New("参数错误")
	// ErrDependency 表示依赖缺失或浏览器启动失败（含 profile 被占用），退出码 1。
	ErrDependency = errors.New("依赖缺失")
	// ErrLogin 表示登录失败或超时，退出码 2。
	ErrLogin = errors.New("登录失败或超时")
	// ErrFetch 表示抓取失败（含连败熔断、点击超限、完整性判定未通过），退出码 3。
	ErrFetch = errors.New("抓取失败")
	// ErrConvert 表示 EPUB/MOBI 转换失败，退出码 4。
	ErrConvert = errors.New("转换失败")
	// ErrCopy 表示 Kindle 复制失败，退出码 5。
	ErrCopy = errors.New("拷贝失败")
)
