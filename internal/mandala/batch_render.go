package mandala

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BatchResult is the deterministic result document for dry-run and applied
// batches. Field order fixes the JSON output order.
type BatchResult struct {
	ResultVersion      int          `json:"batch_result_version"`
	DryRun             bool         `json:"dry_run"`
	Changed            bool         `json:"changed"`
	Operations         int          `json:"operations"`
	RequiredGapsBefore int          `json:"required_gaps_before"`
	RequiredGapsAfter  int          `json:"required_gaps_after"`
	Changes            []CellChange `json:"changes"`
}

// newBatchResult assembles the result of applying ops that turned original
// into work.
func newBatchResult(original, work State, ops []BatchOperation, dryRun bool) BatchResult {
	changes := diffStates(original, work)
	return BatchResult{
		ResultVersion:      batchResultVersion,
		DryRun:             dryRun,
		Changed:            len(changes) > 0,
		Operations:         len(ops),
		RequiredGapsBefore: original.Counts().RequiredOpen,
		RequiredGapsAfter:  work.Counts().RequiredOpen,
		Changes:            changes,
	}
}

// renderBatchResult writes the compact one-line JSON document or the fixed
// text preview. Both forms end with exactly one LF.
func renderBatchResult(result BatchResult, asJSON bool) (string, error) {
	if asJSON {
		data, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("encode batch result: %w", err)
		}
		return string(data) + "\n", nil
	}
	var text strings.Builder
	fmt.Fprintf(&text, "batch dry-run: %d operation(s), %d changed cell(s)\n", result.Operations, len(result.Changes))
	fmt.Fprintf(&text, "required gaps: %d -> %d\n", result.RequiredGapsBefore, result.RequiredGapsAfter)
	for _, change := range result.Changes {
		afterKind := kindLabel(change.After.Required)
		if change.Before == nil {
			fmt.Fprintf(&text, "+ %s [%s, %s]\n", change.ID, change.After.Status, afterKind)
			continue
		}
		fmt.Fprintf(&text, "~ %s [%s, %s] -> [%s, %s]\n", change.ID, change.Before.Status, kindLabel(change.Before.Required), change.After.Status, afterKind)
	}
	return text.String(), nil
}

func kindLabel(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}
