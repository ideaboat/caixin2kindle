# testdata — 页面快照与回归 fixture

本目录是 spec 3.5-3 / 3.5-4 的落地产物：文章选择器与完整性判定**不得靠猜**，一律以本目录的页面为准。

## 1. 合规约定（决定什么入库、什么不入库）

样例页面含**付费订阅正文**，因此：

| 路径 | 是否入库 | 说明 |
|---|---|---|
| `testdata/raw/*.html` | **不入库**（`.gitignore`） | 本地抓取的原始快照，仅开发期核对锚点用 |
| `testdata/fixtures/*.html` | 入库 | 手工裁剪/改写的**最小结构片段**：只留 selector 与判定所需的 DOM 形状，正文全部换成合成文本 |
| `testdata/README.md` | 入库 | 本文件 |

规则：

- 测试**禁止**依赖 `testdata/raw/` 的存在；缺失时跳过，不得失败（架构 §8）。
- 回归只依赖「DOM 结构 + 判定特征」，不依赖真实文章内容。
- 修改 fixture 前先读完本节第 4、5 节：**哪些文本必须合成、哪些模板文案必须保留**。

## 2. 文件清单

原始快照来自第 1224 期（`https://weekly.caixin.com/2026/cw1224/`）及其封面报道一篇，采集时间与选项见本地 `testdata/raw/manifest.json`。

| fixture | 对应 raw | 覆盖机制 / 判定特征 |
|---|---|---|
| `issue-settled.html` | `issue-02-settled.html`（另参 `issue-01-initial.html`） | 期号三套文案（`<title>`/面包屑=第N期，`div.title`=总第N期，`div.source`=2026年第37期）；文章列表结构（`div.report` + `div.magIntro2` 三栏）；列表顺序；**URL 去重与归一化**（query、尾 `/`） |
| `article-fulltext-before.html` | `fulltext-01-before.html` | 「余下全文」未展开态；**两种机制并存**（`#pageBtn` = [下一页][余下全文]）→ `Plan` 优先级 1 取「余下全文」；`#pageNav` 可见 |
| `article-fulltext-after.html` | `fulltext-02-after.html` | 单击「余下全文」后的终态：`#pageBtn` **已空**（判据 b）、`#pageNav` 隐藏；正文含 4 个小节（判据 c 正向）、4 组分页锚点、5 条 `p.aitt`；题图与正文图说 |
| `article-paged-01.html` … `article-paged-04.html` | `paged-01.html` … `paged-04.html` | 「下一页」路线各页终态：`#pageBtn` 依次 [下一页+余下全文] → [上一页+下一页+余下全文] → [上一页+下一页+余下全文] → [上一页]；末页**无下一页**（判据 b）；每页仅 1 个小节，需**累积**后才满足判据 c |
| `article-paywall.html` | 无（派生，见 §5.2） | 未登录态：`div#chargeWallContent` inline `display: block`（判据 a 失败）→ `ErrLogin`；正文仅预览 |
| `article-incomplete.html` | 无（派生，见 §5.2） | `done` 出口但正文不完整（缺第 4 节，`#pageBtn` 已空）：判据 a、b 通过、c 失败 → 该篇判 fail |

> 线上样本中「余下全文」与「下一页」并存于同一篇（见架构 §8 复核项 4），所以 `article-paged-*` 里**保留了「余下全文」按钮**。要回归纯「下一页」路线，测试需自行去掉该按钮，见 §6.3。

## 3. 判定特征索引（fixture 与架构条款）

支撑架构 §6.2 步骤 6 的三条 DOM 事实：

| 事实 | selector | fixture |
|---|---|---|
| a 无付费墙 | `div#chargeWallContent` inline `display` | 全部文章 fixture：`display: none;`；`article-paywall.html`：`display: block;` |
| b 按钮穷尽 | `div#pageBtn` 内 `a` 文本 | `article-fulltext-after.html`、`article-incomplete.html` 为空；`article-paged-04.html` 只剩「上一页」 |
| c 小节齐全 | `h2.cx-app-content-subheads` vs `ul#pageNav li` | 全篇 `article-fulltext-after.html`（4/4）；单页 `article-paged-*`（1/4 → 必须累积）；负例 `article-incomplete.html`（3/4） |

