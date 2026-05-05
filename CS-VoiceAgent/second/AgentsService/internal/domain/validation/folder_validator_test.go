package validation

import "testing"

func TestValidateFolderName_OK(t *testing.T) {
	if err := ValidateFolderName("Sales Agents"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateFolderName_TemplateAgentsForbidden(t *testing.T) {
	if err := ValidateFolderName("Template Agents"); err == nil {
		t.Fatalf("expected error")
	}
}

