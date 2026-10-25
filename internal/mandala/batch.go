package mandala

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"
)

// batchResultVersion is the schema version of the batch output contract.
const batchResultVersion = 1

// Test seams: batchOpenInputFile lets tests simulate the input file being
// swapped between Stat and Open, and batchSave lets tests count Save calls.
// Both default to the real implementations.
var (
	batchOpenInputFile = os.Open
	batchSave          = Save
)

// BatchOperation is one parsed batch input operation.
type BatchOperation struct {
	Op       string // "add" or "mark"
	ID       string // target cell ID
	Optional bool   // add only
	Status   Status // mark only: open, done, or na
}

// cloneState returns a deep copy of s. The copy shares no mutable storage
// with s, so mutations applied to the copy never reach the original.
func cloneState(s State) State {
	cells := make([]Cell, len(s.Cells))
	copy(cells, s.Cells)
	return State{SchemaVersion: s.SchemaVersion, Goal: s.Goal, Cells: cells}
}

// applyBatchOperations applies ops in order to a deep copy of s and returns
// the validated final working state. s itself is never modified. A failure in
// any operation or in the final validation leaves the caller's state untouched.
func applyBatchOperations(s State, ops []BatchOperation) (State, error) {
	work := cloneState(s)
	for i, op := range ops {
		if err := applyBatchOperation(&work, op); err != nil {
			var p *Problem
			if !validID(op.ID) && errors.As(err, &p) {
				switch {
				case op.Op == "add" && p.Code == "E_USAGE":
					err = problem(p.Code, "invalid cell ID %s", displayBatchID(op.ID))
				case op.Op == "mark" && p.Code == "E_UNKNOWN_NODE":
					err = problem(p.Code, "unknown cell %s", displayBatchID(op.ID))
				}
			}
			return State{}, fmt.Errorf("batch operation %d (%s %s): %w", i+1, displayBatchOp(op.Op), displayBatchID(op.ID), err)
		}
	}
	if err := work.Validate(); err != nil {
		return State{}, err
	}
	return work, nil
}

// truncateBatchToken caps an untrusted JSON token rendered in a diagnostic
// at 64 bytes on a UTF-8 rune boundary, so hostile inputs cannot flood
// stderr. The caller formats the result (e.g. with %q, which also escapes
// control characters and keeps the line unforgable).
func truncateBatchToken(value string) string {
	if len(value) > 64 {
		end := 64
		for end > 0 && !utf8.RuneStart(value[end]) {
			end--
		}
		return value[:end]
	}
	return value
}

// displayBatchID renders an operation ID for diagnostics. Valid IDs print
// as-is to keep the documented error format; any other value is quoted and
// capped so hostile IDs cannot forge stderr lines or flood the terminal.
func displayBatchID(id string) string {
	if validID(id) {
		return id
	}
	return fmt.Sprintf("%q", truncateBatchToken(id))
}

// displayBatchOp renders an operation kind for diagnostics. Known ops print
// as-is to keep the documented error format; any other value is quoted and
// capped like displayBatchID.
func displayBatchOp(op string) string {
	switch op {
	case "add", "mark":
		return op
	}
	return fmt.Sprintf("%q", truncateBatchToken(op))
}

func applyBatchOperation(s *State, op BatchOperation) error {
	switch op.Op {
	case "add":
		return s.Add(op.ID, op.Optional)
	case "mark":
		return s.Mark(op.ID, op.Status)
	default:
		return problem("E_BATCH_INVALID", "unknown batch op %q", truncateBatchToken(op.Op))
	}
}

