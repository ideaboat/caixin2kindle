package cli

import (
	"errors"

	"caixin2kindle/internal/model"
)

// ExitCode 把哨兵错误映射为退出码 0–5，是全项目**唯一**的映射点（§6.1、§7）。
// nil → 0；ErrUsage/ErrDependency → 1；ErrLogin → 2；ErrFetch → 3；ErrConvert → 4；ErrCopy → 5。
// 未被识别的错误按 1 处理（属编程错误，不应出现）。
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, model.ErrUsage), errors.Is(err, model.ErrDependency):
		return 1
	case errors.Is(err, model.ErrLogin):
		return 2
	case errors.Is(err, model.ErrFetch):
		return 3
	case errors.Is(err, model.ErrConvert):
		return 4
	case errors.Is(err, model.ErrCopy):
		return 5
	default:
		return 1
	}
}
