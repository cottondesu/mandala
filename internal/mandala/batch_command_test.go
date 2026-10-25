package mandala

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specPreviewFixture builds the exact scenario from the specification's
// preview example: an open required root "security", then a batch that adds
// security.csrf and tests.
func specPreviewFixture(t *testing.T) (root, input string) {
	t.Helper()
	root = initBatchProject(t, "Ship it")
	if code, _, diagnostics := runTestCommand(root, "add", "security"); code != 0 {
		t.Fatal(diagnostics)
	}
	input = batchDocument(batchAdd("security.csrf"), batchAdd("tests"))
	return root, input
}

func TestBatchPreviewJSONGolden(t *testing.T) {
	root, input := specPreviewFixture(t)
	path := writeBatchFile(t, input)
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run", "--json")
	if code != 0 || diagnostics != "" {
		t.Fatalf("exit=%d stderr=%q", code, diagnostics)
	}
	want := `{"batch_result_version":1,"dry_run":true,"changed":true,"operations":2,"required_gaps_before":1,"required_gaps_after":2,"changes":[` +
		`{"id":"security","before":{"parent":"","status":"open","required":true},"after":{"parent":"","status":"expanded","required":true}},` +
		`{"id":"security.csrf","before":null,"after":{"parent":"security","status":"open","required":true}},` +
		`{"id":"tests","before":null,"after":{"parent":"","status":"open","required":true}}` +
		`]}` + "\n"
	if out != want {
		t.Fatalf("JSON preview:\n got=%q\nwant=%q", out, want)
	}
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "}\n") {
		t.Fatalf("newline contract violated: %q", out)
	}
	if strings.Contains(out, ": ") || strings.Contains(out, ", ") {
		t.Fatalf("preview is not compact: %q", out)
	}
}

func TestBatchPreviewTextGolden(t *testing.T) {
	root, input := specPreviewFixture(t)
	path := writeBatchFile(t, input)
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 0 || diagnostics != "" {
		t.Fatalf("exit=%d stderr=%q", code, diagnostics)
	}
	want := "batch dry-run: 2 operation(s), 3 changed cell(s)\n" +
		"required gaps: 1 -> 2\n" +
		"~ security [open, required] -> [expanded, required]\n" +
		"+ security.csrf [open, required]\n" +
		"+ tests [open, required]\n"
	if out != want {
		t.Fatalf("text preview:\n got=%q\nwant=%q", out, want)
	}
}

func TestBatchPreviewMarkChange(t *testing.T) {
	root := initBatchProject(t, "goal")
	if code, _, diagnostics := runTestCommand(root, "add", "tests"); code != 0 {
		t.Fatal(diagnostics)
	}
	path := writeBatchFile(t, batchDocument(batchMark("tests", "done")))
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 0 || diagnostics != "" {
		t.Fatalf("exit=%d stderr=%q", code, diagnostics)
	}
	want := "batch dry-run: 1 operation(s), 1 changed cell(s)\n" +
		"required gaps: 1 -> 0\n" +
		"~ tests [open, required] -> [done, required]\n"
	if out != want {
		t.Fatalf("mark preview:\n got=%q\nwant=%q", out, want)
	}
}

func TestBatchPreviewOptionalLabels(t *testing.T) {
	root := initBatchProject(t, "goal")
	input := batchDocument(batchAddOptional("research"), batchAdd("research.papers"))
	path := writeBatchFile(t, input)
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 0 || diagnostics != "" {
		t.Fatalf("exit=%d stderr=%q", code, diagnostics)
	}
	want := "batch dry-run: 2 operation(s), 2 changed cell(s)\n" +
		"required gaps: 0 -> 0\n" +
		"+ research [expanded, optional]\n" +
		"+ research.papers [open, optional]\n"
	if out != want {
		t.Fatalf("optional preview:\n got=%q\nwant=%q", out, want)
	}
}

// TestBatchPreviewSortedByID proves the change list is sorted by ID in Go
// string order regardless of the input operation order.
func TestBatchPreviewSortedByID(t *testing.T) {
	root := initBatchProject(t, "goal")
	input := batchDocument(batchAdd("zeta"), batchAdd("mid"), batchAdd("mid.beta"), batchAdd("alpha"))
	path := writeBatchFile(t, input)
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 0 || diagnostics != "" {
		t.Fatalf("exit=%d stderr=%q", code, diagnostics)
	}
	got := make([]string, 0, 4)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.HasPrefix(line, "+ ") {
			got = append(got, strings.Fields(line)[1])
		}
	}
	want := []string{"alpha", "mid", "mid.beta", "zeta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("change order=%v want=%v in output:\n%s", got, want, out)
	}
}

