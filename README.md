# caixin2kindle

[English README](README.en.md)

把《财新周刊》某一期变成 Kindle 里的 EPUB + MOBI。

```
caixin2kindle https://weekly.caixin.com/2026/cw1224/
```

## 速览

| 项 | 说明 |
|---|---|
| 语言 | Go 1.27 |
| 平台 | macOS |
| 依赖 | Chromium 系浏览器、calibre (`ebook-convert`) |
| 输出 | `<out>/<期号>/` 目录下的 EPUB 和 MOBI |

## 安装

```bash
go build -o caixin2kindle .
```

确保 `ebook-convert` 在 PATH 中，Chromium 系浏览器可用。

## 用法

```
caixin2kindle [参数] <期号页 URL>

参数：
  --out <dir>          父目录（默认 ~/Downloads/caixin）
  --kindle <mount>     Kindle 挂载点（默认 /Volumes/Kindle）
  --browser <path>     浏览器路径（默认自动探测）
  --delay "M-N"        篇间随机等待秒数区间（默认 "3-8"）
  --no-kindle          只产出 EPUB + MOBI
  --full               全量重跑
  --headless           无头模式
  -h, --help           显示帮助
```

## 流程

1. 启动浏览器 → 访问期号页 → 解析文章列表
2. 逐篇点击展开全文 → 提取纯文字
3. 组装 EPUB → `ebook-convert` 转 MOBI → 拷贝到 Kindle

首次运行需手动登录（持久化 profile 保存在 `~/.caixin/browser-profile`）。运行期间请勿操作浏览器窗口。

## 退出码

| 码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 参数错误 / 依赖缺失 / 浏览器启动失败 |
| 2 | 抓取失败 |
| 3 | 构建失败 |
| 4 | Kindle 检测失败 |
| 5 | 其他错误 |

## 许可

MIT
