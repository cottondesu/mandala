package mandala

import (
	"reflect"
	"testing"
)

func TestGapsExpandedParent(t *testing.T) {
	s := State{SchemaVersion: 1, Goal: "Implement OAuth", Cells: []Cell{
		{ID: "security", Status: Expanded, Required: true},
		{ID: "security.csrf", Parent: "security", Status: Done, Required: true},
		{ID: "security.token-storage", Parent: "security", Status: Open, Required: true},
		{ID: "tests", Status: Open, Required: true},
		{ID: "compatibility", Status: Open, Required: true},
	}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	got := s.Gaps(true)
	want := []Gap{{ID: "compatibility", Required: true}, {ID: "security.token-storage", Required: true}, {ID: "tests", Required: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gaps = %#v, want %#v", got, want)
	}
}

func TestAddLimitsAndOptionalInheritance(t *testing.T) {
	s := State{SchemaVersion: 1, Goal: "goal", Cells: []Cell{}}
	if err := s.Add("optional", true); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("optional.child", false); err != nil {
		t.Fatal(err)
	}
	if s.Cells[1].Required {
		t.Fatal("child of optional cell became required")
	}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		if err := s.Add(id, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Add("ninth", false); err == nil {
		t.Fatal("ninth root cell was accepted")
	}
}

func TestValidateRejectsCorruptRelations(t *testing.T) {
	tests := []struct {
		name  string
		cells []Cell
	}{
		{"duplicate", []Cell{{ID: "a", Status: Open, Required: true}, {ID: "a", Status: Open, Required: true}}},
		{"missing parent", []Cell{{ID: "a.b", Parent: "a", Status: Open, Required: true}}},
		{"cycle", []Cell{{ID: "a", Parent: "a.b", Status: Expanded, Required: true}, {ID: "a.b", Parent: "a", Status: Expanded, Required: true}}},
		{"invalid status", []Cell{{ID: "a", Status: "blocked", Required: true}}},
		{"invalid parent", []Cell{{ID: "a", Status: Expanded, Required: true}, {ID: "a.b", Parent: "other", Status: Open, Required: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := State{SchemaVersion: 1, Goal: "goal", Cells: tt.cells}
			if err := s.Validate(); err == nil {
				t.Fatal("invalid state was accepted")
			}
		})
	}
}

func TestMarkNotApplicableAndReopen(t *testing.T) {
	s := State{SchemaVersion: 1, Goal: "goal", Cells: []Cell{{ID: "a", Status: Open, Required: true}}}
	if err := s.Mark("a", NotApplicable); err != nil {
		t.Fatal(err)
	}
	if len(s.Gaps(true)) != 0 {
		t.Fatal("not-applicable cell is a gap")
	}
	if err := s.Mark("a", Open); err != nil {
		t.Fatal(err)
	}
	if len(s.Gaps(true)) != 1 {
		t.Fatal("reopened cell is not a gap")
	}
}

func TestAddRejectsNinthChild(t *testing.T) {
	s, _ := NewState("goal")
	if err := s.Add("parent", false); err != nil {
		t.Fatal(err)
	}
	for _, child := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		if err := s.Add("parent."+child, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Add("parent.i", false); err == nil {
		t.Fatal("ninth child was accepted")
	}
	if len(s.Cells) != 9 {
		t.Fatal("rejected add mutated state")
	}
}

func TestAddRequiresExistingOpenParent(t *testing.T) {
	s, _ := NewState("goal")
	if err := s.Add("missing.child", false); err == nil {
		t.Fatal("missing parent was accepted")
	}
	if err := s.Add("parent", false); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark("parent", Done); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("parent.child", false); err == nil {
		t.Fatal("done parent was expanded")
	}
	if err := s.Mark("parent", Open); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("parent.child", false); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark("parent", Done); err == nil {
		t.Fatal("expanded parent was marked done")
	}
}

func TestOptionalGapDoesNotBlockCompletion(t *testing.T) {
	s, _ := NewState("goal")
	if err := s.Add("optional", true); err != nil {
		t.Fatal(err)
	}
	if len(s.Gaps(true)) != 0 || len(s.Gaps(false)) != 1 {
		t.Fatalf("optional gaps: required=%v all=%v", s.Gaps(true), s.Gaps(false))
	}
}

func TestGoalRejectsUnicodeLineSeparators(t *testing.T) {
	for _, separator := range []rune{'\u2028', '\u2029'} {
		if _, err := NewState("first" + string(separator) + "second"); err == nil {
			t.Fatalf("accepted goal containing U+%04X line separator", separator)
		}
	}
}
