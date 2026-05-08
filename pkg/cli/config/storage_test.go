package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestStorage_ConfigureRequiresBucket(t *testing.T) {
	var s config.Storage
	historyRepo, traceRepo, cleanup, err := s.Configure(context.Background())
	gt.Error(t, err)
	gt.Value(t, historyRepo).Nil()
	gt.Value(t, traceRepo).Nil()
	gt.Value(t, cleanup).Nil()
}
