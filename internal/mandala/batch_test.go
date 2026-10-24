package mandala

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// runBatchCommand runs `mandala batch` with the given arguments and stdin
// content, returning exit code, stdout, and stderr.
func runBatchCommand(start, stdinContent string, args ...string) (int, string, string) {
	var out, diagnostics bytes.Buffer
	code := run(append([]string{"batch"}, args...), start, strings.NewReader(stdinContent), &out, &diagnostics, "test")
	return code, out.String(), diagnostics.String()
}

func writeBatchFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func initBatchProject(t *testing.T, goal string) string {
	t.Helper()
	root := t.TempDir()
	if code, _, diagnostics := runTestCommand(root, "init", goal); code != 0 {
		t.Fatalf("init: %s", diagnostics)
	}
	return root
}

func batchStatePath(root string) string {
	return filepath.Join(root, ".mandala", "state.json")
}

func readBatchState(t *testing.T, root string) []byte {
	t.Helper()
	data, err := os.ReadFile(batchStatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func batchStateModTime(t *testing.T, root string) time.Time {
	t.Helper()
	info, err := os.Stat(batchStatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	return info.ModTime()
}

// requireOnlyStateFile asserts the .mandala directory contains exactly
// state.json, proving no temporary files survived a failure path.
func requireOnlyStateFile(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".mandala"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{"state.json"}) {
		t.Fatalf("unexpected .mandala contents: %v", names)
	}
}

func batchDocument(ops ...string) string {
	return `{"batch_version":1,"operations":[` + strings.Join(ops, ",") + `]}`
}

func batchAdd(id string) string { return `{"op":"add","id":` + quoteJSON(id) + `}` }
func batchAddOptional(id string) string {
	return `{"op":"add","id":` + quoteJSON(id) + `,"optional":true}`
}
func batchMark(id, status string) string {
	return `{"op":"mark","id":` + quoteJSON(id) + `,"status":` + quoteJSON(status) + `}`
}

func quoteJSON(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			builder.WriteString("\\\"")
		case '\\':
			builder.WriteString("\\\\")
		default:
			builder.WriteRune(r)
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

// requireStateUnchanged asserts state.json kept its exact bytes, mtime, and
// file identity. A save replaces the file via rename, so the inode or file ID
// would change even if bytes and a coarse mtime looked identical.
func requireStateUnchanged(t *testing.T, root string, before []byte, mtime time.Time, identity os.FileInfo) {
	t.Helper()
	if after := readBatchState(t, root); !bytes.Equal(after, before) {
		t.Fatalf("state bytes changed:\nbefore=%s\nafter=%s", before, after)
	}
	if after := batchStateModTime(t, root); !after.Equal(mtime) {
		t.Fatalf("state mtime changed: before=%v after=%v", mtime, after)
	}
	after, err := os.Open(batchStatePath(root))
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer after.Close()
	afterInfo, err := after.Stat()
	if err != nil {
		t.Fatalf("stat state: %v", err)
	}
	if !os.SameFile(identity, afterInfo) {
		t.Fatal("state file identity changed: state.json was replaced")
	}
}

// batchStateIdentity captures state.json's identity for same-file checks.
// Identity comes from an open file handle so it is populated on every
// platform.
func batchStateIdentity(t *testing.T, root string) os.FileInfo {
	t.Helper()
	f, err := os.Open(batchStatePath(root))
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat state: %v", err)
	}
	return info
}

func TestBatchAddMarkSuccess(t *testing.T) {
	root := initBatchProject(t, "Ship v0.5.0")
	input := batchDocument(
		batchAdd("security"),
		batchAdd("security.csrf"),
		batchAdd("tests"),
		batchMark("security.csrf", "done"),
	)
	path := writeBatchFile(t, input)
	code, out, diagnostics := runBatchCommand(root, "", "--file", path)
	if code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("apply: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	state, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Cell{
		{ID: "security", Status: Expanded, Required: true},
		{ID: "security.csrf", Parent: "security", Status: Done, Required: true},
		{ID: "tests", Status: Open, Required: true},
	}
	if !reflect.DeepEqual(state.Cells, want) {
		t.Fatalf("cells=%v want=%v", state.Cells, want)
	}
	requireOnlyStateFile(t, root)
}

func TestBatchOptionalInheritance(t *testing.T) {
	root := initBatchProject(t, "goal")
	input := batchDocument(
		batchAddOptional("research"),
		batchAdd("research.papers"),
		batchAdd("tests"),
		batchAddOptional("tests.e2e"),
	)
	path := writeBatchFile(t, input)
	if code, out, diagnostics := runBatchCommand(root, "", "--file", path); code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("apply: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	state, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]Cell, len(state.Cells))
	for _, cell := range state.Cells {
		byID[cell.ID] = cell
	}
	if cell := byID["research"]; cell.Required {
		t.Fatalf("optional parent flag lost: %+v", cell)
	}
	if cell := byID["research.papers"]; cell.Required {
		t.Fatalf("child must inherit optional from parent: %+v", cell)
	}
	if cell := byID["tests"]; !cell.Required {
		t.Fatalf("required parent made optional: %+v", cell)
	}
	if cell := byID["tests.e2e"]; cell.Required {
		t.Fatalf("explicit optional child ignored: %+v", cell)
	}
}

func TestBatchOperationFailures(t *testing.T) {
	cases := []struct {
		name     string
		setup    []string // CLI commands run before the batch
		ops      []string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "child before parent",
			ops:      []string{batchAdd("security.csrf"), batchAdd("security")},
			wantCode: "E_INVALID_PARENT",
			wantMsg:  `batch operation 1 (add security.csrf)`,
		},
		{
			name:     "duplicate add within batch",
			ops:      []string{batchAdd("tests"), batchAdd("tests")},
			wantCode: "E_DUPLICATE_ID",
			wantMsg:  `batch operation 2 (add tests)`,
		},
		{
			name:     "mark unknown cell",
			ops:      []string{batchMark("nope", "done")},
			wantCode: "E_UNKNOWN_NODE",
			wantMsg:  `batch operation 1 (mark nope)`,
		},
		{
			name:     "mark expanded parent",
			setup:    []string{"add", "security", "add", "security.csrf"},
			ops:      []string{batchMark("security", "done")},
			wantCode: "E_STATE_INVALID",
			wantMsg:  `batch operation 1 (mark security)`,
		},
		{
			name:     "add under done parent",
			setup:    []string{"add", "security", "done", "security"},
			ops:      []string{batchAdd("security.csrf")},
			wantCode: "E_STATE_INVALID",
			wantMsg:  `batch operation 1 (add security.csrf)`,
		},
		{
			name:     "invalid id grammar",
			ops:      []string{batchAdd("Bad_ID")},
			wantCode: "E_USAGE",
			wantMsg:  `batch operation 1 (add "Bad_ID")`,
		},
		{
			name:     "three-level id",
			ops:      []string{batchAdd("a.b.c")},
			wantCode: "E_USAGE",
			wantMsg:  `batch operation 1 (add "a.b.c")`,
		},
		{
			name:     "mark with expanded status",
			ops:      []string{batchMark("x", "expanded")},
			wantCode: "E_BATCH_INVALID",
			wantMsg:  "invalid status",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := initBatchProject(t, "goal")
			for i := 0; i+1 < len(testCase.setup); i += 2 {
				if code, _, diagnostics := runTestCommand(root, testCase.setup[i], testCase.setup[i+1]); code != 0 {
					t.Fatalf("setup %v: %s", testCase.setup, diagnostics)
				}
			}
			before := readBatchState(t, root)
			mtime := batchStateModTime(t, root)
			identity := batchStateIdentity(t, root)
			path := writeBatchFile(t, batchDocument(testCase.ops...))
			code, out, diagnostics := runBatchCommand(root, "", "--file", path)
			if code != 2 || out != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out, diagnostics)
			}
			if !strings.Contains(diagnostics, testCase.wantCode) || !strings.Contains(diagnostics, testCase.wantMsg) {
				t.Fatalf("stderr=%q want code %q and %q", diagnostics, testCase.wantCode, testCase.wantMsg)
			}
			requireStateUnchanged(t, root, before, mtime, identity)
			requireOnlyStateFile(t, root)
		})
	}
}

