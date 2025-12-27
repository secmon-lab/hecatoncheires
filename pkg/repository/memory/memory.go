package memory

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

type Memory struct {
	// Future: Add data storage fields here
}

var _ interfaces.Repository = &Memory{}

func New() *Memory {
	return &Memory{}
}
