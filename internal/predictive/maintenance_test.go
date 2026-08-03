package predictive

import "testing"

func TestNewPredictiveMaintenance(t *testing.T) {
	t.Parallel()
	pm := NewPredictiveMaintenance()
	if pm == nil {
		t.Fatal("NewPredictiveMaintenance() returned nil")
	}
}
