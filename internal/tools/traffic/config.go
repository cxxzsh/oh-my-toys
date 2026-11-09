package traffic

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"slices"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// The ports a capture proxy may listen on: below 1024 the system asks for
// an administrator's password.
const (
	portMin = 1024
	portMax = 65535
)

// defaultProxyPort is the port the proxy listens on until a person picks
// another one.
const defaultProxyPort = 9099

// defaultConfig is what a fresh installation captures with: a proxy on
// loopback, set as the system proxy, opening HTTPS.
func defaultConfig() tool.CaptureConfig {
	return tool.CaptureConfig{
		Mode:        tool.ModeProxy,
		ProxyPort:   defaultProxyPort,
		SystemProxy: true,
		Decrypt:     true,
	}
}

// loadConfig reads the saved settings. A missing, unreadable or unusable
// file is reported in the log and replaced by the defaults, because a
// capture that cannot start is worse than one with default settings.
func (t *Tool) loadConfig() tool.CaptureConfig {
	var c tool.CaptureConfig
	err := readJSON(t.path(configFile), &c)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return defaultConfig()
	case err != nil:
		log.Printf("traffic: %v; capturing with the defaults", err)
		return defaultConfig()
	}
	c.Bypass = nonEmpty(c.Bypass)
	c.Scope = nonEmpty(c.Scope)
	if err := validateConfig(c); err != nil {
		log.Printf("traffic: %v; capturing with the defaults", err)
		return defaultConfig()
	}
	return c
}

// SetConfig keeps the settings, and restarts a capture in progress so that
// it uses them. It is the choice of the next capture too.
func (t *Tool) SetConfig(c tool.CaptureConfig) error {
	if err := validateConfig(c); err != nil {
		return err
	}
	c.Bypass = slices.Clone(c.Bypass)
	c.Scope = slices.Clone(c.Scope)
	t.opMu.Lock()
	defer t.opMu.Unlock()
	old := t.Config()
	if err := writeJSON(t.path(configFile), c); err != nil {
		return err
	}
	t.mu.Lock()
	t.cfg = c
	t.mu.Unlock()
	if !t.running() || sameConfig(old, c) {
		return nil
	}
	if err := t.stopCapture(); err != nil {
		return err
	}
	return t.startCapture(context.Background(), c, rootMake)
}

// validateConfig says whether the settings can work, and what is wrong
// with the first one that cannot.
func validateConfig(c tool.CaptureConfig) error {
	switch c.Mode {
	case tool.ModeProxy, tool.ModePackets:
	default:
		return fmt.Errorf("%w: unknown capture mode %d", ErrBadConfig, int(c.Mode))
	}
	if c.ProxyPort < portMin || c.ProxyPort > portMax {
		return fmt.Errorf("%w: proxy port %d is outside %d to %d", ErrBadConfig, c.ProxyPort, portMin, portMax)
	}
	if c.Upstream != "" {
		u, err := parseUpstream(c.Upstream)
		if err != nil {
			return err
		}
		if isOurAddress(u.Host, c.ProxyPort) {
			return fmt.Errorf("%w: upstream %q is this capture's own proxy", ErrBadConfig, u.Redacted())
		}
	}
	if i := slices.IndexFunc(c.Bypass, blank); i >= 0 {
		return fmt.Errorf("%w: the bypass entry %d is empty", ErrBadConfig, i+1)
	}
	if i := slices.IndexFunc(c.Scope, blank); i >= 0 {
		return fmt.Errorf("%w: the scope entry %d is empty", ErrBadConfig, i+1)
	}
	return nil
}

// sameConfig reports whether two settings would start the same capture.
// The order of the bypass patterns counts, as it does for the proxy.
func sameConfig(a, b tool.CaptureConfig) bool {
	return a.Mode == b.Mode &&
		a.ProxyPort == b.ProxyPort &&
		a.SystemProxy == b.SystemProxy &&
		a.Decrypt == b.Decrypt &&
		a.Interface == b.Interface &&
		a.Upstream == b.Upstream &&
		slices.Equal(a.Bypass, b.Bypass) &&
		slices.Equal(a.Scope, b.Scope)
}

// blank reports whether a pattern is empty or only spaces.
func blank(s string) bool { return strings.TrimSpace(s) == "" }

// nonEmpty drops the patterns a file holds that are empty or only spaces.
func nonEmpty(patterns []string) []string {
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if !blank(pattern) {
			out = append(out, pattern)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
