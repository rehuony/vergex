package agent

import "strings"

// targetRef identifies a resource the user referred to. Resolution order:
// exact id, exact name (case-insensitive), then a unique name substring.
type targetRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

func (r *targetRef) empty() bool {
	return r == nil || (strings.TrimSpace(r.ID) == "" && strings.TrimSpace(r.Name) == "")
}

// named is one candidate for reference resolution.
type named struct {
	ID   string
	Name string
}

// resolveError is returned to the model instead of guessing a target.
type resolveError struct {
	Code       string   `json:"error"` // not_found | ambiguous | none_exist
	Kind       string   `json:"kind"`
	Message    string   `json:"message"`
	Candidates []string `json:"candidates,omitempty"`
}

func describe(items []named) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Name+" (id="+it.ID+")")
	}
	return out
}

// resolve picks exactly one item for ref. With an empty ref it only succeeds
// when there is a single candidate; it never guesses between several.
func resolve(kind string, items []named, ref *targetRef) (named, *resolveError) {
	if len(items) == 0 {
		return named{}, &resolveError{Code: "none_exist", Kind: kind, Message: "no " + kind + " exists yet"}
	}
	if ref.empty() {
		if len(items) == 1 {
			return items[0], nil
		}
		return named{}, &resolveError{Code: "ambiguous", Kind: kind, Message: "several " + kind + "s exist; ask the user which one", Candidates: describe(items)}
	}
	if id := strings.TrimSpace(ref.ID); id != "" {
		for _, it := range items {
			if it.ID == id {
				return it, nil
			}
		}
	}
	name := strings.ToLower(strings.TrimSpace(ref.Name))
	if name == "" {
		// An id that matched nothing may really be a name.
		name = strings.ToLower(strings.TrimSpace(ref.ID))
	}
	var exact, partial []named
	for _, it := range items {
		lower := strings.ToLower(it.Name)
		switch {
		case lower == name:
			exact = append(exact, it)
		case name != "" && strings.Contains(lower, name):
			partial = append(partial, it)
		}
	}
	for _, matches := range [][]named{exact, partial} {
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return named{}, &resolveError{Code: "ambiguous", Kind: kind, Message: "several " + kind + "s match; ask the user which one", Candidates: describe(matches)}
		}
	}
	return named{}, &resolveError{Code: "not_found", Kind: kind, Message: "no " + kind + " matches the reference; do not guess", Candidates: describe(items)}
}
