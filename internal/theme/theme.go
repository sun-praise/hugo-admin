// Package theme 对齐 services/theme_service.py：
// 已安装主题发现（themes/ 目录 + submodule 检测）、
// 安装（submodule/clone）、激活（改 Hugo 配置）与默认主题目录。
package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DefaultThemes 对齐 ThemeService.list_default_themes：知名主题白名单，
// 供主题页一键安装（copy 模式克隆到 themes/）后预览/启用。
var DefaultThemes = []map[string]any{
	{
		"name":        "Fried-Rice",
		"repo":        "https://github.com/Svtter/Fried-Rice",
		"description": "svtter 的默认博客主题",
	},
	{
		"name":        "PaperMod",
		"repo":        "https://github.com/adityatelange/hugo-PaperMod",
		"description": "最流行的 Hugo 极简博客主题之一，速度快、开箱即用",
	},
	{
		"name":        "stack",
		"repo":        "https://github.com/CaiJimmy/hugo-theme-stack",
		"description": "卡片式布局，侧栏头像归档，中文社区热门",
	},
	{
		"name":        "LoveIt",
		"repo":        "https://github.com/dillonzq/LoveIt",
		"description": "功能丰富的双栏博客主题，中文文档完善",
	},
	{
		"name":        "DoIt",
		"repo":        "https://github.com/HEIGE-PCloud/DoIt",
		"description": "LoveIt 的活跃分支，持续维护、中文生态好",
	},
	{
		"name":        "ananke",
		"repo":        "https://github.com/theNewDynamic/gohugo-theme-ananke",
		"description": "Hugo 官方入门教程默认主题，稳妥朴素",
	},
	{
		"name":        "congo",
		"repo":        "https://github.com/jpanther/congo",
		"description": "现代化定制主题，深浅色与排版细节出色",
	},
	{
		"name":        "terminal",
		"repo":        "https://github.com/panr/hugo-theme-terminal",
		"description": "终端风格极简主题，程序员气质拉满",
	},
}

type Service struct {
	hugoRoot     string
	themesDir    string
	setActive    func(name string) error
	getActive    func() string
	submoduleSet func() map[string]bool
}

func NewService(hugoRoot string, setActive func(string) error, getActive func() string) *Service {
	return &Service{
		hugoRoot:     hugoRoot,
		themesDir:    filepath.Join(hugoRoot, "themes"),
		setActive:    setActive,
		getActive:    getActive,
		submoduleSet: func() map[string]bool { return DetectSubmodules(hugoRoot) },
	}
}

type ThemeInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	IsSubmodule bool   `json:"is_submodule"`
	IsActive    bool   `json:"is_active"`
}

// ListThemes 对齐 list_themes：themes/ 下的目录（含 theme.toml/hugo.toml 的版本描述）。
func (s *Service) ListThemes() ([]map[string]any, error) {
	entries, err := os.ReadDir(s.themesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, fmt.Errorf("读取主题目录失败: %v", err)
	}
	submodules := s.submoduleSet()
	active := s.ActiveTheme()

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	themes := []map[string]any{}
	for _, name := range names {
		info := map[string]any{
			"name":         name,
			"is_submodule": submodules[name],
			"is_active":    name == active,
		}
		// 版本/描述来自主题的 theme.toml / hugo.toml（宽松解析）
		for _, meta := range []string{"theme.toml", "hugo.toml"} {
			if raw, err := os.ReadFile(filepath.Join(s.themesDir, name, meta)); err == nil {
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil {
					continue // TOML 文件不按 JSON 解析，跳过元数据
				}
				if v, ok := m["version"]; ok {
					info["version"] = fmt.Sprintf("%v", v)
				}
				if v, ok := m["description"]; ok {
					info["description"] = fmt.Sprintf("%v", v)
				}
				break
			}
		}
		themes = append(themes, info)
	}
	return themes, nil
}

// ActiveTheme 对齐 get_active_theme：解析 hugo 配置的 theme 字段。
func (s *Service) ActiveTheme() string {
	if s.getActive != nil {
		return s.getActive()
	}
	for _, name := range []string{"hugo.toml", "hugo.yaml", "config.toml", "config.yaml"} {
		raw, err := os.ReadFile(filepath.Join(s.hugoRoot, name))
		if err != nil {
			continue
		}
		re := regexp.MustCompile(`(?m)^\s*theme\s*=\s*"([^"]+)"`)
		if m := re.FindSubmatch(raw); m != nil {
			return string(m[1])
		}
		reYaml := regexp.MustCompile(`(?m)^\s*theme:\s*(\S+)`)
		if m := reYaml.FindSubmatch(raw); m != nil {
			return strings.Trim(string(m[1]), `"'`)
		}
	}
	return ""
}

