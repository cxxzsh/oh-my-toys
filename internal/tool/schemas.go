package tool

import "github.com/cxxzsh/oh-my-toys/internal/i18n"

// Tone names, as the output of a plan spells them.
const (
	toneNormal  = "normal"
	toneSuccess = "success"
	toneWarning = "warning"
	toneDanger  = "danger"
)

// ToneName is the name of a tone in a plan's output.
func ToneName(t Tone) string {
	switch t {
	case ToneSuccess:
		return toneSuccess
	case ToneWarning:
		return toneWarning
	case ToneDanger:
		return toneDanger
	default:
		return toneNormal
	}
}

func text(title Message) Schema { return Schema{Type: TypeString, Title: title} }

// NoInput is the input of an action that takes nothing.
func NoInput() Schema { return Schema{Type: TypeObject} }

// PlanOutput is what every plan returns: a summary and the changes it
// would make, which change nothing yet.
func PlanOutput() Schema {
	change := Schema{Type: TypeObject, Properties: []Property{
		{Name: "title", Schema: text(i18n.Messages.SchemaChangeTitle), Required: true},
		{Name: "detail", Schema: text(i18n.Messages.SchemaChangeDetail)},
		{Name: "value", Schema: text(i18n.Messages.SchemaChangeValue)},
		{Name: "tone", Schema: Schema{
			Type: TypeString, Title: i18n.Messages.SchemaChangeTone,
			Enum: []string{toneNormal, toneSuccess, toneWarning, toneDanger},
		}},
	}}
	return Schema{Type: TypeObject, Properties: []Property{
		{Name: "summary", Schema: text(i18n.Messages.SchemaPlanSummary), Required: true},
		{Name: "changes", Schema: Schema{Type: TypeArray, Title: i18n.Messages.SchemaPlanChanges, Items: &change}, Required: true},
		{Name: "apply_label", Schema: text(i18n.Messages.SchemaPlanApplyLabel)},
		{Name: "destructive", Schema: Schema{Type: TypeBoolean, Title: i18n.Messages.SchemaPlanDestructive}},
		{Name: "plan_id", Schema: text(i18n.Messages.SchemaPlanID), Required: true},
	}}
}

// ApplyInput is what every apply takes: the plan to carry out, by its ID
// (docs/agents.md section 2).
func ApplyInput() Schema {
	return Schema{Type: TypeObject, Properties: []Property{
		{Name: "plan_id", Schema: text(i18n.Messages.SchemaPlanID), Required: true},
	}}
}

// ApplyOutput is what every apply returns.
func ApplyOutput() Schema {
	return Schema{Type: TypeObject, Properties: []Property{
		{Name: "summary", Schema: text(i18n.Messages.SchemaApplySummary), Required: true},
	}}
}

// StatusOutput is what a switch tool's start, stop and status return.
func StatusOutput() Schema {
	return Schema{Type: TypeObject, Properties: []Property{
		{Name: "on", Schema: Schema{Type: TypeBoolean, Title: i18n.Messages.SchemaStatusOn}, Required: true},
		{Name: "text", Schema: text(i18n.Messages.SchemaStatusText), Required: true},
		{Name: "detail", Schema: text(i18n.Messages.SchemaStatusDetail)},
		{Name: "note", Schema: text(i18n.Messages.SchemaStatusNote)},
	}}
}

// SwitchSpecs are the start, stop and status actions of a switch tool,
// which change system state through start and stop.
func SwitchSpecs(start, stop, status Message) []Spec {
	return switchSpecs(start, stop, status, NoInput())
}

// TimedSwitchSpecs are SwitchSpecs for a Timed tool: start also takes how
// long to run, as a length ("for") or a time of day ("until"), either one.
func TimedSwitchSpecs(start, stop, status Message) []Spec {
	return switchSpecs(start, stop, status, Schema{Type: TypeObject, Properties: []Property{
		{Name: PropFor, Schema: text(i18n.Messages.SchemaHoldFor)},
		{Name: PropUntil, Schema: text(i18n.Messages.SchemaHoldUntil)},
	}})
}

func switchSpecs(start, stop, status Message, startInput Schema) []Spec {
	return []Spec{
		{Name: ActionStart, Summary: start, Input: startInput, Output: StatusOutput(), Risk: RiskSystem, Idempotent: true},
		{Name: ActionStop, Summary: stop, Input: NoInput(), Output: StatusOutput(), Risk: RiskSystem, Idempotent: true},
		{Name: ActionStatus, Summary: status, Input: NoInput(), Output: StatusOutput(), Risk: RiskRead, Idempotent: true},
	}
}

// PlanSpec is an action tool's plan: it reads input and changes nothing.
func PlanSpec(summary Message, input Schema) Spec {
	return Spec{Name: ActionPlan, Summary: summary, Input: input, Output: PlanOutput(), Risk: RiskRead, Idempotent: true}
}

// ApplySpec is an action tool's apply, of a given risk; running a plan a
// second time finds its changes already made, so it is not idempotent.
func ApplySpec(summary Message, risk Risk) Spec {
	return Spec{Name: ActionApply, Summary: summary, Input: ApplyInput(), Output: ApplyOutput(), Risk: risk}
}
