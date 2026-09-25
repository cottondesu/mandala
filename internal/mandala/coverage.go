package mandala

import "strings"

func (s *State) Add(id string, optional bool) error {
	if !validID(id) {
		return problem("E_USAGE", "invalid cell ID %q", id)
	}
	for _, cell := range s.Cells {
		if cell.ID == id {
			return problem("E_DUPLICATE_ID", "duplicate cell ID %q", id)
		}
	}
	parentID := ""
	required := !optional
	if dot := strings.IndexByte(id, '.'); dot >= 0 {
		parentID = id[:dot]
		parent := s.find(parentID)
		if parent == nil {
			return problem("E_INVALID_PARENT", "missing parent %q for %q", parentID, id)
		}
		if parent.Status == Done || parent.Status == NotApplicable {
			return problem("E_STATE_INVALID", "reopen %q before adding a child", parentID)
		}
		if !parent.Required {
			required = false
		}
		parent.Status = Expanded
	}
	s.Cells = append(s.Cells, Cell{ID: id, Parent: parentID, Status: Open, Required: required})
	if err := s.Validate(); err != nil {
		s.Cells = s.Cells[:len(s.Cells)-1]
		if parentID != "" {
			parent := s.find(parentID)
			if parent != nil && !s.hasChild(parentID) {
				parent.Status = Open
			}
		}
		return err
	}
	return nil
}

func (s *State) Mark(id string, status Status) error {
	if status != Open && status != Done && status != NotApplicable {
		return problem("E_USAGE", "status must be open, done, or na")
	}
	cell := s.find(id)
	if cell == nil {
		return problem("E_UNKNOWN_NODE", "unknown cell %q", id)
	}
	if cell.Status == Expanded {
		return problem("E_STATE_INVALID", "cannot mark expanded cell %q", id)
	}
	cell.Status = status
	return nil
}

func (s *State) find(id string) *Cell {
	for i := range s.Cells {
		if s.Cells[i].ID == id {
			return &s.Cells[i]
		}
	}
	return nil
}

func (s State) hasChild(id string) bool {
	for _, cell := range s.Cells {
		if cell.Parent == id {
			return true
		}
	}
	return false
}

func (s State) Gaps(requiredOnly bool) []Gap {
	gaps := make([]Gap, 0)
	for _, cell := range s.SortedCells() {
		if cell.Status == Open && (!requiredOnly || cell.Required) {
			gaps = append(gaps, Gap{ID: cell.ID, Required: cell.Required})
		}
	}
	return gaps
}

type Counts struct {
	Groups       int
	RequiredOpen int
	RequiredDone int
	RequiredNA   int
	OptionalOpen int
	OptionalDone int
	OptionalNA   int
}

func (s State) Counts() Counts {
	var counts Counts
	for _, cell := range s.Cells {
		if cell.Status == Expanded {
			counts.Groups++
			continue
		}
		switch {
		case cell.Required && cell.Status == Open:
			counts.RequiredOpen++
		case cell.Required && cell.Status == Done:
			counts.RequiredDone++
		case cell.Required && cell.Status == NotApplicable:
			counts.RequiredNA++
		case !cell.Required && cell.Status == Open:
			counts.OptionalOpen++
		case !cell.Required && cell.Status == Done:
			counts.OptionalDone++
		case !cell.Required && cell.Status == NotApplicable:
			counts.OptionalNA++
		}
	}
	return counts
}
