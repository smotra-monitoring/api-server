package agent_configuration

import (
	"testing"

	"github.com/smotra-monitoring/server/internal/logger"
	"github.com/smotra-monitoring/server/internal/testutil"
)

func TestNewHandler(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()
	version := "1.0.0"

	handler := NewHandler(log, db, version)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
}

func TestHandler_GetMetrics(t *testing.T) {
	log := logger.Default()
	db := testutil.NewMockDatabase()

	handler := NewHandler(log, db, "1.0.0")

	metrics := handler.GetMetrics()

	if metrics == nil {
		t.Fatal("GetMetrics returned nil")
	}

	expectedKeys := []string{
		"get_configuration_total",
		"get_configuration_success",
		"get_configuration_failure",
	}

	for _, key := range expectedKeys {
		if _, ok := metrics[key]; !ok {
			t.Errorf("Expected metric %s to be present", key)
		}
	}
}

func TestHandler_GetAgentConfiguration_NotFound(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}

func TestHandler_GetAgentConfiguration_Response(t *testing.T) {
	t.Skip("Skipping unit test - requires real database. See integration tests.")
}
