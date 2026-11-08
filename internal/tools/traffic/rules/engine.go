package rules

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// The errors the engine reports.
var (
	// ErrInvalidRule is what Set returns when it cannot compile a rule; the
	// error names the rule at fault.
	ErrInvalidRule = errors.New("invalid rule")
	// ErrBodyTooLarge is what Request and Response report when they skipped
	// a body edit, because a body larger than tool.MaxBody is never held in
	// memory to be rewritten.
	ErrBodyTooLarge = errors.New("body too large to edit")
)

// maxDelay is the longest a delay rule may hold a response back; a longer
// one is far more likely to be a typing mistake than an intention.
const maxDelay = 5 * time.Minute

// The status codes a rule may set. A 1xx is an interim answer the proxy
// cannot rewrite or mock without breaking the exchange, so a rule may only
// speak for a final one.
const (
	minStatus = 200
	maxStatus = 599
)

// Engine holds a rule book and applies it to the exchanges of the proxy.
// It is safe for any goroutine: Set publishes a whole new book at once, so
// an exchange sees either the old book or the new one, never a mix.
type Engine struct {
	current atomic.Pointer[book]
}

// New makes an engine whose rule book is empty.
func New() *Engine { return &Engine{} }

// Set validates and compiles rules and swaps them in atomically. On error
// the book in use is left as it was.
func (e *Engine) Set(rules []tool.Rule) error {
	b, err := compileBook(rules)
	if err != nil {
		return err
	}
	e.current.Store(b)
	return nil
}

// Get returns the rules of the book in use, as a copy that may be changed
// without touching the engine.
func (e *Engine) Get() []tool.Rule {
	b := e.current.Load()
	if b == nil {
		return nil
	}
	return b.export()
}

// book is a compiled rule book; once Set publishes it, nothing changes it.
type book struct {
	rules []rule
}

// export rebuilds the rules of the book as the caller set them.
func (b *book) export() []tool.Rule {
	out := make([]tool.Rule, len(b.rules))
	for i, r := range b.rules {
		out[i] = copyRule(r.raw)
	}
	return out
}

// invalidRule is the error Set returns for a rule it cannot compile, named
// by the rule's name or, failing that, by its place in the book.
func invalidRule(i int, r tool.Rule, msg string) error {
	return fmt.Errorf("%s: %w: %s", ruleWhere(i, r), ErrInvalidRule, msg)
}

// ruleWhere names a rule in an error.
func ruleWhere(i int, r tool.Rule) string {
	if r.Name != "" {
		return fmt.Sprintf("rule %q", r.Name)
	}
	return fmt.Sprintf("rule #%d", i)
}

// checkStatus rejects a status code a rule asks to answer with.
func checkStatus(i int, r tool.Rule, status int) error {
	if status == 0 || (status >= minStatus && status <= maxStatus) {
		return nil
	}
	return invalidRule(i, r, fmt.Sprintf("status %d is not between %d and %d", status, minStatus, maxStatus))
}