// TestBatchPreviewInputOrderIndependence is property P4's input-facing half:
// two operation orders with the same final state produce identical output.
func TestBatchPreviewInputOrderIndependence(t *testing.T) {
	root := initBatchProject(t, "goal")
	inputA := batchDocument(batchAdd("alpha"), batchMark("alpha", "done"), batchAdd("beta"))
	inputB := batchDocument(batchAdd("beta"), batchAdd("alpha"), batchMark("alpha", "done"))
	pathA := writeBatchFile(t, inputA)
	pathB := writeBatchFile(t, inputB)
	codeA, outA, diagnosticsA := runBatchCommand(root, "", "--file", pathA, "--dry-run", "--json")
	codeB, outB, diagnosticsB := runBatchCommand(root, "", "--file", pathB, "--dry-run", "--json")
	if codeA != 0 || codeB != 0 || diagnosticsA != "" || diagnosticsB != "" {
		t.Fatalf("A: exit=%d stderr=%q; B: exit=%d stderr=%q", codeA, diagnosticsA, codeB, diagnosticsB)
	}
	if outA != outB {
		t.Fatalf("operation order changed preview:\nA=%q\nB=%q", outA, outB)
	}
}

// TestBatchPreviewStorageOrderIndependence is property P4's storage-facing
// half: rewriting state.json with its cells in a different order must not
// change the preview.
func TestBatchPreviewStorageOrderIndependence(t *testing.T) {
	root := initBatchProject(t, "goal")
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if code, _, diagnostics := runTestCommand(root, "add", id); code != 0 {
			t.Fatal(diagnostics)
		}
	}
	input := batchDocument(batchAdd("alpha.a"), batchMark("beta", "done"))
	path := writeBatchFile(t, input)
	_, reference, _ := runBatchCommand(root, "", "--file", path, "--dry-run", "--json")

	state, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	reversed := cloneState(state)
	for i, j := 0, len(reversed.Cells)-1; i < j; i, j = i+1, j-1 {
		reversed.Cells[i], reversed.Cells[j] = reversed.Cells[j], reversed.Cells[i]
	}
	data, err := json.Marshal(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(batchStatePath(root), data, 0600); err != nil {
		t.Fatal(err)
	}
	code, shuffled, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run", "--json")
	if code != 0 || diagnostics != "" {
		t.Fatalf("exit=%d stderr=%q", code, diagnostics)
	}
	if shuffled != reference {
		t.Fatalf("storage order changed preview:\n got=%q\nwant=%q", shuffled, reference)
	}
}

// TestBatchDryRunRepeatable is property P6: repeated dry-runs of the same
// input are byte-identical.
func TestBatchDryRunRepeatable(t *testing.T) {
	root, input := specPreviewFixture(t)
	path := writeBatchFile(t, input)
	codeA, outA, _ := runBatchCommand(root, "", "--file", path, "--dry-run", "--json")
	codeB, outB, _ := runBatchCommand(root, "", "--file", path, "--dry-run", "--json")
	if codeA != 0 || codeB != 0 || outA != outB {
		t.Fatalf("repeat: A(exit=%d)=%q B(exit=%d)=%q", codeA, outA, codeB, outB)
	}
}

