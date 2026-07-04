package usecase

import "context"

type Service struct{}

// Exported use-case method with no context.Context first parameter — flagged.
func (s *Service) DoWork(name string) error { return nil }

// Exported use-case function with no parameters at all — flagged.
func Refresh() {}

// context.Context present but not the first parameter — flagged.
func (s *Service) Handle(name string, ctx context.Context) error { return nil }
