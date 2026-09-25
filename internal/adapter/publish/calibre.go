package publish

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

// defaultConvertTimeout 是 ebook-convert 的默认执行上限：一期杂志转换通常只需数秒，10 分钟足以兜底卡死。
const defaultConvertTimeout = 10 * time.Minute

// convertOutputTailLimit 是失败诊断中保留的工具输出尾部字节上限。
const convertOutputTailLimit = 2000

// noOutputPlaceholder 是工具没有任何输出时写入错误信息的占位文案。
const noOutputPlaceholder = "（无输出）"

// Converter 实现 app.Converter：执行 ebook-convert 并把任何失败归类为 model.ErrConvert。
// 类型无隐藏状态，测试注入 Run 即可在不启动 calibre 的前提下覆盖全部路径。
type Converter struct {
	Path    string                                                                 // ebook-convert 可执行文件；为空表示未找到
	Timeout time.Duration                                                          // 单次执行上限，默认 10m；≤0 表示取默认
	Run     func(ctx context.Context, name string, args ...string) ([]byte, error) // nil → exec.CommandContext 的 CombinedOutput
}

// NewConverter 构造转换器：先 exec.LookPath("ebook-convert")，再回退 macOS 应用包路径（C3）。
// cfg 目前不携带 ebook-convert 候选字段（config 冻结），保留入参以与其它适配器构造签名一致。
func NewConverter(cfg config.Config) *Converter {
	return &Converter{
		Path:    resolveEbookConvert(),
		Timeout: defaultConvertTimeout,
	}
}

// resolveEbookConvert 按 C3 顺序解析 ebook-convert：PATH 优先，其次 macOS 应用包内路径；都未命中返回空串。
func resolveEbookConvert() string {
	if path, err := exec.LookPath(ebookConvertExecutable); err == nil {
		return path
	}
	if info, err := os.Stat(calibreBundlePath); err == nil && !info.IsDir() {
		return calibreBundlePath
	}
	return ""
}

// ToMOBI 执行一次 ebook-convert 转换（spec 3.9）；plan.Args 是不含可执行文件名的完整参数表。
// 成功返回 nil。可执行文件缺失、计划缺少参数、启动失败、非零退出或超时，一律返回包装
// model.ErrConvert 的错误，并附带工具输出的尾部片段（≤2000 字节）便于用户排查。
func (c *Converter) ToMOBI(ctx context.Context, plan model.ConvertPlan) error {
	if c.Path == "" {
		return fmt.Errorf("%w：未找到 ebook-convert，请安装 calibre 或将其加入 PATH", model.ErrConvert)
	}
	if len(plan.Args) == 0 {
		return fmt.Errorf("%w：转换计划缺少参数（输入 %s）", model.ErrConvert, plan.InputPath)
	}

	runCtx, cancel := context.WithTimeout(ctx, c.effectiveTimeout())
	defer cancel()

	output, err := c.run()(runCtx, c.Path, plan.Args...)
	if err != nil {
		tail := tailOutput(output, convertOutputTailLimit)
		if tail == "" {
			tail = noOutputPlaceholder
		}
		return fmt.Errorf("%w：ebook-convert 执行失败（%w）：%s", model.ErrConvert, err, tail)
	}
	return nil
}

// effectiveTimeout 返回实际生效的执行上限：Timeout ≤ 0 时取默认 10 分钟。
func (c *Converter) effectiveTimeout() time.Duration {
	if c.Timeout <= 0 {
		return defaultConvertTimeout
	}
	return c.Timeout
}

// run 返回生效的执行函数：测试注入 c.Run，生产环境走 exec.CommandContext + CombinedOutput。
func (c *Converter) run() func(context.Context, string, ...string) ([]byte, error) {
	if c.Run != nil {
		return c.Run
	}
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
}

// tailOutput 把工具输出整理为诊断文本：去除首尾空白，超过 limit 字节时只保留末尾（按 rune 边界对齐，
// 不切断 UTF-8）并前置一行截断提示；空输出与 limit ≤ 0 返回空串。
func tailOutput(output []byte, limit int) string {
	if limit <= 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(output))
	if len(trimmed) <= limit {
		return trimmed
	}

	tail := trimmed[len(trimmed)-limit:]
	for len(tail) > 0 && !utf8.RuneStart(tail[0]) {
		tail = tail[1:]
	}
	return "…（输出已截断，仅保留末尾 " + strconv.Itoa(limit) + " 字节）\n" + tail
}
