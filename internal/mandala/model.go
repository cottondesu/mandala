package mandala

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const schemaVersion = 1

type Status string

const (
	Open          Status = "open"
	Done          Status = "done"
	NotApplicable Status = "na"
	Expanded      Status = "expanded"
)

type State struct {
	SchemaVersion int    `json:"schema_version"`
	Goal          string `json:"goal"`
	Cells         []Cell `json:"cells"`
}

type Cell struct {
	ID       string `json:"id"`
	Parent   string `json:"parent"`
	Status   Status `json:"status"`
	Required bool   `json:"required"`
}

type Gap struct {
	ID       string `json:"id"`
	Required bool   `json:"required"`
}

type Problem struct {
	Code    string
	Message string
}

func (p *Problem) Error() string { return p.Message }

func problem(code, format string, args ...any) error {
	return &Problem{Code: code, Message: fmt.Sprintf(format, args...)}
}

func NewState(goal string) (State, error) {
	s := State{SchemaVersion: schemaVersion, Goal: goal, Cells: []Cell{}}
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	return s, nil
}

func (s State) Validate() error {
	if s.SchemaVersion != schemaVersion {
		return problem("E_STATE_INVALID", "unsupported schema_version %d", s.SchemaVersion)
	}
	if strings.TrimSpace(s.Goal) == "" || len(s.Goal) > 512 || !utf8.ValidString(s.Goal) {
		return problem("E_STATE_INVALID", "goal must be nonempty UTF-8 and at most 512 bytes")
	}
	for _, r := range s.Goal {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return problem("E_STATE_INVALID", "goal must be one line without control or line separator characters")
		}
	}
	if len(s.Cells) > 72 {
		return problem("E_LIMIT", "state exceeds 72 cells")
	}
	byID := make(map[string]Cell, len(s.Cells))
	children := make(map[string]int, len(s.Cells))
	for _, cell := range s.Cells {
		if !validID(cell.ID) {
			return problem("E_STATE_INVALID", "invalid cell ID %q", cell.ID)
		}
		if _, exists := byID[cell.ID]; exists {
			return problem("E_DUPLICATE_ID", "duplicate cell ID %q", cell.ID)
		}
		if cell.Status != Open && cell.Status != Done && cell.Status != NotApplicable && cell.Status != Expanded {
			return problem("E_STATE_INVALID", "invalid status %q for %q", cell.Status, cell.ID)
		}
		byID[cell.ID] = cell
		children[cell.Parent]++
	}
	if children[""] > 8 {
		return problem("E_LIMIT", "root exceeds 8 children")
	}
	for _, cell := range s.Cells {
		parts := strings.Split(cell.ID, ".")
		if len(parts) == 1 {
			if cell.Parent != "" {
				return problem("E_INVALID_PARENT", "root cell %q must have empty parent", cell.ID)
			}
		} else {
			parent, exists := byID[cell.Parent]
			if !exists {
				return problem("E_INVALID_PARENT", "missing parent %q for %q", cell.Parent, cell.ID)
			}
			if cell.Parent != parts[0] {
				return problem("E_INVALID_PARENT", "parent of %q must be %q", cell.ID, parts[0])
			}
			if cell.Required && !parent.Required {
				return problem("E_INVALID_PARENT", "required child %q cannot have optional parent", cell.ID)
			}
		}
		if children[cell.ID] > 8 {
			return problem("E_LIMIT", "%q exceeds 8 children", cell.ID)
		}
		if children[cell.ID] > 0 && cell.Status != Expanded {
			return problem("E_STATE_INVALID", "cell %q with children must be expanded", cell.ID)
		}
		if children[cell.ID] == 0 && cell.Status == Expanded {
			return problem("E_STATE_INVALID", "expanded cell %q has no children", cell.ID)
		}
	}
	return nil
}

func validID(id string) bool {
	parts := strings.Split(id, ".")
	if len(parts) < 1 || len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 64 || part[0] < 'a' || part[0] > 'z' {
			return false
		}
		for i := 1; i < len(part); i++ {
			c := part[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

func (s State) SortedCells() []Cell {
	cells := slices.Clone(s.Cells)
	slices.SortFunc(cells, func(a, b Cell) int { return strings.Compare(a.ID, b.ID) })
	return cells
}
