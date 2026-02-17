package main

import "testing"

func TestGetTerminologyDBURL(t *testing.T) {
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://env/test")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "uses_flag",
			args: []string{"--db", "postgres://flag/test"},
			want: "postgres://flag/test",
		},
		{
			name: "ignores_missing_value",
			args: []string{"--db"},
			want: "postgres://env/test",
		},
		{
			name: "uses_env_fallback",
			args: []string{"--other", "x"},
			want: "postgres://env/test",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getTerminologyDBURL(tc.args)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