// ThemeExists 对齐 theme_exists。
func (s *Service) ThemeExists(name string) bool {
	if !validThemeName(name) {
		return false
	}
	st, err := os.Stat(filepath.Join(s.themesDir, name))
	return err == nil && st.IsDir()
}

var reSafeTheme = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func validThemeName(name string) bool {
	return reSafeTheme.MatchString(name)
}

// InstallTheme 对齐 install_theme：submodule 或 clone 模式。
func (s *Service) InstallTheme(repoURL, name, mode string) (map[string]any, error) {
	if repoURL == "" || name == "" {
		return nil, fmt.Errorf("repo_url 与 name 不能为空")
	}
	if !validThemeName(name) {
		return nil, fmt.Errorf("主题名称不合法: %s", name)
	}
	if s.ThemeExists(name) {
		return nil, fmt.Errorf("主题已存在: %s", name)
	}
	if mode == "" {
		mode = "submodule"
	}
	_ = os.MkdirAll(s.themesDir, 0o755)

	target := filepath.Join(s.themesDir, name)
	if mode == "submodule" {
		cmd := exec.Command("git", "-C", s.hugoRoot, "submodule", "add", "-f", repoURL, target)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("submodule 添加失败: %v\n%s", err, out)
		}
	} else {
		cmd := exec.Command("git", "clone", "--depth", "1", repoURL, target)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("克隆失败: %v\n%s", err, out)
		}
		os.RemoveAll(filepath.Join(target, ".git"))
	}
	return map[string]any{"name": name, "repo_url": repoURL}, nil
}

// ActivateTheme 对齐 activate_theme：写站点配置 + settings 的 theme.name。
// 顺序是先站点配置、后 settings：反过来的话配置写失败时 settings 已指向
// 新主题，而 handleThemeList 以 settings 为准，界面会显示「已启用新主题」
// 而 Hugo 仍渲染旧主题。settings 写失败则用备份把配置还原。
func (s *Service) ActivateTheme(name string) (map[string]any, error) {
	if !s.ThemeExists(name) {
		return nil, fmt.Errorf("主题不存在: %s", name)
	}
	if s.setActive == nil {
		return nil, fmt.Errorf("设置服务不可用")
	}
	// 把主题写进站点配置，让（重启或热加载后的）Hugo 真正渲染它；
	// 找不到配置文件时只保留管理设置（项目未初始化等场景）。
	note, changes, err := s.applyToSite(name)
	if err != nil {
		return nil, fmt.Errorf("写入站点配置失败: %v", err)
	}
	if err := s.setActive(name); err != nil {
		rollbackChanges(changes)
		return nil, fmt.Errorf("保存活跃主题失败: %v", err)
	}
	return map[string]any{"name": name, "config_note": note}, nil
}

// ============ 启用时写站点配置 ============

var (
	// 前两个带 (?m)：除了按行匹配，还要能对整份文件做存在性判断
	//（文件首行可能是注释/其他键，^ 必须能匹配任意行首）
	reImportHeader = regexp.MustCompile(`(?m)^\s*\[\[imports\]\]`)
	reAnyTableHead = regexp.MustCompile(`(?m)^\s*\[`)
	// reImportPath 只按行调用，不需要 (?m)
	reImportPath = regexp.MustCompile(`^\s*path\s*=\s*["'](.+?)["']`)
	// 只匹配顶层 theme 键（行首可缩进但必须独占一个键行）；YAML 顶层键
	// 顶格，故不加 ^\s*
	reTopLevelThemeTOML = regexp.MustCompile(`^\s*theme\s*=`)
	reTopLevelThemeYAML = regexp.MustCompile(`^theme\s*:`)
	reShortcodeUse      = regexp.MustCompile(`\{\{[<%]\s*([A-Za-z0-9_-]+)`)
)

// stubMarker 空桩文件里的标记注释：清理时只删内容与本函数产出一致的文件，
// 避免误删用户手写或改写过的同名 shortcode 覆盖。
const stubMarker = "hugo-admin 主题切换空桩"

