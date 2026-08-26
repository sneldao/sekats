package verdicts

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/udingethe/sekats/internal/store"
)

func TestRecordPersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdicts.json")

	s1 := NewStore(path)
	if err := s1.Record(101, "accepted"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := s1.Record(202, "rejected"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	s2 := NewStore(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := s2.All()
	if len(got) != 2 {
		t.Fatalf("want 2 entries after reload, got %d", len(got))
	}
	if got[0].DecisionID != 101 || got[0].Verdict != "accepted" {
		t.Errorf("entry 0 = %+v, want id 101 accepted", got[0])
	}
	if got[1].DecisionID != 202 || got[1].Verdict != "rejected" {
		t.Errorf("entry 1 = %+v, want id 202 rejected", got[1])
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nope.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if got := s.All(); len(got) != 0 {
		t.Fatalf("want empty store, got %d entries", len(got))
	}
}

func TestRecordOverwritesVerdictForSameDecision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdicts.json")
	s := NewStore(path)
	if err := s.Record(7, "accepted"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := s.Record(7, "rejected"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	e, ok := s.Lookup(7)
	if !ok || e.Verdict != "rejected" {
		t.Fatalf("Lookup(7) = %+v ok=%v, want rejected", e, ok)
	}
}

func sampleRows() []store.Disagreement {
	return []store.Disagreement{
		{DecisionID: 101, HandCode: "AsKd", Street: "flop", Pot: 12.5, OurAction: "bet", LLMAction: "check"},
		{DecisionID: 202, HandCode: "7c7d", Street: "turn", Pot: 30.0, OurAction: "bet", LLMAction: "check"},
	}
}

func TestBuildReportJoinsVerdictsToFixtureRows(t *testing.T) {
	vs := NewStore("")
	_ = vs.Record(101, "accepted")
	_ = vs.Record(999, "rejected") // no matching fixture row

	md, joinErrs := BuildReport(sampleRows(), vs.All())
	if len(joinErrs) != 1 || joinErrs[0].DecisionID != 999 {
		t.Fatalf("joinErrs = %+v, want exactly decision 999", joinErrs)
	}
	for _, want := range []string{"AsKd", "flop", "12.50", "accepted"} {
		if !contains(md, want) {
			t.Errorf("report missing %q\nreport:\n%s", want, md)
		}
	}
}

func TestBuildReportEmptyVerdicts(t *testing.T) {
	md, joinErrs := BuildReport(sampleRows(), NewStore("").All())
	if len(joinErrs) != 0 {
		t.Fatalf("want no join errors, got %+v", joinErrs)
	}
	if !contains(md, "No verdicts recorded yet") {
		t.Errorf("empty report should say so explicitly:\n%s", md)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
