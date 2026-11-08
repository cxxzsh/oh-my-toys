package traffic

import (
	"errors"
	"io/fs"
	"log"
	"os"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Rules returns the rule book in use, as a copy a person may change
// without touching the capture.
func (t *Tool) Rules() []tool.Rule { return t.rules.Get() }

// SetRules replaces the rule book: it takes effect at once, on the capture
// in progress, and is kept for the next launch. The editor saves as a
// person types, so the swap and the save of one book are held together.
func (t *Tool) SetRules(list []tool.Rule) error {
	t.rulesMu.Lock()
	defer t.rulesMu.Unlock()
	if err := t.rules.Set(list); err != nil {
		return err
	}
	return writeJSON(t.path(rulesFile), list)
}

// loadRules reads the saved rule book into the engine. A missing file is
// an empty book; a broken or unusable one is moved aside, so that the next
// save does not overwrite what the person wrote, and the capture runs
// without rules, which is better than not starting.
func (t *Tool) loadRules() {
	path := t.path(rulesFile)
	var list []tool.Rule
	if err := readJSON(path, &list); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			t.setRulesAside(path, err)
		}
		return
	}
	if err := t.rules.Set(list); err != nil {
		t.setRulesAside(path, err)
	}
}

// setRulesAside keeps a book the engine refuses, for a person to look at.
func (t *Tool) setRulesAside(path string, cause error) {
	broken := path + brokenSuffix
	log.Printf("traffic: %v; capturing without rules, moved to %s", cause, broken)
	_ = os.Rename(path, broken)
}