// stubContent 空桩布局的唯一内容：Hugo 的注释模板，渲染为空。
func stubContent() string {
	return "{{/* " + stubMarker + "：主题未提供该 shortcode，渲染为空；可删除 */}}"
}

// hugoBuiltinShortcodes Hugo 内置 shortcode：无需主题提供，缺失也不打桩
// （打了反而把内链/媒体渲染清空）。
var hugoBuiltinShortcodes = map[string]bool{
	"ref": true, "relref": true, "figure": true, "highlight": true,
	"youtube": true, "vimeo": true, "twitter": true, "x": true,
	"gist": true, "instagram": true, "comment": true,
}

// ApplyToSite 把主题写入站点配置使其在下次服务器启动（或配置热加载）时
// 真正渲染：
//   - 经典站点（根目录 hugo.toml/config.toml/yaml）：替换或追加 theme 键
//   - 配置目录站点（config/_default/）：module.toml（或 config.toml）里有
//     [[imports]] 时把主题插到 imports 首位（Hugo 组件按首个匹配解析，
//     首位优先级最高）；否则同样写 theme 键
//   - 内容用到而主题未提供的 shortcode 在站点根 layouts/_shortcodes/ 生成
//     空桩，避免切换后构建失败；切换到已提供该 shortcode 的主题时自动清桩
//
// 被修改的文件先写 <file>.hugo-admin.bak 滚动备份。返回变更说明。
func (s *Service) ApplyToSite(name string) (string, error) {
	note, _, err := s.applyToSite(name)
	return note, err
}

// siteChange 是一次可回滚的站点改动（配置文件改写 / 空桩新建 / 空桩删除）。
// rollback 把该项恢复到改动前；多项按逆序回滚。
type siteChange struct {
	desc     string
	rollback func()
}

// configChange 配置文件改写的回滚：用 .hugo-admin.bak 覆盖回去，再删备份。
func configChange(path string) siteChange {
	backup := path + ".hugo-admin.bak"
	return siteChange{
		desc: "config:" + path,
		rollback: func() {
			data, err := os.ReadFile(backup)
			if err != nil {
				return
			}
			if err := os.WriteFile(path, data, 0o644); err == nil {
				_ = os.Remove(backup)
			}
		},
	}
}

// applyToSite 是 ApplyToSite 的本体，额外返回本次改动的回滚清单。
// 中途任一步失败都会先自行回滚已做的改动再返回错误——否则调用方拿到的
// changes 是「半成品」，settings 没变而配置已改，正是要消除的双源分叉。
func (s *Service) applyToSite(name string) (string, []siteChange, error) {
	if !s.ThemeExists(name) {
		return "", nil, fmt.Errorf("主题不存在: %s", name)
	}
	var notes []string
	var changes []siteChange
	abort := func(err error) (string, []siteChange, error) {
		rollbackChanges(changes)
		return "", nil, err
	}

	cfgPath := s.findConfigFile()
	if cfgPath != "" {
		note, change, err := s.applyThemeInConfig(cfgPath, name)
		if err != nil {
			return abort(err)
		}
		if note != "" {
			notes = append(notes, note)
		}
		if change != nil {
			changes = append(changes, *change)
		}
	}

	if err := s.syncShortcodeStubs(name, &changes); err != nil {
		return abort(fmt.Errorf("同步 shortcode 空桩失败: %v", err))
	}

	if len(notes) == 0 {
		return "未找到站点配置文件，仅更新管理设置", changes, nil
	}
	return strings.Join(notes, "；"), changes, nil
}

// rollbackChanges 逆序回滚（后做的先撤），单项失败只跳过不影响其余项。
func rollbackChanges(changes []siteChange) {
	for i := len(changes) - 1; i >= 0; i-- {
		changes[i].rollback()
	}
}

