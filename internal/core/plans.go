package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// planTTL is how long a plan may be applied, from docs/agents.md section 2.
const planTTL = 10 * time.Minute

// planID is the shape of an id this store writes: 8 random bytes, hex.
var planID = regexp.MustCompile(`^[0-9a-f]{16}$`)

var (
	errPlanMissing = errors.New("plan not found")
	errPlanExpired = errors.New("plan expired")
	errPlanTool    = errors.New("plan is for another tool")
)

// wrongToolError is a plan applied to a different tool than the one that made it.
type wrongToolError struct{ Tool string }

func (e *wrongToolError) Error() string { return errPlanTool.Error() }

func (e *wrongToolError) Unwrap() error { return errPlanTool }

// PlanStore keeps plans on disk so a later apply can find them. A plan is
// bound to the inputs that produced it and dies after planTTL.
type PlanStore struct {
	Dir string
	Now func() time.Time // tests set this; nil means time.Now
}

type savedPlan struct {
	ID       string     `json:"id"`
	Tool     string     `json:"tool"`
	Inputs   tool.Input `json:"inputs"`
	Body     planBody   `json:"body"`
	Created  time.Time  `json:"created"`
	bodyHash string
}

// planBody is the part of a plan apply re-checks. It leaves out the tool's
// private Data, which is rebuilt by planning again.
type planBody struct {
	Summary     string     `json:"summary"`
	Rows        []savedRow `json:"rows"`
	ApplyLabel  string     `json:"apply_label"`
	Destructive bool       `json:"destructive"`
}

type savedRow struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Value  string `json:"value"`
	Tone   string `json:"tone"`
}

func (s PlanStore) clock() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// save stores a plan and returns its id.
func (s PlanStore) save(toolID string, in tool.Input, p tool.Plan) (string, error) {
	id, err := newPlanID()
	if err != nil {
		return "", err
	}
	saved := savedPlan{
		ID: id, Tool: toolID, Inputs: in, Body: bodyOf(p), Created: s.clock(),
	}
	b, err := json.Marshal(saved)
	if err != nil {
		return "", err
	}
	if err := s.write(id, b); err != nil {
		return "", err
	}
	return id, nil
}

// load reads a plan for toolID. It fails when the id is unknown, the plan
// has expired, or it was made for another tool.
func (s PlanStore) load(id, toolID string) (savedPlan, error) {
	if !planID.MatchString(id) {
		return savedPlan{}, errPlanMissing
	}
	b, err := s.read(id)
	if errors.Is(err, os.ErrNotExist) {
		return savedPlan{}, errPlanMissing
	}
	if err != nil {
		return savedPlan{}, err
	}
	var saved savedPlan
	if err := json.Unmarshal(b, &saved); err != nil {
		return savedPlan{}, fmt.Errorf("read plan %s: %w", id, err)
	}
	if saved.Tool != toolID {
		return savedPlan{}, &wrongToolError{Tool: saved.Tool}
	}
	if s.clock().Sub(saved.Created) > planTTL {
		return savedPlan{}, fmt.Errorf("plan %s: %w", id, errPlanExpired)
	}
	saved.bodyHash = mustHash(saved.Body)
	return saved, nil
}

// Matches reports whether planning again produced the same changes. Apply
// uses that to refuse a plan whose files or processes have moved.
func (saved savedPlan) Matches(p tool.Plan) bool {
	return saved.bodyHash == mustHash(bodyOf(p))
}

func bodyOf(p tool.Plan) planBody {
	rows := make([]savedRow, len(p.Rows))
	for i, r := range p.Rows {
		rows[i] = savedRow{Title: r.Title, Detail: r.Detail, Value: r.Value, Tone: tool.ToneName(r.Tone)}
	}
	return planBody{Summary: p.Summary, Rows: rows, ApplyLabel: p.ApplyLabel, Destructive: p.Destructive}
}

func mustHash(body planBody) string {
	b, err := json.Marshal(body)
	if err != nil {
		return ""
	}
	return string(b)
}

func newPlanID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("plan id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func (s PlanStore) write(id string, b []byte) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("plan dir: %w", err)
	}
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return fmt.Errorf("plan dir: %w", err)
	}
	defer func() { _ = root.Close() }() // directory handle; the file close is checked
	f, err := root.OpenFile(id+".json", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write plan %s: %w", id, err)
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil {
		return fmt.Errorf("write plan %s: %w", id, werr)
	}
	if cerr != nil {
		return fmt.Errorf("write plan %s: %w", id, cerr)
	}
	return nil
}

func (s PlanStore) read(id string) ([]byte, error) {
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }() // only read
	return root.ReadFile(id + ".json")
}
