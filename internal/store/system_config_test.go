package store

import "testing"

func TestDeleteSystemConfigRemovesKey(t *testing.T) {
	st, err := New(t.TempDir() + "/vergex.db")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := st.SetSystemConfig("example_key", "value"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSystemConfig("example_key"); err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetSystemConfig("example_key"); err != nil || got != "" {
		t.Fatalf("example_key = %q, %v; want removed", got, err)
	}
	if err := st.DeleteSystemConfig("example_key"); err != nil {
		t.Fatalf("deleting a missing key should succeed: %v", err)
	}
}
