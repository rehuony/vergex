package testdb

import (
	"strings"
	"testing"
	"time"
)

// The port is closed, so these cases can't reach any database even if the check fails.
func TestRefusesADatabaseNotNamedAsATestDatabase(t *testing.T) {
	for _, name := range []string{"vergex", "vergex_test_copy", "production", ""} {
		t.Setenv("VERGEX_TEST_DATABASE_URL", "postgres://nobody:nothing@127.0.0.1:1/"+name+"?sslmode=disable")
		_, err := prepare()
		if err == nil || !strings.Contains(err.Error(), "must end in _test") {
			t.Errorf("database %q: err = %v", name, err)
		}
	}
}

func TestOnlyNamesFromNewNameCountAsClones(t *testing.T) {
	const template = "vergex_test"
	if made, ok := cloneTime("vergex_test_1700000000_0123456789ab", template); !ok || made.Unix() != 1700000000 {
		t.Fatalf("a real clone name: %v %v", made, ok)
	}
	for _, name := range []string{
		"vergex_test",
		"vergex_test_backup",
		"vergex_test_2024_backup",
		"vergex_test_1700000000_notHexAtAll!",
		"vergex_test_1700000000_0123456789ab_old",
		"other_vergex_test_1700000000_0123456789ab",
		"vergexXtest_1700000000_0123456789ab",
	} {
		if made, ok := cloneTime(name, template); ok {
			t.Errorf("%s counted as a clone made at %s", name, made.Format(time.RFC3339))
		}
	}
}
