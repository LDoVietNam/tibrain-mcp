package verification

import (
	"context"
	"fmt"
	"reflect"
)

// Verifier defines the interface for verifying execution results
type Verifier interface {
	Verify(ctx context.Context, exec interface{}) (*VerificationResult, error)
}

// VerificationResult holds the result of a verification
type VerificationResult struct {
	Passed   bool
	Feedback string
	Errors   []error
}

// VerificationRule defines a single verification rule
type VerificationRule struct {
	Name     string
	Validate func(ctx context.Context, result interface{}) (bool, string, error)
}

// SchemaValidator validates result against schema
type SchemaValidator struct {
	Rules []VerificationRule
}

// NewSchemaValidator creates validator with default rules
func NewSchemaValidator() *SchemaValidator {
	return &SchemaValidator{
		Rules: []VerificationRule{
			{Name: "non_nil", Validate: validateNonNil},
			{Name: "non_empty", Validate: validateNonEmpty},
		},
	}
}

func validateNonNil(ctx context.Context, result interface{}) (bool, string, error) {
	if result == nil {
		return false, "result is nil", nil
	}
	return true, "result is not nil", nil
}

func validateNonEmpty(ctx context.Context, result interface{}) (bool, string, error) {
	if result == nil {
		return false, "result is nil", nil
	}
	v := reflect.ValueOf(result)
	switch v.Kind() {
	case reflect.Slice, reflect.Map, reflect.String:
		if v.Len() == 0 {
			return false, fmt.Sprintf("result is empty %s", v.Kind()), nil
		}
	case reflect.Struct:
		// Struct is considered non-empty
	}
	return true, "result is not empty", nil
}

// Verify validates result against all rules
func (v *SchemaValidator) Verify(ctx context.Context, exec interface{}) (*VerificationResult, error) {
	result := exec
	if m, ok := exec.(map[string]interface{}); ok {
		if r, ok := m["Result"]; ok {
			result = r
		}
	}

	var errors []error
	var feedback []string

	for _, rule := range v.Rules {
		passed, msg, err := rule.Validate(ctx, result)
		if err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", rule.Name, err))
			continue
		}
		if !passed {
			errors = append(errors, fmt.Errorf("rule %s failed: %s", rule.Name, msg))
			feedback = append(feedback, msg)
		}
	}

	return &VerificationResult{
		Passed:   len(errors) == 0,
		Feedback: fmt.Sprintf("Schema validation: %s", feedback),
		Errors:   errors,
	}, nil
}

// BasicVerifier implements a simple verifier that always passes
type BasicVerifier struct{}

// NewBasicVerifier creates a new basic verifier
func NewBasicVerifier() *BasicVerifier {
	return &BasicVerifier{}
}

// Verify always returns a passed result (stub implementation)
func (v *BasicVerifier) Verify(ctx context.Context, exec interface{}) (*VerificationResult, error) {
	return &VerificationResult{
		Passed:   true,
		Feedback: "Verification passed (stub implementation)",
		Errors:   []error{},
	}, nil
}
