package flowlog

import (
	"testing"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestRowsCachesTheLastResult(t *testing.T) {
	log := New(10, 1<<20)
	log.Put(sizedFlow(1, "one"))
	log.Put(sizedFlow(1, "two"))
	f := tool.Filter{Source: tool.SourceProxy}

	first := log.Rows(f)
	second := log.Rows(f)
	if len(first) != 2 || &first[0] != &second[0] {
		t.Fatal("Rows built again without a change")
	}

	log.Put(sizedFlow(1, "three"))
	third := log.Rows(f)
	if len(third) != 3 || &third[0] == &first[0] {
		t.Fatal("Rows did not pick up the new flow")
	}
	if ids := idsOf(third); len(ids) != 3 || ids[2] != 3 {
		t.Fatalf("IDs %v", ids)
	}
}

func TestRowsCacheKeepsItsOwnFilter(t *testing.T) {
	log := New(10, 1<<20)
	log.Put(sizedFlow(1, "one"))
	methods := []string{"GET"}
	f := tool.Filter{Source: tool.SourceProxy, Methods: methods}
	first := log.Rows(f)

	methods[0] = "POST"

	again := log.Rows(tool.Filter{Source: tool.SourceProxy, Methods: []string{"GET"}})
	if len(first) != 1 || len(again) != 1 || &first[0] != &again[0] {
		t.Fatal("a changed filter slice reached the cache key")
	}
}

func TestRowsReloadsAfterClear(t *testing.T) {
	log := New(10, 1<<20)
	log.Put(sizedFlow(1, "one"))
	f := tool.Filter{Source: tool.SourceProxy}
	first := log.Rows(f)
	log.Clear()
	if rows := log.Rows(f); len(rows) != 0 {
		t.Fatalf("rows after clear %+v", rows)
	}
	log.Put(sizedFlow(1, "two"))
	second := log.Rows(f)
	if len(second) != 1 || &second[0] == &first[0] {
		t.Fatal("Rows reused the result from before the clear")
	}
}
