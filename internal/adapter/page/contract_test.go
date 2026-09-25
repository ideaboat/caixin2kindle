package page

import (
	"context"

	"caixin2kindle/internal/config"
	"caixin2kindle/internal/port"
)

// 本文件把 Client 与消费方接口的形状钉在编译期：任一处签名漂移都会让 go test 构建失败。
// app.BrowserOpener / app.LoginHandler 定义在 app 包，而 adapter 不得反向导入 app（§3 依赖表），
// 故这里用结构等价的本地接口断言，效果与 wire 中的断言一致。
type browserOpenerContract interface {
	Open(ctx context.Context, cfg config.Config) error
}

type loginHandlerContract interface {
	EnsureReady(ctx context.Context) error
}

var (
	_ port.PageSource       = (*Client)(nil)
	_ port.Navigator        = (*Client)(nil)
	_ browserOpenerContract = (*Client)(nil)
	_ loginHandlerContract  = (*Client)(nil)
)
