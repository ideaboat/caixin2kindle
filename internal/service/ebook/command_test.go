package ebook

import (
	"slices"
	"testing"
)

func TestBuildConvertPlan(t *testing.T) {
	// Arrange
	const (
		input   = "/out/财新周刊第1224期/财新周刊第1224期.epub"
		output  = "/out/财新周刊第1224期/财新周刊第1224期.mobi"
		profile = "kindle"
	)

	// Act
	plan := BuildConvertPlan(input, output, profile)

	// Assert
	if plan.InputPath != input {
		t.Errorf("InputPath = %q，期望 %q", plan.InputPath, input)
	}
	if plan.OutputPath != output {
		t.Errorf("OutputPath = %q，期望 %q", plan.OutputPath, output)
	}
	if plan.OutputProfile != profile {
		t.Errorf("OutputProfile = %q，期望 %q", plan.OutputProfile, profile)
	}
	wantArgs := []string{input, output, "--output-profile", profile}
	if !slices.Equal(plan.Args, wantArgs) {
		t.Errorf("Args = %q，期望 %q", plan.Args, wantArgs)
	}
}
