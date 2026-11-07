package comp

import (
	"github.com/egoist/mygo/ui"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/ui/ds"
)

// DialogItem is one affected item of a Plan, shown in a Dialog.
type DialogItem struct {
	Process *tool.Process
	Text    string
}

// DialogOpts describe a modal confirmation dialog.
type DialogOpts struct {
	Title       string       // a question naming the count ("Kill 3 dev processes?")
	Description string       // one sentence of consequence
	Items       []DialogItem // at most DialogMaxItems shown, then MoreText
	MoreText    string       // "and 4 more" when len(Items) > DialogMaxItems
	DontAsk     *bool        // nil when there is no "don't ask again"
	DontAskText string
	Cancel      string // "Cancel"
	Confirm     string // the verb and count ("Kill 3 Processes")
	Destructive bool   // Cancel is the default, confirm is danger-solid
	Processes   *ProcessIcons
	OnConfirm   func()
}

// Dialog is a window-modal confirmation for an action that cannot be
// undone, shown after a Plan and before Apply. Cancel is first and the
// action is at the trailing edge, on every platform; for a destructive
// action Cancel holds the focus, so Enter and Esc both cancel.
func (k *Kit) Dialog(c *ui.Context, open *bool, o DialogOpts) {
	p := ds.P(c)
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.Scrim)
		ds.ApplyShadow(panel.Width(ds.DialogWidth).Column().Radius(ds.RadiusMd).Background(p.BgPopover).
			Border(ds.LineWidth, p.LineControl).Clip(), ds.S(c).ShadowPopover)
		ui.Column(c).FillWidth().Padding(ds.Space20).Gap(ds.Space10).Children(func() {
			Text(c, o.Title, ds.Headline, p.TextStrong)
			if o.Description != "" {
				desc := ds.Label
				desc.Weight = ds.Regular
				Text(c, o.Description, desc, p.TextSecondary)
			}
			k.dialogItems(c, o)
			if o.DontAsk != nil {
				k.Checkbox(c, o.DontAsk, o.DontAskText, false)
			}
		})
		k.dialogButtons(c, open, o)
	})
}

func (k *Kit) dialogItems(c *ui.Context, o DialogOpts) {
	if len(o.Items) == 0 {
		return
	}
	p := ds.P(c)
	shown := o.Items[:min(len(o.Items), ds.DialogMaxItems)]
	ui.Column(c).FillWidth().Padding(ds.Space8, ds.Space10).Gap(ds.Space4).Radius(ds.RadiusSm).
		Background(p.FillControl).Children(func() {
		for i, it := range shown {
			ui.Row(c.Key(i)).Gap(ds.Space8).AlignItems(ui.Center).Children(func() {
				if it.Process != nil {
					k.ProcessIcon(c, o.Processes, *it.Process)
				}
				Value(c, it.Text, p.TextPrimary)
			})
		}
		if o.MoreText != "" {
			Value(c, o.MoreText, p.TextMuted)
		}
	})
}

func (k *Kit) dialogButtons(c *ui.Context, open *bool, o DialogOpts) {
	ui.Row(c).FillWidth().Padding(0, ds.Space20, ds.Space20).Gap(ds.Space8).Justify(ui.End).Children(func() {
		cancel := k.Button(c.Key("cancel"), o.Cancel, ButtonOpts{Kind: Secondary})
		if o.Destructive {
			cancel.AutoFocus()
		}
		if cancel.Clicked() {
			*open = false
		}
		kind := Primary
		if o.Destructive {
			kind = DangerSolid
		}
		confirm := k.Button(c.Key("confirm"), o.Confirm, ButtonOpts{Kind: kind})
		if !o.Destructive {
			confirm.AutoFocus()
		}
		if confirm.Clicked() {
			*open = false
			if o.OnConfirm != nil {
				o.OnConfirm()
			}
		}
	})
}