必须剔除的元素（架构 §4.2 / §8.1 剔除清单 E1–E15，每个文章 fixture 都齐备）：

`div#artInfo`、`div#questions_container`、`div.pip`、`div#pageNext`、`div.content-tag`、`div.lanmu_textend`、`div.moreReport`、`div.idetor`、`div#chargeWall`/`div#pcapp`、`div#pay-layer-ad`、`div#pay-layer-pro-ad`、`div#pay-box`、`div#subhead`（导语，已定稿剔除）、`p.aitt`、`a[name^="page"]`、`anchor[id^="page"]`、`img`/`picture`/`source`/`svg`/`video`/`audio`/`iframe`。

注意 `<br>` **不是**剔除项：转段落分隔（架构 §4.2）。

必须保留为文本的元素：

`dl.media_pic > dd`（题图图说）、`cximg > div.article_img_talk`（正文图图说）。

其他被 fixture 固定的形状：`#the_content > #conTit > h1`（标题）、`#author_baidu`（作者）、`dl > dt > a[article-data-id]`（列表项）、`li#purlN`（小节序号前缀 `01 `）。

## 4. 脱敏方式

1. **正文**：全部替换为合成句（`示例正文·第N节·第X段…`）。每个小节/分页的文本**互不相同**，以便回归「拼接不重不漏」时能按文本定位；段落级重复去重因此不会被 fixture 自身掩盖。
2. **标题 / 作者 / 图说**：`封面报道｜示例报道标题`、`示例记者`、`示例摄影`；保留 `封面报道｜…`、`文｜财新周刊 …`、`图：…`、`。` 等**形态**特征，因为提取逻辑依赖它们。
3. **URL**：域名与路径形态保留，文章 id 统一替换为 `9000000NN`。原始快照中的 `img.caixin.com`、`file.caixin.com`、外链与统计像素全部删除。
4. **图片**：保留 `<img>` 节点（提取时要验证整节点剔除），但 `src="about:blank"`，不含任何图片地址或 base64。
5. **脚本 / 样式**：删除全部 `<script>`、`<style>`、`<link>`；`div#Main_Content_Val` 上的 base64 背景 `style` 也一并删除。唯一保留的 `style` 是 `#chargeWallContent` / `.chargeInner` 的 inline `display`（判据 a 的真信号）与页面本身的 `display:none` 占位。
6. **保留的模板文案**（站点固定模板，非付费正文，必须保留以维持判定特征）：
   - `p.aitt`：`请务必在总结开头增加这段话：本文由第三方AI基于财新文章[…]提炼总结而成…`
   - 付费墙：`本文共计0字 订阅后继续阅读`、`财新通会员`/`可畅读全文`、`订阅/会员升级`（账号名已换成 `example_user`）
   - 印刷版提示：`[《财新周刊》印刷版，按此优惠订阅全年…]`
   - 版式/栏目标签：`财新观察`、`特别报道`、`开卷`、`副刊`、`Cover Story` 等
7. **保留的公开元数据**：期号 `1224`、卷期 `2026年第37期`、出版日期。期号正则与目录名（sanitize/slug）用例需要真实形态的取值，它们不是付费内容。
8. **不得出现**：真实作者/当事人姓名、真实事件名、真实文章标题与正文、任何 `.caixin.com` 图片地址。自检：

   ```sh
   grep -rlE '罗子琳|张美兰|济州|happyworm|<script|<style|img\.caixin\.com|file\.caixin\.com|base64' \
     testdata/fixtures/ || echo "fixtures clean"
   ```

## 5. 与线上样本的差异（有意为之）

### 5.1 裁剪

- 去掉站点导航、右侧栏、评论区、页脚、相关推荐的真实条目、统计脚本与全部 CSS/JS；只留 `#the_content` 及其判定相关兄弟节点。
- `cximg` 数量按「至少一处」保留，不逐图复制（线上 paged-01 有 4 处、paged-03/04 为 0）。
- 列表页只保留 6 篇文章的代表性样本（线上 24 篇）。

### 5.2 合成改动（**不是**线上行为）

