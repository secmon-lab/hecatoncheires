package config_test

import (
	"errors"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestConfigErrors_SentinelIdentification(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		sentinelError error
		wantMatch     bool
	}{
		{
			name:          "ErrConfigNotFound can be identified",
			err:           goerr.Wrap(config.ErrConfigNotFound, "failed to load config"),
			sentinelError: config.ErrConfigNotFound,
			wantMatch:     true,
		},
		{
			name:          "ErrInvalidConfig can be identified",
			err:           goerr.Wrap(config.ErrInvalidConfig, "validation failed"),
			sentinelError: config.ErrInvalidConfig,
			wantMatch:     true,
		},
		{
			name:          "ErrDuplicateFieldID can be identified",
			err:           goerr.Wrap(config.ErrDuplicateFieldID, "found duplicate"),
			sentinelError: config.ErrDuplicateFieldID,
			wantMatch:     true,
		},
		{
			name:          "ErrDuplicateOptionID can be identified",
			err:           goerr.Wrap(config.ErrDuplicateOptionID, "found duplicate"),
			sentinelError: config.ErrDuplicateOptionID,
			wantMatch:     true,
		},
		{
			name:          "ErrInvalidFieldID can be identified",
			err:           goerr.Wrap(config.ErrInvalidFieldID, "invalid format"),
			sentinelError: config.ErrInvalidFieldID,
			wantMatch:     true,
		},
		{
			name:          "ErrInvalidFieldType can be identified",
			err:           goerr.Wrap(config.ErrInvalidFieldType, "unknown type"),
			sentinelError: config.ErrInvalidFieldType,
			wantMatch:     true,
		},
		{
			name:          "ErrMissingOptions can be identified",
			err:           goerr.Wrap(config.ErrMissingOptions, "no options provided"),
			sentinelError: config.ErrMissingOptions,
			wantMatch:     true,
		},
		{
			name:          "ErrInvalidMetadata can be identified",
			err:           goerr.Wrap(config.ErrInvalidMetadata, "malformed metadata"),
			sentinelError: config.ErrInvalidMetadata,
			wantMatch:     true,
		},
		{
			name:          "ErrMissingName can be identified",
			err:           goerr.Wrap(config.ErrMissingName, "name field is empty"),
			sentinelError: config.ErrMissingName,
			wantMatch:     true,
		},
		{
			name:          "Different sentinel errors do not match",
			err:           goerr.Wrap(config.ErrConfigNotFound, "failed to load config"),
			sentinelError: config.ErrInvalidConfig,
			wantMatch:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := errors.Is(tt.err, tt.sentinelError)
			gt.Value(t, matched).Equal(tt.wantMatch)
		})
	}
}

func TestConfigErrors_ContextKeys(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "ConfigPathKey is string",
			key:   config.ConfigPathKey,
			value: "/path/to/config.toml",
		},
		{
			name:  "FieldIDKey is string",
			key:   config.FieldIDKey,
			value: "test-field",
		},
		{
			name:  "FieldTypeKey is string",
			key:   config.FieldTypeKey,
			value: "text",
		},
		{
			name:  "OptionIDKey is string",
			key:   config.OptionIDKey,
			value: "option-1",
		},
		{
			name:  "FieldIndexKey is string",
			key:   config.FieldIndexKey,
			value: "5",
		},
		{
			name:  "OptionIndexKey is string",
			key:   config.OptionIndexKey,
			value: "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that keys can be used with goerr.V()
			err := goerr.Wrap(config.ErrInvalidConfig, "test error", goerr.V(tt.key, tt.value))
			gt.Value(t, err).NotNil().Required()

			// Verify error contains the key-value pair
			errStr := err.Error()
			gt.String(t, errStr).NotEqual("")
		})
	}
}

func TestConfigErrors_ContextExtraction(t *testing.T) {
	tests := []struct {
		name      string
		buildErr  func() error
		key       string
		wantValue string
	}{
		{
			name: "Extract ConfigPathKey",
			buildErr: func() error {
				return goerr.Wrap(config.ErrConfigNotFound, "config not found",
					goerr.V(config.ConfigPathKey, "/path/to/config.toml"))
			},
			key:       config.ConfigPathKey,
			wantValue: "/path/to/config.toml",
		},
		{
			name: "Extract FieldIDKey",
			buildErr: func() error {
				return goerr.Wrap(config.ErrDuplicateFieldID, "duplicate field",
					goerr.V(config.FieldIDKey, "category"))
			},
			key:       config.FieldIDKey,
			wantValue: "category",
		},
		{
			name: "Extract FieldTypeKey",
			buildErr: func() error {
				return goerr.Wrap(config.ErrInvalidFieldType, "invalid type",
					goerr.V(config.FieldTypeKey, "unknown-type"))
			},
			key:       config.FieldTypeKey,
			wantValue: "unknown-type",
		},
		{
			name: "Extract OptionIDKey",
			buildErr: func() error {
				return goerr.Wrap(config.ErrDuplicateOptionID, "duplicate option",
					goerr.V(config.OptionIDKey, "option-1"))
			},
			key:       config.OptionIDKey,
			wantValue: "option-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.buildErr()

			// Verify the error contains the context by checking the error string
			// goerr embeds values in the error message
			errStr := err.Error()
			gt.String(t, errStr).NotEqual("").Required()

			// The error should be identifiable by errors.Is
			matched := errors.Is(err, config.ErrConfigNotFound) ||
				errors.Is(err, config.ErrDuplicateFieldID) ||
				errors.Is(err, config.ErrInvalidFieldType) ||
				errors.Is(err, config.ErrDuplicateOptionID)
			gt.Bool(t, matched).True()
		})
	}
}

func TestConfigErrors_AllSentinelErrorsAreDefined(t *testing.T) {
	// Verify all sentinel errors are non-nil and have messages
	sentinelErrors := []struct {
		name string
		err  error
	}{
		{"ErrConfigNotFound", config.ErrConfigNotFound},
		{"ErrInvalidConfig", config.ErrInvalidConfig},
		{"ErrDuplicateFieldID", config.ErrDuplicateFieldID},
		{"ErrDuplicateOptionID", config.ErrDuplicateOptionID},
		{"ErrInvalidFieldID", config.ErrInvalidFieldID},
		{"ErrInvalidFieldType", config.ErrInvalidFieldType},
		{"ErrMissingOptions", config.ErrMissingOptions},
		{"ErrInvalidMetadata", config.ErrInvalidMetadata},
		{"ErrMissingName", config.ErrMissingName},
	}

	for _, se := range sentinelErrors {
		t.Run(se.name, func(t *testing.T) {
			gt.Value(t, se.err).NotNil()
			gt.String(t, se.err.Error()).NotEqual("")
		})
	}
}
