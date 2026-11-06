package tool

// Risk says what an action can change; docs/agents.md maps it to MCP
// annotations and to whether a person must confirm.
type Risk string

// The risks, from none to most.
const (
	RiskRead    Risk = "read"    // changes nothing
	RiskWrite   Risk = "write"   // changes files
	RiskProcess Risk = "process" // ends or starts processes
	RiskSystem  Risk = "system"  // changes system state, such as sleep
)

// Risks lists the valid risks.
func Risks() []Risk { return []Risk{RiskRead, RiskWrite, RiskProcess, RiskSystem} }

// Type is the type of a schema, in JSON Schema's words.
type Type string

// The types of the JSON Schema subset tools use.
const (
	TypeObject  Type = "object"
	TypeString  Type = "string"
	TypeInteger Type = "integer"
	TypeNumber  Type = "number"
	TypeBoolean Type = "boolean"
	TypeArray   Type = "array"
)

// FormatPath marks a string that is a path on this computer.
const FormatPath = "path"

// Schema is a typed description of a value: a subset of JSON Schema with
// objects, strings, numbers, booleans, enums, arrays and paths. Texts are
// catalog messages, so every front end shows them in its language.
type Schema struct {
	Type        Type
	Format      string // FormatPath, or empty
	Title       Message
	Description Message
	Hint        Message // an example of a value, a form's placeholder
	Enum        []string
	Minimum     *float64
	Maximum     *float64
	Properties  []Property // of an object, in the order forms show them
	Items       *Schema    // of an array
}

// Property is a named member of an object schema.
type Property struct {
	Name     string
	Schema   Schema
	Required bool
}

// Spec declares one action of a tool: what it takes, what it returns, what
// it can change and whether running it again changes more.
type Spec struct {
	Name       string // stable English identifier: "plan", "apply", "start"
	Summary    Message
	Input      Schema // an object
	Output     Schema // an object
	Risk       Risk
	Idempotent bool
}

// Bounds returns a pointer to v, for a schema's Minimum and Maximum.
func Bounds(v float64) *float64 { return &v }

// Names of the actions every action tool has.
const (
	ActionPlan  = "plan"
	ActionApply = "apply"
)

// Names of the actions every switch tool has.
const (
	ActionStart  = "start"
	ActionStop   = "stop"
	ActionStatus = "status"
)