// TestBatchDryRunImmutability is the T04 matrix: a successful dry-run never
// writes state.json, never creates temp files, and never touches Git-local
// exclude or .gitignore.
func TestBatchDryRunImmutability(t *testing.T) {
	requireGit(t)
	repo := initFixtureRepository(t, map[string]string{"README.md": "fixture\n"})
	if code, _, diagnostics := runTestCommand(repo, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	if code, _, diagnostics := runTestCommand(repo, "add", "security"); code != 0 {
		t.Fatal(diagnostics)
	}
	stateBytes := readBatchState(t, repo)
	mtime := batchStateModTime(t, repo)
	identity := batchStateIdentity(t, repo)
	exclude := fixtureExcludePath(t, repo)
	excludeBefore, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	gitStatusBefore := runFixtureGit(t, repo, nil, "status", "--porcelain")
	input := batchDocument(batchAdd("security.csrf"), batchAdd("tests"))
	path := writeBatchFile(t, input)
	code, out, diagnostics := runBatchCommand(repo, "", "--file", path, "--dry-run")
	if code != 0 || diagnostics != "" || out == "" {
		t.Fatalf("dry-run: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, repo, stateBytes, mtime, identity)
	requireOnlyStateFile(t, repo)
	excludeAfter, err := os.ReadFile(exclude)
	if err != nil || string(excludeAfter) != string(excludeBefore) {
		t.Fatalf("git exclude changed: %q -> %q", excludeBefore, excludeAfter)
	}
	if gitStatusAfter := runFixtureGit(t, repo, nil, "status", "--porcelain"); gitStatusAfter != gitStatusBefore {
		t.Fatalf("git status changed: %q -> %q", gitStatusBefore, gitStatusAfter)
	}
	if _, err := os.Stat(filepath.Join(repo, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf(".gitignore must not be created: %v", err)
	}
}

// TestBatchDryRunFailureLeavesStateUntouched complements the atomicity test:
// a dry-run over invalid operations exits 2 with empty stdout and writes
// nothing.
func TestBatchDryRunFailureLeavesStateUntouched(t *testing.T) {
	root := initBatchProject(t, "goal")
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	path := writeBatchFile(t, batchDocument(batchMark("missing", "done")))
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_UNKNOWN_NODE") {
		t.Fatalf("dry-run failure: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
	requireOnlyStateFile(t, root)
}

// TestBatchDryRunMatchesApply is property P3: dry-run and apply over the same
// start state and input produce the same final diff.
func TestBatchDryRunMatchesApply(t *testing.T) {
	root, input := specPreviewFixture(t)
	path := writeBatchFile(t, input)
	_, dryRunJSON, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run", "--json")
	if diagnostics != "" {
		t.Fatal(diagnostics)
	}
	code, applyJSON, diagnostics := runBatchCommand(root, "", "--file", path, "--json")
	if code != 0 || diagnostics != "" {
		t.Fatalf("apply: exit=%d stderr=%q", code, diagnostics)
	}
	var dryRunResult, applyResult map[string]any
	if err := json.Unmarshal([]byte(dryRunJSON), &dryRunResult); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(applyJSON), &applyResult); err != nil {
		t.Fatal(err)
	}
	if dryRunResult["dry_run"] != true || applyResult["dry_run"] != false {
		t.Fatalf("dry_run flags: %v %v", dryRunResult["dry_run"], applyResult["dry_run"])
	}
	if toJSON(t, dryRunResult["changes"]) != toJSON(t, applyResult["changes"]) {
		t.Fatalf("changes differ:\ndry-run=%s\napply=%s", dryRunJSON, applyJSON)
	}
	if dryRunResult["required_gaps_after"] != applyResult["required_gaps_after"] {
		t.Fatalf("gap counts differ: %v vs %v", dryRunJSON, applyJSON)
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestBatchTruncatedStderrDeterministic pins the full stderr line for
// truncated input at the CLI level, byte-exact on every supported
// toolchain (audit round 3, MINOR-1: the wording must never regress to
// "invalid JSON: EOF" on Go 1.26 or "empty batch input" anywhere).
func TestBatchTruncatedStderrDeterministic(t *testing.T) {
	root := initBatchProject(t, "goal")
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	inputs := []string{
		`{"batch_version":1,"operations":`,
		`{"batch_version":1,"operations":[{"op":"add"`,
		`{"batch_version":1,"operations":[{"op":"add","id":"aa`,
	}
	for _, input := range inputs {
		code, out, diagnostics := runBatchCommand(root, input, "--file", "-", "--dry-run")
		if code != 2 {
			t.Fatalf("truncated input %q: exit = %d, want 2 (stderr %q)", input, code, diagnostics)
		}
		if out != "" {
			t.Fatalf("truncated input %q: stdout = %q, want empty", input, out)
		}
		want := "mandala: E_BATCH_INVALID: invalid JSON: unexpected end of JSON input\n"
		if diagnostics != want {
			t.Fatalf("truncated input %q: stderr = %q, want %q", input, diagnostics, want)
		}
	}
	requireStateUnchanged(t, root, before, mtime, identity)
	requireOnlyStateFile(t, root)
}

func TestBatchCLIContract(t *testing.T) {
	root := initBatchProject(t, "goal")
	valid := writeBatchFile(t, batchDocument(batchAdd("tests")))
	cases := []struct {
		name    string
		args    []string
		stdin   string
		wantMsg string
	}{
		{name: "missing --file", args: nil, wantMsg: "batch requires --file"},
		{name: "empty --file value", args: []string{"--file", ""}, wantMsg: "batch requires --file"},
		{name: "duplicate --file", args: []string{"--file", valid, "--file", valid}, wantMsg: "--file may be specified at most once"},
		{name: "unknown flag", args: []string{"--file", valid, "--verbose"}, wantMsg: "E_USAGE"},
		{name: "positional argument", args: []string{"--file", valid, "extra"}, wantMsg: "batch expects 0 argument"},
		{name: "missing file", args: []string{"--file", filepath.Join(t.TempDir(), "nope.json")}, wantMsg: "E_IO"},
		{name: "directory input", args: []string{"--file", root}, wantMsg: "not a regular file"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			code, out, diagnostics := runBatchCommand(root, "", testCase.args...)
			if code != 2 || out != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out, diagnostics)
			}
			if !strings.Contains(diagnostics, testCase.wantMsg) {
				t.Fatalf("stderr=%q want %q", diagnostics, testCase.wantMsg)
			}
		})
	}

	code, out, diagnostics := runBatchCommand(root, "", "--help")
	want := "Usage: mandala [--project DIR] batch --file <file|-> [--dry-run] [--json]\n"
	if code != 0 || out != want || diagnostics != "" {
		t.Fatalf("help: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestBatchFileFlagValuesAndDuplicates(t *testing.T) {
	root := initBatchProject(t, "File flags")
	input := []byte(batchDocument(batchAdd("alpha")))
	inputInfo := make(map[string]os.FileInfo)
	for _, name := range []string{"-file", "--file", "-file=ops", "--file=ops", "ops.json", "other.json"} {
		if err := os.WriteFile(filepath.Join(root, name), input, 0600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		inputInfo[name] = info
	}
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"dash value", []string{"--file", "-file"}, 0},
		{"equals value", []string{"--file=-file"}, 0},
		{"relative value", []string{"--file", "./-file"}, 0},
		{"double dash value", []string{"--file", "--file"}, 0},
		{"single dash equals value", []string{"--file", "-file=ops"}, 0},
		{"double dash equals value", []string{"--file", "--file=ops"}, 0},
		{"duplicate separated", []string{"--file", "ops.json", "--file", "other.json"}, 2},
		{"duplicate equals", []string{"--file=ops.json", "--file=other.json"}, 2},
		{"duplicate mixed spelling", []string{"-file", "ops.json", "--file", "other.json"}, 2},
		{"missing", nil, 2},
		{"empty", []string{"--file", ""}, 2},
		{"empty equals", []string{"--file="}, 2},
		{"unknown flag", []string{"--file", "ops.json", "--unknown"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, diagnostics := runBatchCommand(root, "", append(tc.args, "--dry-run", "--json")...)
			if code != tc.want {
				t.Fatalf("exit=%d want=%d stdout=%q stderr=%q", code, tc.want, out, diagnostics)
			}
			if tc.want == 0 {
				if diagnostics != "" || !json.Valid([]byte(out)) || !strings.Contains(out, `"id":"alpha"`) {
					t.Fatalf("valid file value: stdout=%q stderr=%q", out, diagnostics)
				}
			} else if out != "" || !strings.Contains(diagnostics, "E_USAGE") {
				t.Fatalf("usage error: stdout=%q stderr=%q", out, diagnostics)
			}
			requireStateUnchanged(t, root, before, mtime, identity)
			requireOnlyStateFile(t, root)
			for _, name := range []string{"-file", "--file", "-file=ops", "--file=ops", "ops.json", "other.json"} {
				got, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || !bytes.Equal(got, input) {
					t.Fatalf("input %q changed: %v", name, err)
				}
				info, err := os.Stat(filepath.Join(root, name))
				if err != nil || !info.ModTime().Equal(inputInfo[name].ModTime()) {
					t.Fatalf("input %q mtime changed: %v", name, err)
				}
			}
		})
	}
}

type partialBatchOutput struct {
	data bytes.Buffer
}

func (w *partialBatchOutput) Write(p []byte) (int, error) {
	n, _ := w.data.Write(p[:min(len(p), 32)])
	return n, io.ErrClosedPipe
}

func TestBatchPartialStdoutFailureKeepsSavedState(t *testing.T) {
	root := initBatchProject(t, "Output failure")
	var out partialBatchOutput
	var diagnostics bytes.Buffer
	input := batchDocument(batchAdd("alpha"))
	code := run([]string{"batch", "--file", "-", "--json"}, root, strings.NewReader(input), &out, &diagnostics, "test")
	if code != 2 || !strings.Contains(diagnostics.String(), "E_IO: write stdout:") {
		t.Fatalf("exit=%d stderr=%q", code, diagnostics.String())
	}
	if out.data.Len() != 32 || json.Valid(out.data.Bytes()) {
		t.Fatalf("expected partial JSON: %q", out.data.String())
	}
	s, err := Load(root)
	if err != nil || len(s.Cells) != 1 || s.Cells[0].ID != "alpha" {
		t.Fatalf("saved state was lost: state=%+v err=%v", s, err)
	}
	requireOnlyStateFile(t, root)
}

// TestBatchStdinMatchesFile proves --file - and a file holding the same
// document behave identically.
func TestBatchStdinMatchesFile(t *testing.T) {
	input := batchDocument(batchAdd("alpha"), batchAdd("alpha.beta"), batchMark("alpha.beta", "done"))
	fileRoot := initBatchProject(t, "goal")
	path := writeBatchFile(t, input)
	codeFile, outFile, diagnosticsFile := runBatchCommand(fileRoot, "", "--file", path, "--dry-run", "--json")
	stdinRoot := initBatchProject(t, "goal")
	codeStdin, outStdin, diagnosticsStdin := runBatchCommand(stdinRoot, input, "--file", "-", "--dry-run", "--json")
	if codeFile != 0 || codeStdin != 0 || diagnosticsFile != "" || diagnosticsStdin != "" {
		t.Fatalf("file: exit=%d stderr=%q; stdin: exit=%d stderr=%q", codeFile, diagnosticsFile, codeStdin, diagnosticsStdin)
	}
	if outFile != outStdin {
		t.Fatalf("stdin differs from file:\nfile=%q\nstdin=%q", outFile, outStdin)
	}
}

// TestBatchInputSizeBoundary proves the 65,536-byte boundary is measured by
// bytes read and that trailing whitespace inside the limit is allowed.
func TestBatchInputSizeBoundary(t *testing.T) {
	root := initBatchProject(t, "goal")
	document := batchDocument(batchAdd("tests"))
	path := filepath.Join(t.TempDir(), "batch.json")
	exact := document + strings.Repeat(" ", maxBatchInputBytes-len(document))
	if len(exact) != maxBatchInputBytes {
		t.Fatalf("fixture length=%d want %d", len(exact), maxBatchInputBytes)
	}
	if err := os.WriteFile(path, []byte(exact), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 0 || diagnostics != "" {
		t.Fatalf("exact limit: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	oversize := document + strings.Repeat(" ", maxBatchInputBytes-len(document)+1)
	if err := os.WriteFile(path, []byte(oversize), 0600); err != nil {
		t.Fatal(err)
	}
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	code, out, diagnostics = runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") {
		t.Fatalf("oversize: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
}

// TestBatchNeverExitsOne proves batch keeps exit 0 on success even when
// required gaps remain, unlike status/gaps which exit 1.
func TestBatchNeverExitsOne(t *testing.T) {
	root := initBatchProject(t, "goal")
	path := writeBatchFile(t, batchDocument(batchAdd("alpha"), batchAdd("beta")))
	if code, out, diagnostics := runBatchCommand(root, "", "--file", path); code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("apply with gaps: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	if code, _, _ := runTestCommand(root, "gaps"); code != 1 {
		t.Fatalf("gaps should still exit 1, got %d", code)
	}
}

// TestBatchProjectOverride proves --project resolves the state while the
// input file path stays relative to the invocation directory. --project is a
// global flag, so this drives run() directly.
func TestBatchProjectOverride(t *testing.T) {
	root := initBatchProject(t, "goal")
	elsewhere := t.TempDir()
	opsPath := filepath.Join(elsewhere, "ops.json")
	if err := os.WriteFile(opsPath, []byte(batchDocument(batchAdd("tests"))), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, diagnostics := runBatchRaw(elsewhere, "--project", root, "batch", "--file", "ops.json")
	if code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("override: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	state, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Cells) != 1 || state.Cells[0].ID != "tests" {
		t.Fatalf("cells=%v", state.Cells)
	}
	empty := t.TempDir()
	code, out, diagnostics = runBatchRaw(empty, "batch", "--file", opsPath)
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_NO_PROJECT") {
		t.Fatalf("no project: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func runBatchRaw(start string, args ...string) (int, string, string) {
	var out, diagnostics bytes.Buffer
	code := run(args, start, strings.NewReader(""), &out, &diagnostics, "test")
	return code, out.String(), diagnostics.String()
}

// TestBatchInputByteValidationAtCLI proves the byte-level validation (BOM,
// invalid UTF-8) is wired into the CLI path with E_BATCH_INVALID, not only
// reachable through the unit-level helper.
func TestBatchInputByteValidationAtCLI(t *testing.T) {
	root := initBatchProject(t, "goal")
	bom := writeBatchFile(t, "\xEF\xBB\xBF"+batchDocument(batchAdd("alpha")))
	code, out, diagnostics := runBatchCommand(root, "", "--file", bom)
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_BATCH_INVALID") || !strings.Contains(diagnostics, "byte order mark") {
		t.Fatalf("BOM input: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	invalidUTF8 := writeBatchFile(t, `{"batch_version":1,"operations":[{"op":"add","id":"`+"\xff"+`"}]}`)
	code, out, diagnostics = runBatchCommand(root, "", "--file", invalidUTF8)
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_BATCH_INVALID") || !strings.Contains(diagnostics, "not valid UTF-8") {
		t.Fatalf("invalid UTF-8 input: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

// TestBatchInputErrorsPrecedeProjectResolution pins the error precedence:
// input reading and validation run before project resolution, so a broken
// input reports its own code even when no project exists.
func TestBatchInputErrorsPrecedeProjectResolution(t *testing.T) {
	dir := t.TempDir()
	malformed := writeBatchFile(t, `{"batch_version":1,"operations":[}`)
	code, out, diagnostics := runBatchRaw(dir, "batch", "--file", malformed)
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_BATCH_INVALID") || strings.Contains(diagnostics, "E_NO_PROJECT") {
		t.Fatalf("malformed input without project: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	oversized := writeBatchFile(t, `{"batch_version":1,"operations":[{"op":"add","id":"`+
		strings.Repeat("a", maxBatchInputBytes)+`"}]}`)
	code, out, diagnostics = runBatchRaw(dir, "batch", "--file", oversized)
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") || strings.Contains(diagnostics, "E_NO_PROJECT") {
		t.Fatalf("oversized input without project: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

// TestBatchStdinSizeLimitCLI pins the input byte limit on the stdin path at
// CLI level: the exact limit is accepted and one byte over is rejected with
// E_LIMIT before anything is parsed or saved.
func TestBatchStdinSizeLimitCLI(t *testing.T) {
	root := initBatchProject(t, "Stdin limit")
	doc := batchDocument(batchAdd("alpha"))
	if len(doc) >= maxBatchInputBytes {
		t.Fatalf("fixture document too long: %d bytes", len(doc))
	}
	pad := func(size int) string { return doc + strings.Repeat(" ", size-len(doc)) }
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)

	code, out, diagnostics := runBatchCommand(root, pad(maxBatchInputBytes), "--file", "-", "--dry-run")
	if code != 0 || diagnostics != "" || out == "" {
		t.Fatalf("exact stdin limit: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}

	code, out, diagnostics = runBatchCommand(root, pad(maxBatchInputBytes+1), "--file", "-", "--dry-run")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") {
		t.Fatalf("stdin over limit: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
}

// TestBatchDiagnosticsBoundedNearInputLimit drives a full CLI run with a
// hostile document sized exactly at the 65,536-byte input limit: the stderr
// diagnostic must remain a bounded single line despite the ~64 KiB token.
func TestBatchDiagnosticsBoundedNearInputLimit(t *testing.T) {
	root := initBatchProject(t, "Bounded diagnostics")
	prefix := `{"batch_version":1,"operations":[{"op":"`
	suffix := `","id":"a"}]}`
	input := prefix + strings.Repeat("x", maxBatchInputBytes-len(prefix)-len(suffix)) + suffix
	if len(input) != maxBatchInputBytes {
		t.Fatalf("fixture must be exactly %d bytes, got %d", maxBatchInputBytes, len(input))
	}
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)

	code, out, diagnostics := runBatchCommand(root, input, "--file", "-", "--dry-run")
	want := "mandala: E_BATCH_INVALID: unknown batch op \"" + strings.Repeat("x", 64) + "\"\n"
	if code != 2 || out != "" || diagnostics != want {
		t.Fatalf("bounded diagnostic: exit=%d stdout=%q stderr=%q want %q", code, out, diagnostics, want)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
}

// TestBatchSaveCallCount verifies the atomicity contract's save discipline
// through a counting seam: dry-run never saves, a changed apply saves exactly
// once, and a semantic no-op apply does not save again.
func TestBatchSaveCallCount(t *testing.T) {
	root := initBatchProject(t, "Save count")
	if code, _, diagnostics := runTestCommand(root, "add", "alpha"); code != 0 {
		t.Fatal(diagnostics)
	}
	saves := 0
	original := batchSave
	batchSave = func(dir string, s State) error {
		saves++
		return original(dir, s)
	}
	t.Cleanup(func() { batchSave = original })

	path := writeBatchFile(t, batchDocument(batchMark("alpha", "done")))
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 0 || diagnostics != "" {
		t.Fatalf("dry-run: exit=%d stderr=%q", code, diagnostics)
	}
	if saves != 0 {
		t.Fatalf("dry-run saved %d time(s), want 0", saves)
	}

	code, out, diagnostics = runBatchCommand(root, "", "--file", path)
	if code != 0 || diagnostics != "" || out != "" {
		t.Fatalf("apply: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	if saves != 1 {
		t.Fatalf("apply saved %d time(s), want exactly 1", saves)
	}

	code, out, diagnostics = runBatchCommand(root, "", "--file", path)
	if code != 0 || diagnostics != "" || out != "" {
		t.Fatalf("no-op apply: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	if saves != 1 {
		t.Fatalf("no-op apply saved %d additional time(s), want 0", saves-1)
	}
}

// TestBatchSaveErrorPropagation injects a save failure through the seam and
// verifies the CLI contract on every platform (the real-I/O variant in
// TestBatchSaveFailure needs Unix permission bits and is skipped on Windows):
// exit 2, empty stdout, an E_IO diagnostic, an untouched state file, and no
// success JSON even when --json was requested.
func TestBatchSaveErrorPropagation(t *testing.T) {
	root := initBatchProject(t, "Save failure wiring")
	if code, _, diagnostics := runTestCommand(root, "add", "alpha"); code != 0 {
		t.Fatal(diagnostics)
	}
	original := batchSave
	batchSave = func(dir string, s State) error {
		return problem("E_IO", "injected save failure")
	}
	t.Cleanup(func() { batchSave = original })

	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	path := writeBatchFile(t, batchDocument(batchMark("alpha", "done")))
	for _, args := range [][]string{
		{"--file", path},
		{"--file", path, "--json"},
	} {
		code, out, diagnostics := runBatchCommand(root, "", args...)
		if code != 2 || out != "" || !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "injected save failure") {
			t.Fatalf("apply %v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
		requireStateUnchanged(t, root, before, mtime, identity)
	}
}

// TestBatchOpenSwapDetection simulates the input file being swapped for a
// different regular file between Stat and Open. The identity check must
// refuse the input instead of silently reading the swapped file.
func TestBatchOpenSwapDetection(t *testing.T) {
	root := initBatchProject(t, "Swap detection")
	if code, _, diagnostics := runTestCommand(root, "add", "alpha"); code != 0 {
		t.Fatal(diagnostics)
	}
	declared := writeBatchFile(t, batchDocument(batchAdd("beta")))
	swapped := filepath.Join(t.TempDir(), "swapped.json")
	if err := os.WriteFile(swapped, []byte(batchDocument(batchMark("alpha", "done"))), 0600); err != nil {
		t.Fatal(err)
	}
	original := batchOpenInputFile
	batchOpenInputFile = func(string) (*os.File, error) { return original(swapped) }
	t.Cleanup(func() { batchOpenInputFile = original })

	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	code, out, diagnostics := runBatchCommand(root, "", "--file", declared, "--dry-run")
	if code != 2 || out != "" {
		t.Fatalf("swapped input: exit=%d stdout=%q", code, out)
	}
	if !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "changed while opening") {
		t.Fatalf("swapped input: stderr=%q", diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
	requireOnlyStateFile(t, root)
}
