package plan

import "testing"

func TestUnknownEntryPolicyRejected(t *testing.T) {
	_, limits, _, _ := fixture()
	limits.EntryPolicy = "unknown"
	if limits.Validate() == nil {
		t.Fatal("unknown entry policy accepted")
	}
}
