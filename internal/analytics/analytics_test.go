package analytics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFrontendTrackerContract 钉住管理端 SPA 携带的 Umami 追踪器契约
// （frontend/src/analytics.ts）：只有公网 demo 域名才注入、必须带
// data-website-id / data-domains / referrerpolicy，且指向自托管 Umami。
// 跨语言字面量钉法与 internal/importsvc 的事件名契约测试同式。
func TestFrontendTrackerContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "analytics.ts"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	for _, want := range []string{
		"analysis.sun-praise.com/script.js",
		`'data-website-id', '1922bd9a-6d58-4801-9bb1-f9cfe289139f'`,
		`'data-domains', PUBLIC_HOST`,
		`'referrerpolicy', 'no-referrer'`,
		"PUBLIC_HOST = 'hugo-admin.sun-praise.com'",
		"location.hostname !== PUBLIC_HOST",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("frontend/src/analytics.ts 缺少契约字面量 %s", want)
		}
	}
}

// TestMainCallsAnalytics pins the call site: analytics.ts 的 initAnalytics
// 必须在 main.tsx 里被实际调用——删掉调用点会让整个 analytics.ts 静默失效
// （契约测试全绿但统计消失）。
func TestMainCallsAnalytics(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "main.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "initAnalytics()") {
		t.Fatal("frontend/src/main.tsx 未调用 initAnalytics()——tracker 不会被注入")
	}
}

// TestIndexHTMLHasNoStaticTracker 保证 index.html 不静态携带 tracker：
// 注入走应用挂载后的运行时门控，静态标签会让私有部署/本地 dev 也向
// 分析域发起请求（data-domains 只挡上报、不挡加载）。
func TestIndexHTMLHasNoStaticTracker(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "frontend", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "analysis.sun-praise.com") {
		t.Fatalf("frontend/index.html 不得静态携带 tracker（应由 analytics.ts 运行时注入）:\n%s", string(raw))
	}
}
