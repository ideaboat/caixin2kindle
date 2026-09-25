package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
)

// errorReader 模拟读取失败，用于覆盖“无法确认”分支。
type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("读取失败") }

func TestPrompter_WaitForLogin(t *testing.T) {
	// Arrange
	var out bytes.Buffer
	prompter := &Prompter{Out: &out, In: strings.NewReader("确认\n")}

	// Act
	err := prompter.WaitForLogin(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("WaitForLogin 返回错误：%v", err)
	}
	if !strings.Contains(out.String(), "请在浏览器窗口中完成登录，完成后回到终端按回车继续") {
		t.Fatalf("登录提示文案错误：%q", out.String())
	}
}

func TestPrompter_WaitForCaptcha(t *testing.T) {
	cases := []struct {
		name string
		in   io.Reader
	}{
		{name: "按回车确认", in: strings.NewReader("\n")},
		{name: "EOF 视作已确认", in: strings.NewReader("")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var out bytes.Buffer
			prompter := &Prompter{Out: &out, In: tc.in}

			// Act
			err := prompter.WaitForCaptcha(context.Background())

			// Assert
			if err != nil {
				t.Fatalf("WaitForCaptcha 返回错误：%v", err)
			}
			if !strings.Contains(out.String(), "请在浏览器窗口完成验证后按回车继续") {
				t.Fatalf("验证码提示文案错误：%q", out.String())
			}
		})
	}
}

func TestPrompter_CanceledContext(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	prompter := &Prompter{Out: &out, In: strings.NewReader("\n")}

	// Act
	err := prompter.WaitForLogin(ctx)

	// Assert
	if !errors.Is(err, model.ErrLogin) {
		t.Fatalf("已取消 ctx 应返回 ErrLogin，实际：%v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("ctx 已取消时不应输出提示：%q", out.String())
	}
}

func TestPrompter_ReadError(t *testing.T) {
	// Arrange
	var out bytes.Buffer
	prompter := &Prompter{Out: &out, In: errorReader{}}

	// Act
	err := prompter.WaitForCaptcha(context.Background())

	// Assert
	if !errors.Is(err, model.ErrLogin) {
		t.Fatalf("读取失败应返回 ErrLogin，实际：%v", err)
	}
}

func TestPrompter_NilSafety(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	nilReceiver := (*Prompter)(nil).WaitForLogin(ctx)
	nilInput := (&Prompter{Out: io.Discard}).WaitForCaptcha(ctx)

	// Assert
	if !errors.Is(nilReceiver, model.ErrLogin) {
		t.Fatalf("nil Prompter 应返回 ErrLogin，实际：%v", nilReceiver)
	}
	if !errors.Is(nilInput, model.ErrLogin) {
		t.Fatalf("nil In 应返回 ErrLogin，实际：%v", nilInput)
	}
}
