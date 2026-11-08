package shell

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/comp"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// certFingerprintChars is how much of the SHA-256 the chip shows: enough
// to tell two certificates apart in a glance.
const certFingerprintChars = 8

// trafficCertRow is the app's root certificate: its state, and the ways to
// trust it, stop trusting it and hand it to another program.
func (a *App) trafficCertRow(c *ui.Context, t tool.Traffic, st *trafficState) {
	a.trafficCertRefresh(c, t, st)
	p := ds.P(c)
	ui.Row(c).FillWidth().Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
		a.kit.Glyph(c, "lock", ds.GlyphSm, p.TextSecondary)
		comp.Text(c, i18n.TrafficCert(), ds.Label, p.TextPrimary)
		if st.certKnown {
			a.trafficCertChip(c, st)
		}
		ui.Spacer(c)
		if a.trafficBusy(st) {
			comp.Text(c, i18n.Working(), ds.Caption, p.TextInactive)
		}
		a.trafficCertButtons(c, t, st)
	})
}

// trafficCertChip is the certificate's state and short fingerprint, in the
// colour of what it means.
func (a *App) trafficCertChip(c *ui.Context, st *trafficState) {
	state, tone := i18n.TrafficCertMissing(), comp.ToneNeutral
	switch {
	case st.cert.Trusted:
		state, tone = i18n.TrafficCertTrusted(), comp.ToneSuccess
	case st.cert.Exists:
		state, tone = i18n.TrafficCertUntrusted(), comp.ToneWarning
	}
	text := state
	if fp := shortFingerprint(st.cert.Fingerprint); fp != "" {
		text = i18n.TrafficCertChip(state, fp)
	}
	comp.Badge(c, text, tone)
}

// shortFingerprint writes the head of a hex fingerprint as one uppercase
// word, or nothing when there is none.
func shortFingerprint(fp string) string {
	clean := strings.ToUpper(strings.NewReplacer(":", "", " ", "").Replace(fp))
	if clean == "" {
		return ""
	}
	return clean[:min(len(clean), certFingerprintChars)]
}

// trafficCertButtons are Install, Remove and the way to another program.
func (a *App) trafficCertButtons(c *ui.Context, t tool.Traffic, st *trafficState) {
	install := comp.ButtonOpts{Kind: comp.Secondary, Small: true, Disabled: a.trafficBusy(st)}
	if st.cert.Trusted {
		install.Kind, install.Disabled = comp.Secondary, true
	}
	if a.kit.Button(c.Key("traffic-cert-install"), i18n.TrafficCertInstall(), install).Clicked() {
		a.trafficCertAction(t, st, false)
	}
	remove := comp.ButtonOpts{Kind: comp.Danger, Small: true, Disabled: a.trafficBusy(st) || !st.cert.Exists}
	if a.kit.Button(c.Key("traffic-cert-remove"), i18n.TrafficCertRemove(), remove).Clicked() {
		a.trafficRemoveCert(t, st)
	}
	copyPath := comp.ButtonOpts{Kind: comp.Ghost, Small: true, Disabled: st.cert.Path == ""}
	if a.kit.Button(c.Key("traffic-cert-copy"), i18n.TrafficCertCopyPath(), copyPath).Clicked() {
		mygo.Clipboard.WriteText(st.cert.Path)
		c.Toast(i18n.TrafficCertCopied())
	}
}

// trafficRemoveCert asks first: the key is gone for good, and HTTPS shows
// as tunnels until the certificate is installed again.
func (a *App) trafficRemoveCert(t tool.Traffic, st *trafficState) {
	a.askDialog(comp.DialogOpts{
		Title:       i18n.TrafficCertRemoveTitle(),
		Description: i18n.TrafficCertRemoveBody(),
		Cancel:      i18n.Cancel(),
		Confirm:     i18n.TrafficCertRemove(),
		Destructive: true,
		OnConfirm:   func() { a.trafficCertAction(t, st, true) },
	})
}

// trafficCertAction installs or removes the trust in the certificate off
// the UI thread: the system's own password prompt is in the way.
func (a *App) trafficCertAction(t tool.Traffic, st *trafficState, remove bool) {
	st.begin()
	st.err = nil
	a.opt.Runner.Go(context.Background(), t.Manifest().ID, trafficPromptTimeout, func(ctx context.Context) {
		var err error
		if remove {
			err = t.RemoveCert(ctx)
		} else {
			err = t.InstallCert(ctx)
		}
		a.tell(st, trafficOutcome{err: err})
		a.trafficLoadCert(ctx, t, st)
	})
}

// trafficCertRefresh reads the certificate's state again every
// trafficCertAge, so that a trust changed elsewhere shows here too.
func (a *App) trafficCertRefresh(c *ui.Context, t tool.Traffic, st *trafficState) {
	age := time.Since(st.certAt)
	if age < trafficCertAge {
		c.After(trafficCertAge - age)
		return
	}
	a.trafficLoadCert(context.Background(), t, st)
}

// trafficLoadCert reads the certificate's state off the UI thread: on
// macOS that asks the keychain.
func (a *App) trafficLoadCert(ctx context.Context, t tool.Traffic, st *trafficState) {
	if a.trafficBusy(st) {
		return
	}
	st.begin()
	a.opt.Runner.Go(ctx, t.Manifest().ID, trafficTimeout, func(context.Context) {
		cs, err := t.Cert()
		a.tell(st, trafficOutcome{err: err, cert: cs, hasCert: true})
	})
}