func TestBatchRepeatedMarksUseFinalStatus(t *testing.T) {
	root := initBatchProject(t, "goal")
	if code, _, diagnostics := runTestCommand(root, "add", "tests"); code != 0 {
		t.Fatal(diagnostics)
	}
	input := batchDocument(
		batchMark("tests", "done"),
		batchMark("tests", "open"),
		batchMark("tests", "done"),
	)
	path := writeBatchFile(t, input)
	if code, out, diagnostics := runBatchCommand(root, "", "--file", path); code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("apply: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	state, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if state.Cells[0].Status != Done {
		t.Fatalf("final status=%v want done", state.Cells[0].Status)
	}
}

// TestBatchSemanticNoOp exercises inputs whose net effect returns every cell
// to its original state: changed must be false, no save may happen, and the
// exit code must be 0 (properties P5 and P2).
func TestBatchSemanticNoOp(t *testing.T) {
	root := initBatchProject(t, "goal")
	if code, _, diagnostics := runTestCommand(root, "add", "tests"); code != 0 {
		t.Fatal(diagnostics)
	}
	if code, _, diagnostics := runTestCommand(root, "done", "tests"); code != 0 {
		t.Fatal(diagnostics)
	}
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)

	// One operation that changes nothing: tests is already done.
	path := writeBatchFile(t, batchDocument(batchMark("tests", "done")))
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 0 || diagnostics != "" {
		t.Fatalf("single no-op dry-run: exit=%d stderr=%q", code, diagnostics)
	}
	want := "batch dry-run: 1 operation(s), 0 changed cell(s)\nrequired gaps: 0 -> 0\n"
	if out != want {
		t.Fatalf("no-op preview=%q want=%q", out, want)
	}

	// Canceling marks end where they started.
	path = writeBatchFile(t, batchDocument(
		batchMark("tests", "open"),
		batchMark("tests", "done"),
	))
	code, out, diagnostics = runBatchCommand(root, "", "--file", path, "--json")
	if code != 0 || diagnostics != "" {
		t.Fatalf("canceling no-op apply: exit=%d stderr=%q", code, diagnostics)
	}
	wantJSON := `{"batch_result_version":1,"dry_run":false,"changed":false,"operations":2,"required_gaps_before":0,"required_gaps_after":0,"changes":[]}` + "\n"
	if out != wantJSON {
		t.Fatalf("no-op JSON=%q want=%q", out, wantJSON)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
	requireOnlyStateFile(t, root)
}

