package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Logger 是 cli 的最小日志底座：所有日志集中经它写 stderr，并在写出前统一脱敏（架构 §7 凭据安全）。
// Out 由调用方注入：生产为 stderr，测试为 bytes.Buffer。
type Logger struct {
	Out io.Writer
}

// Infof 写普通日志（进度行等），不额外添加前缀。
func (l *Logger) Infof(format string, args ...any) {
	if l == nil {
		return
	}
	writeRedacted(l.Out, format, args...)
}

// Warnf 写警告日志，固定以“警告：”开头。
func (l *Logger) Warnf(format string, args ...any) {
	if l == nil {
		return
	}
	writeRedacted(l.Out, "警告："+format, args...)
}

// Hintf 写排查提示，固定以“提示：”开头。
func (l *Logger) Hintf(format string, args ...any) {
	if l == nil {
		return
	}
	writeRedacted(l.Out, "提示："+format, args...)
}

// usagePrefix 是参数错误日志的强制首行前缀，用于与依赖缺失区分（架构 §9.3 C4）。
const usagePrefix = "参数错误"

// printErrorLine 把 err 以“参数错误：…”写入 w；err 已带同名前缀时不重复添加，避免双前缀。
func printErrorLine(w io.Writer, err error) {
	if w == nil || err == nil {
		return
	}
	message := err.Error()
	if !strings.HasPrefix(message, usagePrefix) {
		message = usagePrefix + "：" + message
	}
	writeRedacted(w, "%s", message)
}

// writeRedacted 是日志唯一出口：先格式化、再脱敏、最后补换行；writer 为 nil 时安全丢弃。
func writeRedacted(workWriter io.Writer, format string, args ...any) {
	if workWriter == nil {
		return
	}
	fmt.Fprintln(workWriter, Redact(fmt.Sprintf(format, args...)))
}

// credentialPatterns 是两类凭据写法：头式（Cookie/Authorization/Set-Cookie，值取到行尾）
// 与键值式（token=/password= 等）。键值式候选按“先长后短”排列，避免 access_token 被 token 抢先匹配。
var credentialPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{
		pattern:     regexp.MustCompile(`(?i)\b(cookie|set-cookie|authorization|proxy-authorization)\s*[:=]\s*[^\r\n]+`),
		replacement: "${1}=***",
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b(access[_-]?token|refresh[_-]?token|auth[_-]?token|token|password|passwd|pwd|secret|api[_-]?key|apikey|credential|credentials|session[_-]?id)\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;&]+)`),
		replacement: "${1}=***",
	},
}

// Redact 遮蔽文本中的凭据片段：Cookie/Authorization 头的值，以及 token/password/api_key 等键值对的值
// 一律替换为 ***。不含凭据的文本原样返回，便于安全地写入日志。
func Redact(text string) string {
	for _, rule := range credentialPatterns {
		text = rule.pattern.ReplaceAllString(text, rule.replacement)
	}
	return text
}
