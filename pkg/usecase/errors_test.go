package usecase_test

import (
	"errors"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestErrors_SentinelErrors(t *testing.T) {
	// Test that sentinel errors are not nil
	tests := []struct {
		name string
		err  error
	}{
		{"ErrDuplicateField", usecase.ErrDuplicateField},
		{"ErrCaseNotFound", usecase.ErrCaseNotFound},
		{"ErrActionNotFound", usecase.ErrActionNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt.Value(t, tt.err).NotNil()
		})
	}
}

func TestErrors_ErrorsAreDistinct(t *testing.T) {
	// Test that sentinel errors are distinct
	gt.Bool(t, errors.Is(usecase.ErrDuplicateField, usecase.ErrCaseNotFound)).False()
	gt.Bool(t, errors.Is(usecase.ErrCaseNotFound, usecase.ErrActionNotFound)).False()
}
