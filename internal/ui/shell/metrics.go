package shell

import (
	"fmt"
	"log"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// Metrics are the spike's measurements, shown on the settings page and
// logged with a "[metric]" prefix for the test checklist.
type Metrics struct {
	mu        sync.Mutex
	start     time.Time
	first     time.Duration // process start (main) to the first painted frame
	firstDone bool
	tray      []time.Duration // tray click to the panel's first paint
	trayT0    time.Time
	keys      []time.Duration // palette: the build that saw a new query to its paint
	keyT0     time.Time
	search    []time.Duration // registry.Search alone
}

func newMetrics(start time.Time) *Metrics { return &Metrics{start: start} }

func (m *Metrics) firstFrame(done func(time.Duration)) {
	m.mu.Lock()
	if m.firstDone {
		m.mu.Unlock()
		return
	}
	m.firstDone = true
	m.first = time.Since(m.start)
	m.mu.Unlock()
	log.Printf("[metric] first_frame_ms=%.1f", ms(m.first))
	if done != nil {
		done(m.first)
	}
}

func (m *Metrics) trayClicked() { m.mu.Lock(); m.trayT0 = time.Now(); m.mu.Unlock() }

func (m *Metrics) trayPainted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.trayT0.IsZero() {
		return
	}
	d := time.Since(m.trayT0)
	m.trayT0 = time.Time{}
	m.tray = append(m.tray, d)
	log.Printf("[metric] tray_open_ms=%.1f", ms(d))
}

func (m *Metrics) keyChanged(search time.Duration) {
	m.mu.Lock()
	m.keyT0 = time.Now().Add(-search)
	m.search = append(m.search, search)
	m.mu.Unlock()
}

func (m *Metrics) keyPainted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keyT0.IsZero() {
		return
	}
	d := time.Since(m.keyT0)
	m.keyT0 = time.Time{}
	m.keys = append(m.keys, d)
	log.Printf("[metric] palette_key_ms=%.2f search_us=%.0f", ms(d), float64(m.search[len(m.search)-1].Microseconds()))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// bytesPerMB converts bytes to megabytes.
const bytesPerMB = 1e6

// summary is "n × · median x ms · max y ms". The measurements are for
// developers and stay in English, as logs do.
func summary(ds []time.Duration) string {
	if len(ds) == 0 {
		return "no data yet"
	}
	s := slices.Clone(ds)
	slices.Sort(s)
	return fmt.Sprintf("%d × · median %.1f ms · max %.1f ms", len(s), ms(s[len(s)/2]), ms(s[len(s)-1]))
}

// measurements is the settings page's section of measurements, shown with
// -dev only.
func (a *App) measurements(c *ui.Context) {
	m := a.metrics
	m.mu.Lock()
	first, tray, keys, search := m.first, slices.Clone(m.tray), slices.Clone(m.keys), slices.Clone(m.search)
	m.mu.Unlock()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	a.valueRow(c, "first frame", fmt.Sprintf("%.0f ms", ms(first)))
	a.valueRow(c, "tray panel (click to paint)", summary(tray))
	a.valueRow(c, "palette keystroke (to paint)", summary(keys))
	a.valueRow(c, "palette search", summary(search))
	a.valueRow(c, "Go heap / from the system", fmt.Sprintf("%.1f MB / %.1f MB", float64(mem.HeapAlloc)/bytesPerMB, float64(mem.Sys)/bytesPerMB))
	a.valueRow(c, "Go / platform", fmt.Sprintf("%s / %s-%s", runtime.Version(), runtime.GOOS, runtime.GOARCH))
}
