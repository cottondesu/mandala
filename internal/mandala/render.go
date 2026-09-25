package mandala

import (
	"encoding/json"
	"fmt"
	"strings"
)

func renderStatus(s State) string {
	c := s.Counts()
	return fmt.Sprintf("goal: %s\ncells: %d\ngroups: %d\nrequired: open=%d done=%d na=%d\noptional: open=%d done=%d na=%d\nrequired gaps: %d\n",
		s.Goal, len(s.Cells), c.Groups, c.RequiredOpen, c.RequiredDone, c.RequiredNA,
		c.OptionalOpen, c.OptionalDone, c.OptionalNA, c.RequiredOpen)
}

func renderGaps(s State, requiredOnly, asJSON bool) (string, error) {
	gaps := s.Gaps(requiredOnly)
	if asJSON {
		response := struct {
			SchemaVersion int   `json:"schema_version"`
			Gaps          []Gap `json:"gaps"`
		}{SchemaVersion: schemaVersion, Gaps: gaps}
		data, err := json.Marshal(response)
		if err != nil {
			return "", fmt.Errorf("encode gaps: %w", err)
		}
		return string(data) + "\n", nil
	}
	var text strings.Builder
	for _, gap := range gaps {
		text.WriteString(gap.ID)
		if !gap.Required {
			text.WriteString(" [optional]")
		}
		text.WriteByte('\n')
	}
	return text.String(), nil
}

func renderShow(s State, asJSON bool) (string, error) {
	cells := s.SortedCells()
	if asJSON {
		response := State{SchemaVersion: schemaVersion, Goal: s.Goal, Cells: cells}
		data, err := json.Marshal(response)
		if err != nil {
			return "", fmt.Errorf("encode cells: %w", err)
		}
		return string(data) + "\n", nil
	}
	var text strings.Builder
	fmt.Fprintf(&text, "goal: %s\n", s.Goal)
	for _, cell := range cells {
		if cell.Parent != "" {
			text.WriteString("  ")
		}
		kind := "required"
		if !cell.Required {
			kind = "optional"
		}
		fmt.Fprintf(&text, "%s [%s, %s]\n", cell.ID, cell.Status, kind)
	}
	return text.String(), nil
}
