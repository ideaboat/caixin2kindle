package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/model"
)

// usageText 是 --help 的完整文案：用法、参数表、默认值与运行须知（spec 2、3.3、4.1；架构 §7）。
// 其中五条属于必须写明的约定：运行期间勿操作自动化窗口、首次运行需手动登录、
// --delay 单值等价 N-N、--browser 显式无效不回退、增量语义边界。
const usageText = `caixin2kindle — 下载《财新周刊》某期并推送到 Kindle

用法：
    caixin2kindle [参数] <期号页 URL>

位置参数：
    <期号页 URL>          必填且唯一，例如 https://weekly.caixin.com/2026/cw1224/

参数：
    --out <dir>           父目录：输出到 <out>/<期号>/（默认 ~/Downloads/caixin）
    --kindle <mount>      Kindle 挂载点（默认 /Volumes/Kindle）
    --browser <path>      浏览器可执行文件路径（默认自动探测）
    --delay "M-N"         相邻两篇文章之间的随机等待秒数区间（默认 "3-8"）
    --no-kindle           只产出 EPUB + MOBI，不拷贝到 Kindle（默认关闭）
    --full                全量重跑：忽略已有进度，重抓全部文章并重建 EPUB/MOBI（默认关闭，缺省为增量）
    --headless            无头模式运行浏览器（默认关闭；风控风险更高，无法手动处理验证码）
    -h, --help            显示本帮助

默认值：
    --out ~/Downloads/caixin    --kindle /Volumes/Kindle    --delay "3-8"
    增量运行、有头模式、以及“检测到设备则拷贝到 Kindle”均为缺省行为。

说明：
    * 运行期间请勿操作自动化窗口。
    * 首次运行需在浏览器中手动登录。
    * --delay 单值等价 "N-N"，例如 "5" 等价 "5-5"。
    * --browser 显式指定但路径无效会报错且不回退自动探测。
    * 增量语义：跳过判定只证明本地正文文件未被改动，不检测线上更新；怀疑线上有更新请用 --full。
`

// parseFlags 解析命令行参数并校验位置参数。
// 返回的 overrides 只包含“真正出现”的参数：用 fs.Visit 判断，未出现的项保持 nil，保持 config 默认值。
// help 为 true 表示用户请求 --help；解析失败或位置参数非法时返回包装 model.ErrUsage 的错误。
func parseFlags(args []string, stderr io.Writer) (config.Overrides, bool, error) {
	fs := flag.NewFlagSet("caixin2kindle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {} // 帮助与错误文案由本包统一渲染，关闭 flag 的默认输出

	out := fs.String("out", "", "父目录")
	kindle := fs.String("kindle", "", "Kindle 挂载点")
	browser := fs.String("browser", "", "浏览器可执行文件路径")
	delay := fs.String("delay", "", "篇间随机等待区间")
	noKindle := fs.Bool("no-kindle", false, "不拷贝到 Kindle")
	full := fs.Bool("full", false, "全量重跑")
	headless := fs.Bool("headless", false, "无头模式")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config.Overrides{}, true, nil
		}
		return config.Overrides{}, false, fmt.Errorf("%w：%v", model.ErrUsage, err)
	}

	var overrides config.Overrides
	fs.Visit(func(visited *flag.Flag) {
		switch visited.Name {
		case "out":
			overrides.Out = out
		case "kindle":
			overrides.Kindle = kindle
		case "browser":
			overrides.Browser = browser
		case "delay":
			overrides.Delay = delay
		case "no-kindle":
			overrides.NoKindle = noKindle
		case "full":
			overrides.Full = full
		case "headless":
			overrides.Headless = headless
		}
	})

	positional, err := singleURL(fs)
	if err != nil {
		return config.Overrides{}, false, err
	}
	overrides.URL = &positional
	return overrides, false, nil
}

// singleURL 校验位置参数：必须且只能有一个期号页 URL，违反时返回包装 model.ErrUsage 的错误。
func singleURL(fs *flag.FlagSet) (string, error) {
	switch {
	case fs.NArg() == 0:
		return "", fmt.Errorf("%w：缺少期号页 URL 位置参数", model.ErrUsage)
	case fs.NArg() > 1:
		return "", fmt.Errorf("%w：位置参数只能有一个期号页 URL，收到 %d 个：%v", model.ErrUsage, fs.NArg(), fs.Args())
	default:
		return fs.Arg(0), nil
	}
}
