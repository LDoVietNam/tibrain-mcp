package verification

import "testing"

func TestVerificationResult_New(t *testing.T) {
	result := &VerificationResult{
		Passed:   true,
		Feedback: "All checks passed",
		Errors:   []error{},
	}

	if !result.Passed {
		t.Errorf("Expected Passed to be true")
	}

	if result.Feedback != "All checks passed" {
		t.Errorf("Expected Feedback 'All checks passed', got '%s'", result.Feedback)
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(result.Errors))
	}
}

func TestBasicVerifier_Verify(t *testing.T) {
	verifier := &BasicVerifier{SchemaValidator: NewSchemaValidator()}

	result, err := verifier.Verify(nil, "valid input")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatalf("Expected non-nil result")
	}

	if !result.Passed {
		t.Errorf("Expected verification to pass")
	}

	if result.Feedback == "" {
		t.Errorf("Expected non-empty feedback")
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors")
	}
}