func TestBatchRootAndChildrenLimits(t *testing.T) {
	root := initBatchProject(t, "goal")
	roots := make([]string, 0, 9)
	for _, id := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india"} {
		roots = append(roots, batchAdd(id))
	}
	path := writeBatchFile(t, batchDocument(roots[:8]...))
	if code, _, diagnostics := runBatchCommand(root, "", "--file", path); code != 0 {
		t.Fatalf("8 roots: %s", diagnostics)
	}
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	path = writeBatchFile(t, batchDocument(roots[8]))
	if code, out, diagnostics := runBatchCommand(root, "", "--file", path); code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") {
		t.Fatalf("9th root: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)

	children := make([]string, 0, 9)
	for i := range 9 {
		children = append(children, batchAdd("alpha.c"+strings.Repeat("x", i))) // alpha.c, alpha.cx, ...
	}
	path = writeBatchFile(t, batchDocument(children[:8]...))
	if code, _, diagnostics := runBatchCommand(root, "", "--file", path); code != 0 {
		t.Fatalf("8 children: %s", diagnostics)
	}
	before = readBatchState(t, root)
	mtime = batchStateModTime(t, root)
	identity = batchStateIdentity(t, root)
	path = writeBatchFile(t, batchDocument(children[8]))
	if code, out, diagnostics := runBatchCommand(root, "", "--file", path); code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") {
		t.Fatalf("9th child: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
}

func fillBatchIDs() []string {
	ops := make([]string, 0, 72)
	for _, root := range []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8"} {
		ops = append(ops, batchAdd(root))
	}
	for _, root := range []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8"} {
		for _, child := range []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8"} {
			ops = append(ops, batchAdd(root+"."+child))
		}
	}
	return ops
}

