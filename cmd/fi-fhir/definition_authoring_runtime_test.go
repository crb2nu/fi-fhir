package main

import "testing"

func TestLifecycleValidationMaxAgeFromEnv(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int64
		ok    bool
	}{
		{"", 300, true}, {"600", 600, true}, {"5", 5, true}, {"86400", 86400, true},
		{"4", 0, false}, {"86401", 0, false}, {"05", 0, false}, {"5m", 0, false}, {"-1", 0, false},
	} {
		t.Setenv(envLifecycleValidationMaxAge, test.value)
		got, err := lifecycleValidationMaxAgeFromEnv()
		if (err == nil) != test.ok || got != test.want {
			t.Errorf("%q = %d, %v; want %d ok=%v", test.value, got, err, test.want, test.ok)
		}
	}
}