// findConfigFile 按优先级定位站点主配置：配置目录模式下，含 [[imports]]
// 的 module.toml 优先（module 站点靠它定优先级），其次 config/_default/
// 的 config.toml|yaml；经典站点取根目录 hugo.toml 等四个经典名。
// 找不到返回 ""。
func (s *Service) findConfigFile() string {
	hasImports := func(p string) bool {
		data, err := os.ReadFile(p)
		return err == nil && reImportHeader.MatchString(string(data))
	}
	moduleToml := filepath.Join(s.hugoRoot, "config", "_default", "module.toml")
	if hasImports(moduleToml) {
		return moduleToml
	}
	for _, rel := range []string{
		filepath.Join("config", "_default", "config.toml"),
		filepath.Join("config", "_default", "config.yaml"),
		"hugo.toml", "config.toml", "hugo.yaml", "config.yaml",
	} {
		p := filepath.Join(s.hugoRoot, rel)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// applyThemeInConfig 把 name 写进单个配置文件，返回变更说明（无变化返回 ""）
// 与实际写入的文件（未写入时为 nil）。
func (s *Service) applyThemeInConfig(path, name string) (string, *siteChange, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	content := string(raw)

	var updated string
	if strings.HasSuffix(path, ".toml") && reImportHeader.MatchString(content) {
		updated = upsertFirstImport(content, name)
	} else if strings.HasSuffix(path, ".toml") {
		updated = setTOMLThemeKey(content, name)
	} else {
		updated = setYAMLThemeKey(content, name)
	}
	if updated == content {
		return fmt.Sprintf("站点配置已指向主题 %s", name), nil, nil
	}
	if err := backupFile(path); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", nil, err
	}
	rel, _ := filepath.Rel(s.hugoRoot, path)
	change := configChange(path)
	return fmt.Sprintf("已写入 %s（备份 .hugo-admin.bak）", rel), &change, nil
}

// importMarker 标记「本工具插入的 [[imports]] 块」：切换主题时据此只删
// 自己写的块，用户原有的导入一个不动。没有标记的话只能按 path 猜，
// 每切一次就多留一条旧主题的导入，旧主题的组件会从后面继续生效。
const importMarker = "# hugo-admin 主题切换（本条由工具维护，启用别的主题时自动替换）"

// importBlock 是一个 [[imports]] 块的行范围（start 含标记行，end 不含）。
type importBlock struct {
	start, end int
	path       string
	marked     bool
}

// markerAbove 返回第 i 行上方最近**非空**行的标记行号（无标记返回 -1）。
// 放宽到「跳过空行」是有意的：用户手一抖在标记与 [[imports]] 之间敲了个
// 空行，块不该因此退化成「用户的块」而永久残留。
// 必须返回行号而不是布尔：块的范围要从标记行起算，否则清理时只从空行开始
// 删，标记会残留下来，下一轮正好贴到用户自己的块上方，把用户块当工具块删掉。
func markerAbove(lines []string, i int) int {
	for j := i - 1; j >= 0; j-- {
		if strings.TrimSpace(lines[j]) == "" {
			continue
		}
		if strings.TrimSpace(lines[j]) == importMarker {
			return j
		}
		return -1
	}
	return -1
}

// parseImportBlocks 切出文件里所有 [[imports]] 块及其 path。
func parseImportBlocks(lines []string) []importBlock {
	var blocks []importBlock
	for i := 0; i < len(lines); i++ {
		if !reImportHeader.MatchString(lines[i]) {
			continue
		}
		b := importBlock{start: i, end: len(lines)}
		if m := markerAbove(lines, i); m >= 0 {
			b.start = m // 从标记行起算，中间的空白行也归本块
			b.marked = true
		}
		for j := i + 1; j < len(lines); j++ {
			if reAnyTableHead.MatchString(lines[j]) {
				b.end = j
				break
			}
			if m := reImportPath.FindStringSubmatch(lines[j]); m != nil && b.path == "" {
				b.path = m[1]
			}
		}
		blocks = append(blocks, b)
		i = b.end - 1
	}
	return blocks
}

// upsertFirstImport 把主题的 [[imports]] 块放到 imports 首位，并清掉上次
// 插入的那条（带 importMarker 标记）。Hugo 组件按导入顺序首个匹配生效，
// 首位即最高优先级。
//
// 只维护自己插入的块是关键：按 path 去重会让用户原有的主题导入随切换次数
// 不断堆积，旧主题的组件留在 imports 里继续生效，切换结果被污染。
// 反过来，若用户自己就有同 path 的导入，则把那条整体移到首位，不再插一条
// 重复的 module import。
func upsertFirstImport(content, name string) string {
	lines := strings.Split(content, "\n")
	blocks := parseImportBlocks(lines)

	skip := map[int]bool{}
	for _, b := range blocks {
		if !b.marked {
			continue
		}
		end := b.end
		for end < len(lines) && strings.TrimSpace(lines[end]) == "" {
			end++ // 连同块后的空行一起清掉，避免反复切换留下空行
		}
		for i := b.start; i < end; i++ {
			skip[i] = true
		}
	}
	rest := make([]string, 0, len(lines))
	for i, ln := range lines {
		if !skip[i] {
			rest = append(rest, ln)
		}
	}

	restBlocks := parseImportBlocks(rest)
	if len(restBlocks) > 0 && restBlocks[0].path == name {
		return strings.Join(rest, "\n") // 已是生效主题，无需改动
	}
	// 用户自己的同 path 导入：移到首位即可，重复插一条会变成两条相同 import
	for _, b := range restBlocks {
		if b.path == name {
			return moveBlockToFront(rest, b)
		}
	}

	newBlock := []string{importMarker, "[[imports]]", fmt.Sprintf("path = %q", name), ""}
	out := make([]string, 0, len(rest)+len(newBlock))
	// 剩余内容里没有 imports 块时，插到首个表头之前而不是第 0 行：
	// TOML 里表头之后的行归属该表，把块顶到最前面会把顶层键（noVendor 等）
	// 吞进 imports 元素里
	insertAt := firstTableIndex(rest)
	if len(restBlocks) > 0 {
		insertAt = restBlocks[0].start
	}
	out = append(out, rest[:insertAt]...)
	out = append(out, newBlock...)
	out = append(out, rest[insertAt:]...)
	return strings.Join(out, "\n")
}

// firstTableIndex 返回首个表头所在行号；无表头返回 len(lines)。
func firstTableIndex(lines []string) int {
	for i, ln := range lines {
		if reAnyTableHead.MatchString(ln) {
			return i
		}
	}
	return len(lines)
}

// moveBlockToFront 把用户已有的 imports 块搬到「首个表头之前」——即让它成为
// 第一个表，同时把顶层键（noVendor/proxy 等）留在它上方。搬到文件第 0 行是
// 错的：TOML 里表头之后的行归属该表，顶层键会变成 imports 元素的字段。
// 其余行原样保留，只合并多余空行。
func moveBlockToFront(lines []string, b importBlock) string {
	moved := append([]string{}, lines[b.start:b.end]...)
	for len(moved) > 0 && strings.TrimSpace(moved[len(moved)-1]) == "" {
		moved = moved[:len(moved)-1]
	}
	others := append(append([]string{}, lines[:b.start]...), lines[b.end:]...)
	at := firstTableIndex(others)
	head, tail := others[:at], others[at:]
	for len(head) > 0 && strings.TrimSpace(head[len(head)-1]) == "" {
		head = head[:len(head)-1]
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, head...)
	if len(out) > 0 {
		out = append(out, "")
	}
	out = append(out, moved...)
	if len(tail) > 0 {
		out = append(out, "")
		out = append(out, tail...)
	}
	return strings.Join(out, "\n")
}

// setTOMLThemeKey 把 theme 键写在**顶层**：已有顶层 theme 行则替换，否则插到
// 首个表头之前。直接追加到文件末尾是错的——真实 hugo.toml 几乎都以表结尾
// （`[params]` 等），追加行会落进最后一张表变成 `<表名>.theme`，Hugo 静默
// 忽略而界面显示「已启用」，站点纹丝不动。
func setTOMLThemeKey(content, name string) string {
	lines := strings.Split(content, "\n")
	value := `theme = "` + name + `"`
	// 顶层区域 = 首个表头之前的所有行；之后的 theme = 属于表内键，不能碰
	head := len(lines)
	for i, ln := range lines {
		if reAnyTableHead.MatchString(ln) {
			head = i
			break
		}
	}
	for i := 0; i < head; i++ {
		if reTopLevelThemeTOML.MatchString(lines[i]) {
			lines[i] = value
			return strings.Join(lines, "\n")
		}
	}
	return insertTopLevel(lines, head, value)
}

// setYAMLThemeKey 同 setTOMLThemeKey 的 YAML 版：顶层键行首无缩进，
// 表内键必然缩进，因此只认行首顶格的 theme:。
func setYAMLThemeKey(content, name string) string {
	lines := strings.Split(content, "\n")
	value := `theme: "` + name + `"`
	for i, ln := range lines {
		if reTopLevelThemeYAML.MatchString(ln) {
			lines[i] = value
			return strings.Join(lines, "\n")
		}
	}
	// 没有 theme 键：插到第一个顶层键之前（跳过注释/空行/文档分隔符）
	insertAt := len(lines)
	for i, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---" || trimmed == "..." {
			continue
		}
		if ln == trimmed { // 无缩进即顶层键
			insertAt = i
			break
		}
	}
	return insertTopLevel(lines, insertAt, value)
}