// TestBatchCellCapacity72 fills a state to exactly 72 cells in one batch,
// then proves the 73rd cell is rejected and nothing is saved.
func TestBatchCellCapacity72(t *testing.T) {
	root := initBatchProject(t, "goal")
	fill := fillBatchIDs()
	path := writeBatchFile(t, batchDocument(fill...))
	if code, _, diagnostics := runBatchCommand(root, "", "--file", path); code != 0 {
		t.Fatalf("fill to 72: %s", diagnostics)
	}
	state, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Cells) != 72 {
		t.Fatalf("cells=%d want 72", len(state.Cells))
	}
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	path = writeBatchFile(t, batchDocument(batchAdd("r1.extra")))
	if code, out, diagnostics := runBatchCommand(root, "", "--file", path); code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") {
		t.Fatalf("73rd cell: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)

	// A single batch that would push past 72 cells fails atomically:
	// the 72 valid adds are not saved.
	empty := initBatchProject(t, "goal")
	path = writeBatchFile(t, batchDocument(append(slices.Clone(fill), batchAdd("r1.extra"))...))
	if code, out, diagnostics := runBatchCommand(empty, "", "--file", path); code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") {
		t.Fatalf("overflow in one batch: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	state, err = Load(empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Cells) != 0 {
		t.Fatalf("partial application left %d cells", len(state.Cells))
	}
}

// TestBatchOperationCountLimits proves 128 operations are accepted when the
// final state is valid (adds plus repeated marks) and 129 are rejected by the
// input limits before any state is touched.
func TestBatchOperationCountLimits(t *testing.T) {
	root := initBatchProject(t, "goal")
	ops := make([]string, 0, 129)
	ops = append(ops, batchAdd("alpha"), batchAdd("bravo"))
	for range 126 {
		ops = append(ops, batchMark("alpha", "done"), batchMark("alpha", "open"))
	}
	ops = ops[:128]
	path := writeBatchFile(t, batchDocument(ops...))
	if code, _, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run"); code != 0 {
		t.Fatalf("128 operations: %s", diagnostics)
	}
	ops = append(ops, batchMark("bravo", "done"))
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	path = writeBatchFile(t, batchDocument(ops...))
	if code, out, diagnostics := runBatchCommand(root, "", "--file", path); code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") {
		t.Fatalf("129 operations: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
}

// TestBatchAtomicityOnFailure drives the T03 matrix: failures at the first,
// middle, and last operation, malformed JSON, an invalid status, a duplicate
// ID, and capacity overflow all leave state.json byte- and mtime-identical
// with no temporary files.
func TestBatchAtomicityOnFailure(t *testing.T) {
	cases := []struct {
		name  string
		setup []string
		input string
	}{
		{
			name:  "first operation fails",
			input: batchDocument(batchMark("missing", "done"), batchAdd("tests")),
		},
		{
			name:  "middle operation fails",
			input: batchDocument(batchAdd("tests"), batchMark("missing", "done"), batchAdd("alpha")),
		},
		{
			name:  "last operation fails",
			input: batchDocument(batchAdd("tests"), batchAdd("alpha"), batchMark("missing", "done")),
		},
		{
			name:  "malformed json",
			input: `{"batch_version":1,"operations":[{"op":"add"`,
		},
		{
			name:  "invalid status",
			input: batchDocument(batchAdd("tests"), batchMark("tests", "finished")),
		},
		{
			name:  "duplicate id against existing state",
			setup: []string{"add", "tests"},
			input: batchDocument(batchAdd("alpha"), batchAdd("tests")),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := initBatchProject(t, "goal")
			for i := 0; i+1 < len(testCase.setup); i += 2 {
				if code, _, diagnostics := runTestCommand(root, testCase.setup[i], testCase.setup[i+1]); code != 0 {
					t.Fatalf("setup: %s", diagnostics)
				}
			}
			before := readBatchState(t, root)
			mtime := batchStateModTime(t, root)
			identity := batchStateIdentity(t, root)
			for _, dryRun := range []bool{false, true} {
				path := writeBatchFile(t, testCase.input)
				args := []string{"--file", path}
				if dryRun {
					args = append(args, "--dry-run")
				}
				code, out, diagnostics := runBatchCommand(root, "", args...)
				if code != 2 || out != "" || diagnostics == "" {
					t.Fatalf("dryRun=%v: exit=%d stdout=%q stderr=%q", dryRun, code, out, diagnostics)
				}
				requireStateUnchanged(t, root, before, mtime, identity)
				requireOnlyStateFile(t, root)
			}
		})
	}
}

