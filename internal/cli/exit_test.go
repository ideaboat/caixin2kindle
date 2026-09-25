package cli

import (
	"errors"
	"fmt"
	"testing"

	"caixin2kindle/internal/model"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "成功", err: nil, want: 0},
		{name: "参数错误", err: model.ErrUsage, want: 1},
		{name: "依赖缺失", err: model.ErrDependency, want: 1},
		{name: "登录失败或超时", err: model.ErrLogin, want: 2},
		{name: "抓取失败", err: model.ErrFetch, want: 3},
		{name: "转换失败", err: model.ErrConvert, want: 4},
		{name: "拷贝失败", err: model.ErrCopy, want: 5},
		{
			name: "包装后的抓取失败仍映射为 3",
			err:  fmt.Errorf("第 3 篇失败：%w", model.ErrFetch),
			want: 3,
		},
		{
			name: "双层包装的依赖缺失仍映射为 1",
			err:  fmt.Errorf("外层：%w", fmt.Errorf("内层：%w", model.ErrDependency)),
			want: 1,
		},
		{
			name: "未识别的错误按 1 处理",
			err:  errors.New("无法归类的错误"),
			want: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := ExitCode(tc.err)

			// Assert
			if got != tc.want {
				t.Fatalf("ExitCode(%v) = %d，期望 %d", tc.err, got, tc.want)
			}
		})
	}
}
