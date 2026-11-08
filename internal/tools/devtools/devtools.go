// Package devtools holds tools for developing the app, which only the
// -dev flag registers: made-up tools, to see the UI with hundreds of them,
// a made-up traffic capture, to see the inspector page before the real
// tool exists, and a tool that panics on purpose, to check that a failure
// stays inside its tool.
package devtools

import (
	"context"
	"fmt"
	"sync"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

type word struct{ en, zh string }

func verbs() []word {
	return []word{
		{"Convert", "转换"},
		{"Compress", "压缩"},
		{"Inspect", "查看"},
		{"Organize", "整理"},
		{"Clean", "清理"},
		{"Generate", "生成"},
		{"Verify", "校验"},
		{"Compare", "比较"},
		{"Extract", "提取"},
		{"Monitor", "监控"},
		{"Sync", "同步"},
		{"Preview", "预览"},
		{"Encode", "编码"},
		{"Count", "统计"},
		{"Export", "导出"},
	}
}

func nouns() map[tool.Category][]word {
	return map[tool.Category][]word{
		tool.CategorySystem:       {{"Memory", "内存"}, {"Processes", "进程"}, {"Battery", "电池"}, {"Disks", "磁盘"}, {"Fonts", "字体"}},
		tool.CategoryFile:         {{"Images", "图片"}, {"Archives", "压缩包"}, {"Folders", "文件夹"}, {"Downloads", "下载"}, {"Metadata", "元数据"}},
		tool.CategoryDev:          {{"Regex", "正则"}, {"Timestamps", "时间戳"}, {"Hashes", "哈希"}, {"JWT", "令牌"}, {"Diffs", "差异"}},
		tool.CategoryNetwork:      {{"Speed", "网速"}, {"DNS", "域名"}, {"Proxy", "代理"}, {"Wi-Fi", "无线网"}, {"Routes", "路由"}},
		tool.CategoryText:         {{"Case", "大小写"}, {"Word Count", "字数"}, {"Spelling", "拼写"}, {"Lines", "排序"}, {"Tables", "表格"}},
		tool.CategoryMedia:        {{"Video", "视频"}, {"Audio", "音频"}, {"Screenshots", "截图"}, {"Subtitles", "字幕"}, {"QR Codes", "二维码"}},
		tool.CategoryProductivity: {{"Clipboard", "剪贴板"}, {"Calendar", "日程"}, {"Timers", "计时"}, {"Notes", "便签"}, {"To-dos", "待办"}},
	}
}

func icons() map[tool.Category][]string {
	return map[tool.Category][]string{
		tool.CategorySystem:       {"memory", "power_settings_new", "settings", "tune", "dark_mode"},
		tool.CategoryFile:         {"drive_file_rename", "content_copy", "upload", "preview", "lock"},
		tool.CategoryDev:          {"terminal", "colorize", "keyboard", "smart_toy", "tune"},
		tool.CategoryNetwork:      {"lan", "open_in_new", "upload", "lock", "terminal"},
		tool.CategoryText:         {"language", "drive_file_rename", "search", "keyboard", "apps"},
		tool.CategoryMedia:        {"palette", "contrast", "colorize", "preview", "open_in_new"},
		tool.CategoryProductivity: {"keep", "bedtime_off", "smart_toy", "language", "palette"},
	}
}

// named returns a message that says en in English and zh in Chinese; the
// made-up names are not in the catalogs.
func named(en, zh string) tool.Message {
	return func(m i18n.Messages) string {
		if m == i18n.In("zh-Hans") {
			return zh
		}
		return en
	}
}

// Fake is a made-up tool: an action tool whose page says it is fake, or,
// one in ten, a switch tool that only remembers whether it is on.
type Fake struct {
	manifest tool.Manifest
}

// Manifest describes the tool.
func (f *Fake) Manifest() tool.Manifest { return f.manifest }

// Specs declares a plan that changes nothing.
func (f *Fake) Specs() []tool.Spec {
	return []tool.Spec{tool.PlanSpec(f.manifest.Summary, tool.NoInput())}
}

// Specs declares start, stop and status.
func (f *FakeSwitch) Specs() []tool.Spec {
	return tool.SwitchSpecs(f.manifest.Summary, f.manifest.Summary, f.manifest.Summary)
}

// FakeSwitch is a made-up switch tool.
type FakeSwitch struct {
	Fake
	mu      sync.Mutex
	on      bool
	changed func()
}

// Start turns it on.
func (f *FakeSwitch) Start(context.Context) error { return f.set(true) }

// Stop turns it off.
func (f *FakeSwitch) Stop() error { return f.set(false) }

func (f *FakeSwitch) set(on bool) error {
	f.mu.Lock()
	f.on = on
	f.mu.Unlock()
	f.changed()
	return nil
}

// Status says whether it is on.
func (f *FakeSwitch) Status() tool.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.on {
		return tool.Status{On: true, Text: i18n.StatusOn()}
	}
	return tool.Status{Text: i18n.StatusOff()}
}

// switchEvery makes one made-up tool in this many a switch tool.
const switchEvery = 10

// Fakes makes n made-up tools spread over the categories, and the made-up
// traffic tool, whose page is the inspector.
func Fakes(n int, changed func()) []tool.Tool {
	vs, ns, is := verbs(), nouns(), icons()
	out := make([]tool.Tool, 0, n)
	for i := 0; len(out) < n; i++ {
		cat := tool.Categories()[i%len(tool.Categories())]
		round := i / len(tool.Categories())
		v, w := vs[round%len(vs)], ns[cat][(round/len(vs))%len(ns[cat])]
		m := tool.Manifest{
			ID:       fmt.Sprintf("fake-%s-%d", cat, round),
			Name:     named(v.en+" "+w.en+suffix(round, len(vs)*len(ns[cat])), v.zh+w.zh+suffix(round, len(vs)*len(ns[cat]))),
			Summary:  named("A made-up tool, for development", "开发用的假工具"),
			Category: cat,
			Kind:     tool.KindAction,
			Icon:     is[cat][round%len(is[cat])],
			Keywords: []string{v.en + " " + w.en},
		}
		if i%switchEvery == 3 {
			m.Kind = tool.KindSwitch
			out = append(out, &FakeSwitch{Fake: Fake{m}, changed: changed})
			continue
		}
		m.Soon = true
		out = append(out, &Fake{m})
	}
	return append(out, NewTraffic())
}

func suffix(round, distinct int) string {
	if round < distinct {
		return ""
	}
	return fmt.Sprintf(" %d", round/distinct+1)
}