// CellSnapshot captures the comparable fields of one cell in a batch diff.
type CellSnapshot struct {
	Parent   string `json:"parent"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
}

// CellChange is one entry of the deterministic semantic diff between two
// states. Before is nil for added cells.
type CellChange struct {
	ID     string        `json:"id"`
	Before *CellSnapshot `json:"before"`
	After  CellSnapshot  `json:"after"`
}

func snapshotCell(cell Cell) CellSnapshot {
	return CellSnapshot{Parent: cell.Parent, Status: string(cell.Status), Required: cell.Required}
}

// diffStates returns the semantic cell-level diff between before and after,
// sorted by cell ID in Go string order. Cells whose final state equals their
// original state are omitted.
func diffStates(before, after State) []CellChange {
	beforeByID := make(map[string]Cell, len(before.Cells))
	for _, cell := range before.Cells {
		beforeByID[cell.ID] = cell
	}
	afterByID := make(map[string]Cell, len(after.Cells))
	for _, cell := range after.Cells {
		afterByID[cell.ID] = cell
	}
	// Batch operations never remove cells, so only IDs present in after can
	// appear in the diff; a Before snapshot exists only alongside an After
	// one and the batch result schema cannot express removals.
	ids := make([]string, 0, len(afterByID))
	for id := range afterByID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	changes := make([]CellChange, 0)
	for _, id := range ids {
		afterCell := afterByID[id]
		beforeCell, inBefore := beforeByID[id]
		switch {
		case !inBefore:
			changes = append(changes, CellChange{ID: id, Before: nil, After: snapshotCell(afterCell)})
		case beforeCell.Parent != afterCell.Parent || beforeCell.Status != afterCell.Status || beforeCell.Required != afterCell.Required:
			snap := snapshotCell(beforeCell)
			changes = append(changes, CellChange{ID: id, Before: &snap, After: snapshotCell(afterCell)})
		}
	}
	return changes
}

// executeBatch is the CLI adapter for the batch command. It owns flag
// parsing, bounded input reading, project resolution, state load and save,
// and result rendering. Output is returned buffered until those stages succeed.
func executeBatch(args []string, start, project string, stdin io.Reader) (string, int, error) {
	flags := flag.NewFlagSet("batch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var file string
	fileCount := 0
	flags.Func("file", "batch input JSON file, or - for standard input", func(value string) error {
		file = value
		fileCount++
		return nil
	})
	dryRun := flags.Bool("dry-run", false, "preview changes without saving")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return commandHelp("batch"), 0, nil
		}
		return "", 0, problem("E_USAGE", "%v", err)
	}
	if flags.NArg() != 0 {
		return "", 0, problem("E_USAGE", "batch expects 0 argument(s); use --help")
	}
	if fileCount > 1 {
		return "", 0, problem("E_USAGE", "--file may be specified at most once")
	}
	if file == "" {
		return "", 0, problem("E_USAGE", "batch requires --file")
	}
	data, err := readBatchInput(file, start, stdin)
	if err != nil {
		return "", 0, err
	}
	if err := validateBatchInputBytes(data); err != nil {
		return "", 0, err
	}
	ops, err := parseBatchInput(data)
	if err != nil {
		return "", 0, err
	}
	root, err := ProjectRoot(start, project)
	if err != nil {
		return "", 0, err
	}
	original, err := Load(root)
	if err != nil {
		return "", 0, err
	}
	work, err := applyBatchOperations(original, ops)
	if err != nil {
		return "", 0, err
	}
	result := newBatchResult(original, work, ops, *dryRun)
	if *dryRun {
		output, err := renderBatchResult(result, *jsonOutput)
		if err != nil {
			return "", 0, err
		}
		return output, 0, nil
	}
	if result.Changed {
		if err := batchSave(root, work); err != nil {
			return "", 0, err
		}
	}
	if !*jsonOutput {
		return "", 0, nil
	}
	output, err := renderBatchResult(result, true)
	if err != nil {
		return "", 0, err
	}
	return output, 0, nil
}

// readBatchInput reads the bounded batch input from stdin (-) or a regular
// file path resolved against the invocation directory, never --project.
func readBatchInput(file, start string, stdin io.Reader) ([]byte, error) {
	if file == "-" {
		data, err := readBoundedBatchInput(stdin)
		if err != nil {
			return nil, fmt.Errorf("read batch input from standard input: %w", err)
		}
		return data, nil
	}
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(start, path)
	}
	// Stat before opening: opening a FIFO (or another special file that
	// blocks in open) would hang, so regular-file status is checked first
	// and the open is allowed only for regular files.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect batch input: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, problem("E_IO", "batch input %q is not a regular file", file)
	}
	f, err := batchOpenInputFile(path)
	if err != nil {
		return nil, fmt.Errorf("open batch input: %w", err)
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect batch input: %w", err)
	}
	if !openedInfo.Mode().IsRegular() {
		return nil, problem("E_IO", "batch input %q is not a regular file", file)
	}
	if !os.SameFile(info, openedInfo) {
		return nil, problem("E_IO", "batch input %q changed while opening", file)
	}
	return readBoundedBatchInput(f)
}
