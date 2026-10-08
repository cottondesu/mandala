package mandala

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRenderStatusJSONCounts(t *testing.T) {
	for _, tt := range []struct {
		name     string
		cells    []Cell
		groups   int
		required statusCounts
		optional statusCounts
	}{
		{name: "empty", cells: []Cell{}},
		{name: "required open", cells: []Cell{{ID: "a", Status: Open, Required: true}}, required: statusCounts{Open: 1}},
		{name: "required done", cells: []Cell{{ID: "a", Status: Done, Required: true}}, required: statusCounts{Done: 1}},
		{name: "required na", cells: []Cell{{ID: "a", Status: NotApplicable, Required: true}}, required: statusCounts{NA: 1}},
		{name: "optional open", cells: []Cell{{ID: "a", Status: Open}}, optional: statusCounts{Open: 1}},
		{name: "optional done", cells: []Cell{{ID: "a", Status: Done}}, optional: statusCounts{Done: 1}},
		{name: "optional na", cells: []Cell{{ID: "a", Status: NotApplicable}}, optional: statusCounts{NA: 1}},
		{name: "expanded required", cells: []Cell{
			{ID: "a", Status: Expanded, Required: true},
			{ID: "a.b", Parent: "a", Status: Open, Required: true},
		}, groups: 1, required: statusCounts{Open: 1}},
		{name: "expanded optional", cells: []Cell{
			{ID: "a", Status: Expanded},
			{ID: "a.b", Parent: "a", Status: Done},
		}, groups: 1, optional: statusCounts{Done: 1}},
		{name: "mixed tree", cells: []Cell{
			{ID: "a", Status: Expanded, Required: true},
			{ID: "a.open", Parent: "a", Status: Open, Required: true},
			{ID: "a.done", Parent: "a", Status: Done, Required: true},
			{ID: "a.na", Parent: "a", Status: NotApplicable, Required: true},
			{ID: "a.optional", Parent: "a", Status: Open},
			{ID: "b", Status: Expanded},
			{ID: "b.open", Parent: "b", Status: Open},
			{ID: "b.done", Parent: "b", Status: Done},
			{ID: "b.na", Parent: "b", Status: NotApplicable},
		}, groups: 2, required: statusCounts{Open: 1, Done: 1, NA: 1}, optional: statusCounts{Open: 2, Done: 1, NA: 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := State{SchemaVersion: 1, Goal: "Example", Cells: tt.cells}
			if err := s.Validate(); err != nil {
				t.Fatal(err)
			}
			out, err := renderStatusJSON(s)
			if err != nil {
				t.Fatal(err)
			}
			var got statusResponse
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err)
			}
			want := statusResponse{SchemaVersion: 1, Goal: "Example", Cells: len(tt.cells), Groups: tt.groups,
				Required: tt.required, Optional: tt.optional, RequiredGaps: tt.required.Open}
			if got != want {
				t.Fatalf("status: got %+v, want %+v", got, want)
			}
			root := t.TempDir()
			if err := Init(root, s); err != nil {
				t.Fatal(err)
			}
			code, commandOut, diagnostics := runTestCommand(root, "status", "--json")
			wantCode := 0
			if tt.required.Open > 0 {
				wantCode = 1
			}
			if code != wantCode || commandOut != out || diagnostics != "" {
				t.Fatalf("command: exit=%d stdout=%q stderr=%q", code, commandOut, diagnostics)
			}
			leaves := got.Required.Open + got.Required.Done + got.Required.NA + got.Optional.Open + got.Optional.Done + got.Optional.NA
			if got.Cells != got.Groups+leaves || got.RequiredGaps != got.Required.Open {
				t.Fatalf("counting invariants failed: %+v", got)
			}
			c := s.Counts()
			if got.Groups != c.Groups || got.Required != (statusCounts{c.RequiredOpen, c.RequiredDone, c.RequiredNA}) ||
				got.Optional != (statusCounts{c.OptionalOpen, c.OptionalDone, c.OptionalNA}) {
				t.Fatalf("status disagrees with State.Counts(): %+v / %+v", got, c)
			}
		})
	}
}

func TestRenderStatusJSONExactEncoding(t *testing.T) {
	for _, tt := range []struct {
		goal string
		want string
	}{
		{"Example", `{"schema_version":1,"goal":"Example","cells":0,"groups":0,"required":{"open":0,"done":0,"na":0},"optional":{"open":0,"done":0,"na":0},"required_gaps":0}` + "\n"},
		{`OAuth "認証" <>&`, `{"schema_version":1,"goal":"OAuth \"認証\" \u003c\u003e\u0026","cells":0,"groups":0,"required":{"open":0,"done":0,"na":0},"optional":{"open":0,"done":0,"na":0},"required_gaps":0}` + "\n"},
		{`path\認証 😀 é`, `{"schema_version":1,"goal":"path\\認証 😀 é","cells":0,"groups":0,"required":{"open":0,"done":0,"na":0},"optional":{"open":0,"done":0,"na":0},"required_gaps":0}` + "\n"},
	} {
		t.Run(tt.goal, func(t *testing.T) {
			s, err := NewState(tt.goal)
			if err != nil {
				t.Fatal(err)
			}
			out, err := renderStatusJSON(s)
			if err != nil {
				t.Fatal(err)
			}
			if out != tt.want || strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
				t.Fatalf("exact output: got %q, want %q", out, tt.want)
			}
			var decoded struct {
				Goal string `json:"goal"`
			}
			if err := json.Unmarshal([]byte(out), &decoded); err != nil || decoded.Goal != tt.goal {
				t.Fatalf("goal round trip: got %q, err=%v", decoded.Goal, err)
			}
		})
	}
}

func TestRunStatusJSONMaximumCapacity(t *testing.T) {
	s, err := NewState("Maximum")
	if err != nil {
		t.Fatal(err)
	}
	statuses := []Status{Open, Done, NotApplicable, Open, Done, NotApplicable, Open, Done}
	for i, parent := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		if err := s.Add(parent, i >= 4); err != nil {
			t.Fatal(err)
		}
		for j, child := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
			id := parent + "." + child
			if err := s.Add(id, j >= 3 && j <= 5); err != nil {
				t.Fatal(err)
			}
			if err := s.Mark(id, statuses[j]); err != nil {
				t.Fatal(err)
			}
		}
	}
	root := t.TempDir()
	if err := Init(root, s); err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":1,"goal":"Maximum","cells":72,"groups":8,"required":{"open":8,"done":8,"na":4},"optional":{"open":16,"done":16,"na":12},"required_gaps":8}` + "\n"
	code, out, diagnostics := runTestCommand(root, "status", "--json")
	if code != 1 || out != want || diagnostics != "" {
		t.Fatalf("maximum: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	slices.Reverse(s.Cells)
	reordered, err := renderStatusJSON(s)
	if err != nil || reordered != want {
		t.Fatalf("maximum reordered: stdout=%q err=%v", reordered, err)
	}
}