// TestBatchSaveFailure follows the existing I/O failure test convention:
// make the state directory unwritable so the temp file cannot be created,
// prove the failure surfaces as E_IO, and prove the state is untouched.
//
// Exception to the no-Windows-skip rule, recorded for human review: Go's
// os.Chmod cannot make a directory unwritable on Windows (the read-only bit
// applies to files only), so this directory-level injection is not portable.
// A Windows-specific alternative may exist — making state.json read-only
// could make Save's rename fail because the Go runtime does not pass
// FILE_RENAME_IGNORE_READONLY_ATTRIBUTE — but it is unverified on Windows
// (audit round 3, MINOR-4) and remains a human decision. The
// platform-portable wiring contract (exit 2, empty stdout, E_IO, untouched
// state, no success JSON) is covered on every platform by
// TestBatchSaveErrorPropagation; this test adds the real filesystem failure
// on Unix. Unverified scope: a genuine OS-level write failure inside Save on
// Windows.
func TestBatchSaveFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix file permissions and a non-root user")
	}
	root := initBatchProject(t, "goal")
	dir := filepath.Join(root, ".mandala")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, perm); err != nil {
			t.Error(err)
		}
	})
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	path := writeBatchFile(t, batchDocument(batchAdd("tests")))
	code, out, diagnostics := runBatchCommand(root, "", "--file", path)
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_IO") {
		t.Fatalf("save failure: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
}

