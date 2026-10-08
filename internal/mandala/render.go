package mandala

import (
	"encoding/json"
	"fmt"
	"strings"
)

const statusOutputSchemaVersion = 1

type statusResponse struct {
	SchemaVersion int          `json:"schema_version"`
	Goal          string       `json:"goal"`
	Cells         int          `json:"cells"`
	Groups        int          `json:"groups"`
	Required      statusCounts `json:"required"`
	Optional      statusCounts `json:"optional"`
	RequiredGaps  int          `json:"required_gaps"`
}

type statusCounts struct {
	Open int `json:"open"`
	Done int `json:"done"`
	NA   int `json:"na"`
}

func renderStatusJSON(s State) (string, error) {
	c := s.Counts()
	response := statusResponse{
		SchemaVersion: statusOutputSchemaVersion,
		Goal:          s.Goal,
		Cells:         len(s.Cells),
		Groups:        c.Groups,
		Required:      statusCounts{Open: c.RequiredOpen, Done: c.RequiredDone, NA: c.RequiredNA},
		Optional:      statusCounts{Open: c.OptionalOpen, Done: c.OptionalDone, NA: c.OptionalNA},
		RequiredGaps:  c.RequiredOpen,
	}
	data, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("encode status: %w", err)
	}
	return string(data) + "\n", nil
}

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
	writeCell := func(cell Cell) {
		if cell.Parent != "" {
			text.WriteString("  ")
		}
		kind := "required"
		if !cell.Required {
			kind = "optional"
		}
		fmt.Fprintf(&text, "%s [%s, %s]\n", cell.ID, cell.Status, kind)
	}
	for _, root := range cells {
		if root.Parent != "" {
			continue
		}
		writeCell(root)
		for _, child := range cells {
			if child.Parent == root.ID {
				writeCell(child)
			}
		}
	}
	return text.String(), nil
}
