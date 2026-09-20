package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadWorkflowConsumer(t *testing.T) {
	t.Setenv("TEST_BROKER_URL", "redis://localhost:6379")
	for _, tt := range []struct {
		name, data string
		valid      bool
	}{
		{"valid", "driver: redis\nsubscription: events\noptions:\n  url: ${TEST_BROKER_URL}\n  group: workers\n", true},
		{"unknown field", "driver: redis\nsubscription: events\nsubscriptions: typo\n", false},
		{"missing subscription", "driver: kafka\n", false},
		{"missing environment", "driver: redis\nsubscription: events\noptions:\n  url: ${FI_FHIR_TEST_ENV_NOT_SET_88271}\n", false},
		{"multiple documents", "driver: redis\nsubscription: events\n---\ndriver: kafka\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "backend.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tt.data), 0600))
			config, err := loadWorkflowConsumer(path)
			if !tt.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "redis://localhost:6379", config.Options["url"])
		})
	}
}

func TestWorkflowConsumeRequiresConfiguration(t *testing.T) {
	require.Error(t, runWorkflow([]string{"consume"}))
	require.NoError(t, runWorkflow([]string{"consume", "--help"}))
	require.Error(t, runWorkflow([]string{"consume", "--unknown"}))
}