// insertTopLevel 在 lines[idx] 之前插入一行顶层键，并补齐前后空行。
func insertTopLevel(lines []string, idx int, value string) string {
	out := make([]string, 0, len(lines)+2)
	out = append(out, lines[:idx]...)
	if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, value)
	if idx < len(lines) {
		out = append(out, "")
	}
	out = append(out, lines[idx:]...)
	return strings.Join(out, "\n")
}

// backupFile 写 <path>.hugo-admin.bak 滚动备份（每次覆盖）。
func backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".hugo-admin.bak", data, 0o644)
}

// syncShortcodeStubs 让站点根 layouts/_shortcodes/ 恰好补齐「内容用到而
// 主题未提供」的 shortcode：缺失的生成空桩；根桩中主题已提供的删除
// （项目布局优先级高于主题，残留空桩会覆盖新主题的真实实现）。
// 每项改动都记进 changes，供调用方在中途失败时回滚。
func (s *Service) syncShortcodeStubs(themeName string, changes *[]siteChange) error {
	used, err := s.scanContentShortcodes()
	if err != nil {
		return err
	}
	provided, err := s.themeShortcodes(themeName)
	if err != nil {
		return err
	}
	stubDir := filepath.Join(s.hugoRoot, "layouts", "_shortcodes")
	entries, _ := os.ReadDir(stubDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		sc := strings.TrimSuffix(e.Name(), ".html")
		if !provided[sc] {
			continue
		}
		// 只删「原样未动」的空桩：内容与被删前逐字相同才动手。用户若把
		// 空桩改成自己的实现（哪怕只留了那行标记），文件已归用户，不动。
		p := filepath.Join(stubDir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil || strings.TrimSpace(string(data)) != stubContent() {
			continue
		}
		if err := os.Remove(p); err != nil {
			continue
		}
		*changes = append(*changes, removedStubChange(p, data))
	}
	missing := map[string]bool{}
	for sc := range used {
		if !provided[sc] && !hugoBuiltinShortcodes[sc] {
			missing[sc] = true
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		return err
	}
	for sc := range missing {
		p := filepath.Join(stubDir, sc+".html")
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.WriteFile(p, []byte(stubContent()), 0o644); err != nil {
			return err
		}
		*changes = append(*changes, createdStubChange(p))
	}
	return nil
}

// createdStubChange 新建空桩的回滚：删掉这个文件。
func createdStubChange(path string) siteChange {
	return siteChange{
		desc:     "stub+:" + path,
		rollback: func() { _ = os.Remove(path) },
	}
}

// removedStubChange 删除空桩的回滚：把删除前的原内容写回。
func removedStubChange(path string, content []byte) siteChange {
	return siteChange{
		desc:     "stub-:" + path,
		rollback: func() { _ = os.WriteFile(path, content, 0o644) },
	}
}

// scanContentShortcodes 扫描 content/ 下 .md 里用到的 shortcode 名
// （无 content 目录返回空集）。
func (s *Service) scanContentShortcodes() (map[string]bool, error) {
	used := map[string]bool{}
	root := filepath.Join(s.hugoRoot, "content")
	if _, err := os.Stat(root); err != nil {
		return used, nil
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range reShortcodeUse.FindAllStringSubmatch(string(raw), -1) {
			used[m[1]] = true
		}
		return nil
	})
	return used, err
}

// themeShortcodes 收集主题提供的 shortcode 名（layouts 下 shortcodes /
// _shortcodes 目录里的 *.html）。
func (s *Service) themeShortcodes(themeName string) (map[string]bool, error) {
	provided := map[string]bool{}
	base := filepath.Join(s.themesDir, themeName, "layouts")
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		dir := filepath.Base(filepath.Dir(path))
		if dir == "shortcodes" || dir == "_shortcodes" {
			provided[strings.TrimSuffix(filepath.Base(path), ".html")] = true
		}
		return nil
	})
	if os.IsNotExist(err) {
		return provided, nil
	}
	return provided, err
}

// DetectSubmodules 是可用的 submodule 检测（按 hugoRoot）。
func DetectSubmodules(hugoRoot string) map[string]bool {
	result := map[string]bool{}
	out, err := exec.Command("git", "-C", hugoRoot, "submodule", "status", "--recursive").Output()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		path := fields[1]
		if strings.HasPrefix(path, "themes/") {
			result[strings.TrimPrefix(path, "themes/")] = true
		}
	}
	return result
}
