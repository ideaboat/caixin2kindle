package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"caixin2kindle/internal/model"
)

func TestRedact_MasksCredentials(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		secret  string
		wantHas string
	}{
		{name: "Cookie 头", input: "Cookie: session=abc123; uid=42", secret: "abc123", wantHas: "Cookie=***"},
		{name: "Set-Cookie 头", input: "Set-Cookie: SID=zzz; Path=/", secret: "zzz", wantHas: "Set-Cookie=***"},
		{name: "Authorization 头", input: "Authorization: Bearer topsecret", secret: "topsecret", wantHas: "Authorization=***"},
		{name: "token 键值", input: "token=deadbeef", secret: "deadbeef", wantHas: "token=***"},
		{name: "access_token 键值", input: "access_token=aabbcc", secret: "aabbcc", wantHas: "access_token=***"},
		{name: "password 键值", input: "password: hunter2", secret: "hunter2", wantHas: "password=***"},
		{name: "api_key 键值", input: "api_key=keyvalue", secret: "keyvalue", wantHas: "api_key=***"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := Redact(tc.input)

			// Assert
			if strings.Contains(got, tc.secret) {
				t.Fatalf("Redact(%q) = %q，仍含凭据 %q", tc.input, got, tc.secret)
			}
			if !strings.Contains(got, tc.wantHas) {
				t.Fatalf("Redact(%q) = %q，期望包含 %q", tc.input, got, tc.wantHas)
			}
		})
	}
}

func TestRedact_KeepsPlainText(t *testing.T) {
	// Arrange
	input := "第 3/32 篇：财新周刊第1224期 封面报道"

	// Act
	got := Redact(input)

	// Assert
	if got != input {
		t.Fatalf("Redact(%q) = %q，非凭据文本应原样返回", input, got)
	}
}

func TestLogger_LevelsPrefixAndRedact(t *testing.T) {
	// Arrange
	var buf bytes.Buffer
	logger := &Logger{Out: &buf}

	// Act
	logger.Infof("第 %d/%d 篇：%s", 3, 32, "封面报道")
	logger.Warnf("cookie=%s", "secretcookie")
	logger.Hintf("请检查网络")

	// Assert
	got := buf.String()
	for _, want := range []string{"第 3/32 篇：封面报道", "警告：cookie=***", "提示：请检查网络"} {
		if !strings.Contains(got, want) {
			t.Fatalf("日志输出缺少 %q：%q", want, got)
		}
	}
	if strings.Contains(got, "secretcookie") {
		t.Fatalf("日志未脱敏：%q", got)
	}
}

func TestLogger_NilReceiverAndNilWriter(t *testing.T) {
	// Arrange
	var logger *Logger

	// Act / Assert（不 panic 即为通过）
	logger.Infof("nil receiver")
	logger.Warnf("nil receiver")
	logger.Hintf("nil receiver")
	(&Logger{}).Infof("nil writer")
}

func TestPrintErrorLine_AddsPrefixOnce(t *testing.T) {
	// Arrange
	var withPrefix, withoutPrefix bytes.Buffer

	// Act
	printErrorLine(&withPrefix, fmt.Errorf("%w：缺少期号页 URL 位置参数", model.ErrUsage))
	printErrorLine(&withoutPrefix, errors.New("冒烟错误"))

	// Assert
	if !strings.HasPrefix(withPrefix.String(), "参数错误：") {
		t.Fatalf("缺少前缀：%q", withPrefix.String())
	}
	if strings.Count(withPrefix.String(), "参数错误") != 1 {
		t.Fatalf("前缀重复：%q", withPrefix.String())
	}
	if !strings.HasPrefix(withoutPrefix.String(), "参数错误：冒烟错误") {
		t.Fatalf("未补前缀：%q", withoutPrefix.String())
	}
}

func TestPrintErrorLine_NilSafety(t *testing.T) {
	// Act / Assert（不 panic 即为通过）
	printErrorLine(nil, errors.New("x"))
	printErrorLine(&bytes.Buffer{}, nil)
}