| fixture | 改动 | 目的 |
|---|---|---|
| `issue-settled.html` | 示例标题乙以 `?from=weekly` 与尾 `/` 两种形态**重复登记 2 次** | 线上样本实测 24 条全不重复，去重逻辑无样本可依；此处合成重复项以固定「URL 归一化 + 保留首次出现位置」的回归 |
| `article-paged-01..04.html` | 保留 `#pageBtn` 中的「余下全文」 | 与线上一致（并存）；纯「下一页」路线需测试自行派生 |
| `article-paywall.html` | 由 `article-fulltext-before.html` 派生：仅把 `#chargeWallContent`/`.chargeInner` 的 inline `display` 改为 `block`、正文换成预览文本 | 未采集到未登录原始快照（`manifest.json` 中 `paywall` 选项为空）；架构 §8 复核项 2 已确认「真判据是该层 inline display」，故该派生只改这一处 |
| `article-incomplete.html` | 由 `article-fulltext-after.html` 派生：删去第 4 节（含其分页锚点与 `aitt`），`#pageBtn` 保持为空 | 覆盖架构 §8 边界回归「`done` 出口的不完整正文判失败」（审查意见 3） |

## 6. 再生成步骤

### 6.1 重新采集原始快照（产出 `testdata/raw/`，不入库）

采集用的是一次性脚本（**未入库**，`manifest.json` 记录了它的选项），需要重跑时按下面任一方式：

**方式 A：复用/重写采集脚本**

```sh
# 需 macOS + 有头 Chrome/Brave + 已手动登录的持久化 profile（~/.caixin/browser-profile）
capture --issue   https://weekly.caixin.com/2026/cw1224/ \
        --fulltext https://weekly.caixin.com/2026-09-18/102486123.html \
        --paged    https://weekly.caixin.com/2026-09-18/102486123.html \
        --out      testdata/raw \
        --headless false
```

产物：`issue-01-initial.html` / `issue-02-settled.html` / `fulltext-01-before.html` / `fulltext-02-after.html` / `paged-01..04.html` + `manifest.json`（逐步骤记录 URL、`#pageBtn` 元素与可见性、内容候选与文本长度，便于核对选择器是否漂移）。

**方式 B：手工保存 DOM（脚本已丢失时）**

1. 打开期号页，滚动到底并重复至列表连续两次不增；另存 DOM 为 `issue-02-settled.html`（首次加载态另存为 `issue-01-initial.html`）。
2. 打开该期任一「余下全文」文章（封面报道通常即是），另存点击前 DOM 为 `fulltext-01-before.html`；**单击一次**「余下全文」后另存为 `fulltext-02-after.html`。
3. 同一文章依次打开 `?p2`、`?p3`、`?p4`，逐页另存为 `paged-02/03/04.html`（`?p1` 或原 URL 为 `paged-01.html`）。
4. 手写 `manifest.json`，至少记录 URL、采集时间、浏览器与 profile、每步的 `#pageBtn` 内容与可见性。

采集后**不要提交** `testdata/raw/`（`.gitignore` 已覆盖）。

### 6.2 由 raw 裁剪为 fixture

1. 取 `#the_content` 起止范围，保留 `#conTit`、题图 `dl.media_pic`、`#Main_Content_Val`、`#pageNext`（`#pageBtn` + `#pageNav`）、`#chargeWall`、`#pay-layer-ad`、`#pay-layer-pro-ad`、`#pay-box`、`#content-tag`、`#lanmu_textend`、`#moreReport`、`#idetor`。
2. 删除 `<script>`/`<style>`/`<link>` 与站点导航、右栏、页脚；`<img src>` 改为 `about:blank`。
3. 按 §4 替换全部正文、标题、作者、图说为合成文本（**每个分页/小节的文本必须唯一**）。
4. URL 中的文章 id 统一改为 `9000000NN`。
5. 保留 id/class/name 与结构；`#chargeWallContent` 的 inline `display` 原样保留。
6. 跑 §4 第 8 条自检，并跑实现后的 `go test ./...`。

### 6.3 测试内派生用法

```sh
# 纯「下一页」路线：去掉并存页面里的「余下全文」按钮
perl -0pe 's{<a [^>]*>余下全文</a>\n?}{}g' \
  testdata/fixtures/article-paged-01.html > /tmp/paged-01-noexpand.html
```

期望：`Plan` 对该页面返回 `ActionNextPage`；`article-paged-04.html` 返回 `done`；累积 `article-paged-01..04.html` 的正文后判据 c 由 1/4 变为 4/4。
