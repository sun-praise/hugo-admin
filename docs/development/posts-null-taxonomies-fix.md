# /api/posts 空标签编成 null 导致文章列表白屏

## 现象

打开文章列表页（`Posts`）报 `TypeError: can't access property "length", e.tags is null`，
页面白屏。仪表板读同一接口，遇到同一篇文章同样会在 `post.tags.slice(0, 2)` 崩掉。

## 根因复盘四问

**怎么发生**：文章的 YAML frontmatter 没写 `tags` / `categories`。Go 侧
`internal/posts/posts.go` 的 `strList()` 对缺失值返回 `nil` 的 `[]string`，
`PostItem` 原样搬运，`encoding/json` 把 nil 切片编成 `"tags":null`
（Python `jsonify` 版本输出的是 `[]`）。前端 `types/index.ts` 把 `Post.tags`
声明成 `string[]`，`Posts.tsx` 读 `post.tags.length`、`Dashboard.tsx` 读
`post.tags.slice(0, 2)`，两者都没有空值分支，于是整个列表渲染中断。

**在哪引入**：Go 重写 `posts` 包时 `strList()` 的 `default` 分支返回 `nil`，
与函数注释「对齐 Python：缺失为空列表」自相矛盾；`GetPosts()` 组装
`PostItem` 时没有在序列化边界兜底。前端把「后端一定返回数组」当成隐含契约，
`get<T>()` 是裸 `response.json()`，没有任何形状校验。

**为什么没拦住**：`internal/posts/posts_test.go` 的 `TestPaginationEmpty`
只钉住 `PostsResult.Posts` 非 nil，没有钉住每个 `PostItem` 的
`tags` / `categories`；`seedPagination` 给每篇 fixture 都写了标签和分类，
「无标签文章」这条路径从未被 Go 测试覆盖。前端单测只覆盖纯工具函数，
没有 `/api/posts` 响应归一化的用例。CI 的 Go 测试在 Linux 跑，浏览器端
解引用崩溃只能靠人工打开列表页发现。

**是否沉淀**：

- `strList()` 改为返回非 nil 空切片；新增 `nonNilStrings()` 在 `GetPosts()`
  序列化边界再挡一次，避免将来从其它路径构造 `BlogPost` 时漏出 null。
- 新增 `TestGetPostsMissingTaxonomiesEncodeAsEmptyArrays`：构造无标签文章，
  直接 `json.Marshal(PostsResult)` 断言输出 `"tags":[]` / `"categories":[]`、
  不含 `null`。`writeJSON()`（`internal/httpapi/server.go`）用的也是
  `json.Encoder`，行为一致，因此该用例等价覆盖 handler 输出。
  另加 `TestAggregatesAreEmptyArraysWithoutTaxonomies` 钉住聚合接口。
- 前端抽 `frontend/src/utils/posts.ts`：`RawPost` / `RawPostsResponse`
  如实描述服务端可能返回 null 的形状，`normalizePostsResponse()` 在
  API 边界转成 `Post[]`：响应体本身可为 null / 顶层数组、`posts` 可为非数组，
  数组内 null / 数组 / 非对象元素被跳过，`tags` / `categories` 非数组或
  含非字符串项一并降级；分页字段缺失或非法时从 `posts.length` 推导
  （`total` / `per_page`；**总量非零时** `total_pages = ceil(total / per_page)`，
  总量为 0 时派生 0），`page` 回落到调用方传入的请求页码以免翻页被弹回
  第 1 页。服务端没给合法页码且请求页码越界时，**收敛页码到最后一页**
  而不是放大总页数，避免「第 5 / 5 页」+ 空列表且翻页按钮被锁死。
  `has_next` / `has_prev` 一律按 `page < total_pages` / `page > 1` 推导
  （与 Go 后端 `posts.go` 同构），不被旧后端陈旧的布尔值锁死。
  空结果（`total === 0`）时派生总页数为 0、可能小于页码，但 `Posts.tsx`
  用 `pagination.total > 0` 隐藏分页栏，不会渲染「第 3 / 0 页」。
  `Posts.tsx` 传 `currentPage`、`Dashboard.tsx` 用默认值，两者统一走归一化；
  `Posts.loadFilters` 的标签 / 分类下拉也补了 `|| []`。
- 新增 `posts.test.ts`（17 例）覆盖 null、字段缺失、`posts` 非数组、
  数组内混入 null / 数组 / 非对象、分页字段缺失 / 非数值 / 从 posts 推导、
  多页时 `has_next` 派生、第 2 页页码收敛、越界页码收敛到最后一页、
  服务端显式 `has_*` 矛盾时以派生值为准、`page` 回落请求页码、
  响应体为 null / 顶层数组、数组内非字符串项、正常数据保持、单篇归一化。

## 验证

- `pnpm lint`、`pnpm test`（7 files / 49 tests）、`pnpm build` 全绿。
- `go test ./internal/posts`：新增两例通过，其余既有用例通过，
  唯一失败项 `TestModTimeAndDateFormats` 是 Windows 路径分隔符
  （`post\t.md`），在**未改动的 main 工作树**上同样失败，与本次改动无关。
- 本地 Windows 无法原生编译整个包（`internal/posts/publish.go` 用
  `syscall.Flock`、`internal/hugo/hugo.go` 用 `syscall.Kill`），验证时用
  Go 的 `-overlay` 临时把 `publish.go` / `publish_test.go` 换成空 stub，
  **不改动仓库文件**；CI 在 Linux 上执行 `go test -race ./...`。
- 复现证据：修复前用例输出
  `Posts[0]: PostItem{... Tags:[]string(nil), Categories:[]string(nil)}`。
- 部署实例 `http://192.168.2.14:5050/api/posts` 需登录（探测返回 401），
  未直接取样，改用本地 fixture 复现。

## 边界与部署

- 后端输出由 `null` 变 `[]`，对其它 `/api/posts` 消费方是兼容的
  （`CommandPalette` 只读 path / title，`Sidebar` 只读 total）；
  且与 Python 版本历史行为一致。
- `toStringArray()` 过滤数组里的非字符串项：后端 `strOr()` 早已把标签
  统一转成字符串，不会丢有效数据，只是让 `Post` 的类型承诺成立。
- 前端归一化只是第二道防线；**后端必须重新编译部署**才是根治，
  前端产物也需重新构建到 `admin-ui/`。
