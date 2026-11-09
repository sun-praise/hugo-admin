# 主题切换内容兼容性指南

## 背景

「启用」主题会把主题写入站点配置（经典站点写 `theme` 键；配置目录站点把主题
`[[imports]]` 块插到 `config/_default/module.toml` 首位——Hugo 组件按导入顺序
首个匹配生效，首位优先级最高），并同步 shortcode 空桩。Hugo dev server 热加载
配置，切换通常立即生效，无需重启。

但主题能否正常渲染，还取决于内容与主题的兼容性。本文记录已知的兼容性问题
类别、识别方法与处理方式。

## 已知不兼容类别

### 1. frontmatter 字段结构冲突

不同主题对同一字段的类型约定可能不同。典型案例：

**PaperMod 要求 `cover` 是对象，部分博客习惯写字符串：**

```toml
# 博客现有写法（inkline 约定）
cover = "https://example.com/cover.jpg"

# PaperMod 期望
[cover]
  image = "https://example.com/cover.jpg"
```

**现象**：切换后站点显示 Hugo 错误页，服务器日志报模板执行错误：

```
execute of template failed: template: _partials/cover.html:2:19
at <.Params.cover.image>: can't evaluate field image in type string
```

**处理**：批量调整 frontmatter 字段结构，或换用对字段类型宽容的主题
（LoveIt / DoIt / Stack 等中文社区主题普遍接受字符串 cover）。
这是内容层面的迁移工作，hugo-admin 不自动改写文章 frontmatter。

### 2. 主题专属 shortcode

内容使用了 A 主题提供的 shortcode（如 `{{< social-card >}}`），切到没有该
shortcode 的 B 主题时构建直接失败。

**hugo-admin 的自动处理**：启用主题时扫描 `content/` 用到的 shortcode，
对主题未提供的在站点根 `layouts/_shortcodes/` 生成带标记注释的空桩：

```html
{{/* hugo-admin 主题切换空桩：主题未提供该 shortcode，渲染为空；可删除 */}}
```

- 缺失的 shortcode 渲染为空，不再阻断构建
- 切换到提供该 shortcode 的主题时，空桩自动清理
- 只清理带标记注释的文件，用户手写的同名覆盖不受影响
- Hugo 内置 shortcode（`ref`/`relref`/`figure` 等）不打桩

### 3. Dart Sass 依赖

Hugo 0.126+ 移除了内置 libsass，需要 SCSS 编译的主题（LoveIt、DoIt、Stack
等）会报：

```
TOCSS-DART: failed to transform "/css/style.scss" ...
You need to install Dart Sass
```

**处理**：安装官方 dart-sass 二进制（npm 的 JS 版 `sass` 与 Hugo 子进程
协议不兼容，会报 unexpected EOF）：

```bash
# 从 https://github.com/sass/dart-sass/releases 下载 linux-x64 包
tar -xzf dart-sass-*-linux-x64.tar.gz
cp -r dart-sass ~/.local/
ln -sf ~/.local/dart-sass/sass ~/.local/bin/sass
# 确保 ~/.local/bin 在 PATH 后重启 Hugo 服务器
```

### 4. 主题参数与主页模式

切换后菜单、文章列表正常，但首页空白或显示主题默认文案：主题的主页模式
（如 LoveIt 的 `[params.home]`）、侧栏、配色等需要按新主题的 params 约定
调整 `params.toml`。这属于正常的主题迁移配置工作。

注意一个隐蔽机制：主题目录可自带 `hugo.toml` 默认配置，会**填充站点未定义
的键**。案例：LoveIt 的 `themes/LoveIt/hugo.toml` 默认
`params.header.title.name = "My cool site"`，站点 `params.toml` 未配置该键
（旧主题不使用它）时，头部导航站点名就会显示这个占位值——而浏览器标签页
`<title>` 读 `Site.Title`（站点配置）仍然正确，两处不一致即是此因。处理：
按新主题文档在 `params.toml` 显式配置同名键。

### 5. HTML 内容文件的安全策略

内容目录含 `.html` 页面时，Hugo 0.163+ 默认拒绝 `text/html` 内容文件。
需在站点配置中放行（预览功能已在覆盖站点中内置，真实站点需自行配置）：

```toml
[security]
  allowContent = ['.*']
```

### 6. 预览实例（覆盖站点）

「预览」不会动真实站点：它在 `<站点根>/.admin/preview/site/` 生成一个临时
Hugo 站点，用随机空闲端口独立启动，与主服务器互不影响。覆盖站点只做三件事：

- `hugo.toml` 仅声明预览主题、`defaultContentLanguage = 'zh-cn'`、
  `[security] allowContent = ['.*']`（见类别 5）、`unsafe = true`
- `themes` 软链接指向站点的 `themes/`（复用已安装主题）
- `content/post/` 放**最新 8 篇**的采样副本：剥掉原有 frontmatter 只留
  标题/日期，正文用到的 shortcode 生成空桩

因此预览**不代表真实内容的渲染结果**——它验证的是主题本身的版式、配色与
列表页效果。要做到「真实内容 + 新主题」的端到端确认，仍需启用主题后看真实
站点（这也正是类别 1 那类 frontmatter 冲突只在启用后才暴露的原因）。

该目录可随时删除，下次预览重新生成；建议在博客仓库 `.gitignore` 里加
`.admin/`。

## 回滚

启用主题会修改站点配置并写滚动备份，且：

- 配置文件改动会出现在 `git status`（推荐博客仓库纳入 git 管理）
- 备份文件：`<配置文件>.hugo-admin.bak`
- 空桩：`layouts/_shortcodes/` 下带标记注释的文件，可整目录删除

手动回滚：

```bash
git restore config/_default/module.toml   # 或对应配置文件
rm -rf layouts/_shortcodes                # 只含空桩时
# 重启 Hugo 服务器
```

## 案例：PaperMod × 模块化博客（2026-10-26）

模块化站点（`go.mod` + `_vendor/`）实测结论：

- `hugo server --theme X` 与 `--config` 覆盖均无法压过 vendored module
  的组件优先级；把主题 `[[imports]]` 插到 module.toml **首位**是唯一
  可靠的切换方式
- PaperMod 因 cover 字段结构冲突无法渲染（见类别 1）；LoveIt 安装
  dart-sass 后 654 篇文章、33 页分页列表完整渲染，菜单等配置目录
  内容全部继承
