package main

import (
	"errors"
	"os"
	"testing"

	"caixin2kindle/internal/model"
)

// smokeIssueURL 是冒烟用例使用的位置参数（不会真正联网：三条路径都在触达浏览器之前结束）。
const smokeIssueURL = "https://weekly.caixin.com/2026/cw1224/"

// silenceStdio 把进程级 stdout/stderr 临时指向 /dev/null，避免测试输出 Usage 文案。
func silenceStdio(t *testing.T) {
	t.Helper()

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("打开 %s 失败：%v", os.DevNull, err)
	}
	originalStdout, originalStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = devNull, devNull
	t.Cleanup(func() {
		os.Stdout, os.Stderr = originalStdout, originalStderr
		devNull.Close()
	})
}

// TestRunSmokeAssembly 覆盖入口装配的三条安全路径：全部在启动浏览器之前结束，
// 因此可以在单测中真实调用 run()（不联网、不启动浏览器），验证组合根与依赖检查确实接通。
func TestRunSmokeAssembly(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want error
	}{
		{name: "缺少位置参数按参数错误退出", args: []string{"caixin2kindle"}, want: model.ErrUsage},
		{name: "--help 正常返回", args: []string{"caixin2kindle", "--help"}, want: nil},
		{
			name: "显式浏览器路径无效按依赖缺失退出",
			args: []string{"caixin2kindle", "--browser", "/nonexistent/caixin2kindle-browser", smokeIssueURL},
			want: model.ErrDependency,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			silenceStdio(t)
			originalArgs := os.Args
			os.Args = tc.args
			t.Cleanup(func() { os.Args = originalArgs })

			// Act
			err := run()

			// Assert
			if tc.want == nil {
				if err != nil {
					t.Fatalf("期望成功，实际：%v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("错误应包装 %v，实际：%v", tc.want, err)
			}
		})
	}
}