// TestApplyBatchOperationsMatchesSequentialMutators is property P1: a fully
// successful batch produces exactly the state the existing mutators produce
// when driven operation by operation. It also asserts P2 at the unit level:
// the caller's state is never mutated, including its backing array.
func TestApplyBatchOperationsMatchesSequentialMutators(t *testing.T) {
	sequences := [][]BatchOperation{
		{
			{Op: "add", ID: "security"},
			{Op: "add", ID: "security.csrf"},
			{Op: "mark", ID: "security.csrf", Status: Done},
		},
		{
			{Op: "add", ID: "research", Optional: true},
			{Op: "add", ID: "research.papers"},
			{Op: "add", ID: "tests"},
			{Op: "mark", ID: "tests", Status: Done},
			{Op: "mark", ID: "tests", Status: Open},
		},
		{
			{Op: "add", ID: "a"},
			{Op: "add", ID: "a.b"},
			{Op: "add", ID: "c"},
			{Op: "mark", ID: "a.b", Status: NotApplicable},
			{Op: "add", ID: "a.d"},
		},
		{
			// Marks against cells that already exist in the start state
			// exercise in-place status writes, not just appends.
			{Op: "mark", ID: "alpha.one", Status: Open},
			{Op: "mark", ID: "beta", Status: Done},
		},
	}
	for _, ops := range sequences {
		original, err := NewState("goal")
		if err != nil {
			t.Fatal(err)
		}
		// A non-empty start state with spare capacity exposes in-place edits
		// to existing cells and backing-array aliasing between the original
		// and the working copy, including slots beyond len.
		original.Cells = make([]Cell, 0, 16)
		if err := original.Add("alpha", false); err != nil {
			t.Fatal(err)
		}
		if err := original.Add("alpha.one", false); err != nil {
			t.Fatal(err)
		}
		if err := original.Mark("alpha.one", Done); err != nil {
			t.Fatal(err)
		}
		if err := original.Add("beta", true); err != nil {
			t.Fatal(err)
		}
		backing := append([]Cell(nil), original.Cells[:cap(original.Cells)]...)
		originalCopy := cloneState(original)
		got, err := applyBatchOperations(original, ops)
		if err != nil {
			t.Fatalf("apply %v: %v", ops, err)
		}
		want := cloneState(original)
		for _, op := range ops {
			if op.Op == "add" {
				err = want.Add(op.ID, op.Optional)
			} else {
				err = want.Mark(op.ID, op.Status)
			}
			if err != nil {
				t.Fatalf("sequential %v: %v", op, err)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("batch result=%+v want=%+v", got, want)
		}
		if !reflect.DeepEqual(original, originalCopy) {
			t.Fatalf("original state mutated: %+v want %+v", original, originalCopy)
		}
		if !reflect.DeepEqual(original.Cells[:cap(original.Cells)], backing) {
			t.Fatalf("original backing array mutated: %+v want %+v", original.Cells[:cap(original.Cells)], backing)
		}
	}
}

// TestApplyBatchOperationsFinalValidation proves the final Validate call is
// defense in depth: a start state that is already over capacity (only
// constructible in-process) still fails validation before Save could run.
func TestApplyBatchOperationsFinalValidation(t *testing.T) {
	cells := make([]Cell, 0, 16)
	for i := range 9 {
		cells = append(cells, Cell{ID: "root" + string(rune('a'+i)), Status: Open, Required: true})
	}
	original := State{SchemaVersion: 1, Goal: "goal", Cells: cells}
	if err := original.Validate(); err == nil {
		t.Fatal("fixture must be invalid")
	}
	if _, err := applyBatchOperations(original, []BatchOperation{{Op: "mark", ID: "roota", Status: Done}}); err == nil {
		t.Fatal("final validation must reject an invalid end state")
	}
}

func TestCloneStateIsolatesBackingArray(t *testing.T) {
	original := State{
		SchemaVersion: 1,
		Goal:          "goal",
		Cells:         make([]Cell, 1, 8),
	}
	original.Cells[0] = Cell{ID: "alpha", Status: Open, Required: true}
	clone := cloneState(original)
	clone.Cells[0].Status = Done
	clone.Cells = append(clone.Cells, Cell{ID: "beta", Status: Open, Required: true})
	if original.Cells[0].Status != Open || len(original.Cells) != 1 {
		t.Fatalf("original mutated through clone: %+v", original.Cells)
	}
}

// TestBatchDisplayIDTruncation pins displayBatchID's 64-byte cap: an invalid
// ID longer than 64 bytes appears in the operation context quoted and
// truncated, so hostile IDs cannot flood diagnostics through the wrapper.
func TestBatchDisplayIDTruncation(t *testing.T) {
	root := initBatchProject(t, "Truncated diagnostics")
	longID := strings.Repeat("a", 100)
	path := writeBatchFile(t, batchDocument(batchAdd(longID)))
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	code, out, diagnostics := runBatchCommand(root, "", "--file", path)
	if code != 2 || out != "" {
		t.Fatalf("long invalid id: exit=%d stdout=%q", code, out)
	}
	if !strings.Contains(diagnostics, "E_USAGE") {
		t.Fatalf("long invalid id: stderr=%q", diagnostics)
	}
	want := "batch operation 1 (add \"" + strings.Repeat("a", 64) + "\")"
	if !strings.Contains(diagnostics, want) {
		t.Fatalf("stderr missing truncated id context:\n got=%q\nwant substring=%q", diagnostics, want)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
}

func TestBatchBoundedIDDiagnostics(t *testing.T) {
	root := initBatchProject(t, "Bounded diagnostics")
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	cases := []struct {
		name   string
		id     string
		prefix string
	}{
		{"huge ASCII", strings.Repeat("a", 65000), strings.Repeat("a", 64)},
		{"quotes and slashes", strings.Repeat("\"\\", 1000), strings.Repeat("\"\\", 32)},
		{"Unicode", strings.Repeat("界", 1000), strings.Repeat("界", 21)},
		{"Unicode after 63 bytes", strings.Repeat("a", 63) + "界tail", strings.Repeat("a", 63)},
		{"Unicode after 62 bytes", strings.Repeat("a", 62) + "界tail", strings.Repeat("a", 62)},
		{"Unicode ends at 64 bytes", strings.Repeat("a", 61) + "界tail", strings.Repeat("a", 61) + "界"},
		{"two-byte Unicode boundary", strings.Repeat("a", 63) + "étail", strings.Repeat("a", 63)},
		{"four-byte Unicode boundary", strings.Repeat("a", 62) + "😀tail", strings.Repeat("a", 62)},
		{"controls", "x\n\t\x1b" + strings.Repeat("a", 1000), "x\n\t\x1b" + strings.Repeat("a", 60)},
		{"ASCII boundary", strings.Repeat("a", 65), strings.Repeat("a", 64)},
	}
	for _, tc := range cases {
		for _, op := range []string{"add", "mark"} {
			t.Run(tc.name+"/"+op, func(t *testing.T) {
				operation := BatchOperation{Op: op, ID: tc.id, Status: Done}
				wantCode, cause := "E_USAGE", "invalid cell ID "
				if op == "mark" {
					wantCode, cause = "E_UNKNOWN_NODE", "unknown cell "
				}
				_, err := applyBatchOperations(State{SchemaVersion: 1, Goal: "goal", Cells: []Cell{}}, []BatchOperation{operation})
				var p *Problem
				if !errors.As(err, &p) || p.Code != wantCode {
					t.Fatalf("Problem classification: %v", err)
				}
				wire := map[string]any{"op": op, "id": tc.id}
				if op == "mark" {
					wire["status"] = "done"
				}
				input, err := json.Marshal(map[string]any{"batch_version": 1, "operations": []any{wire}})
				if err != nil {
					t.Fatal(err)
				}
				code, out, diagnostics := runBatchCommand(root, string(input), "--file", "-")
				want := "mandala: " + wantCode + ": batch operation 1 (" + op + " " + strconv.Quote(tc.prefix) + "): " + cause + strconv.Quote(tc.prefix) + "\n"
				if code != 2 || out != "" || diagnostics != want {
					t.Fatalf("exit=%d stdout=%q diagnostic bytes=%d, want %q", code, out, len(diagnostics), want)
				}
				if len(diagnostics) > 700 || !utf8.ValidString(diagnostics) || strings.Count(diagnostics, "\n") != 1 || strings.ContainsRune(diagnostics, '\x1b') {
					t.Fatalf("unsafe diagnostic: %q", diagnostics)
				}
				_, _, repeated := runBatchCommand(root, string(input), "--file", "-")
				if repeated != diagnostics {
					t.Fatal("diagnostic is not deterministic")
				}
				requireStateUnchanged(t, root, before, mtime, identity)
				requireOnlyStateFile(t, root)
			})
		}
	}
}

func TestBatchShortIDDiagnosticsAndStatusPrecedence(t *testing.T) {
	cases := []struct {
		op   BatchOperation
		want string
		code string
	}{
		{BatchOperation{Op: "add", ID: "Bad_ID"}, `batch operation 1 (add "Bad_ID"): invalid cell ID "Bad_ID"`, "E_USAGE"},
		{BatchOperation{Op: "add", ID: "a\"\\猫"}, `batch operation 1 (add "a\"\\猫"): invalid cell ID "a\"\\猫"`, "E_USAGE"},
		{BatchOperation{Op: "mark", ID: "missing", Status: Done}, `batch operation 1 (mark missing): unknown cell "missing"`, "E_UNKNOWN_NODE"},
		{BatchOperation{Op: "mark", ID: "bad\nid", Status: Done}, `batch operation 1 (mark "bad\nid"): unknown cell "bad\nid"`, "E_UNKNOWN_NODE"},
		{BatchOperation{Op: "mark", ID: strings.Repeat("a", 1000), Status: "invalid"}, "batch operation 1 (mark " + strconv.Quote(strings.Repeat("a", 64)) + "): status must be open, done, or na", "E_USAGE"},
	}
	for _, tc := range cases {
		_, err := applyBatchOperations(State{SchemaVersion: 1, Goal: "goal", Cells: []Cell{}}, []BatchOperation{tc.op})
		var p *Problem
		if !errors.As(err, &p) || p.Code != tc.code || err.Error() != tc.want {
			t.Fatalf("diagnostic: got %v want %q (%s)", err, tc.want, tc.code)
		}
	}
}
