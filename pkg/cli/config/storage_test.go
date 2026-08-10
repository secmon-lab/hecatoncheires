package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestStorage_ConfigureRequiresBucket(t *testing.T) {
	var s config.Storage
	archive, err := s.Configure(context.Background())
	gt.Error(t, err)
	gt.Value(t, archive).Nil()
}
