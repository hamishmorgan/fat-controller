package apply

import "fmt"

// Result holds apply summary counts.
type Result struct {
	Applied int `json:"applied" toml:"applied"`
	Failed  int `json:"failed" toml:"failed"`
}

// Summary returns a concise summary string.
func (r *Result) Summary() string {
	if r == nil {
		return "applied=0 failed=0"
	}
	return fmt.Sprintf("applied=%d failed=%d", r.Applied, r.Failed)
}

// HasFailures returns true if any operations failed.
func (r *Result) HasFailures() bool {
	return r != nil && r.Failed > 0
}
