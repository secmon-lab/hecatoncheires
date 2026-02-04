package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
)

// Repository defines the interface for data persistence
type Repository interface {
	Risk() RiskRepository
	Response() ResponseRepository
	RiskResponse() RiskResponseRepository
	Slack() SlackRepository
	SlackUser() SlackUserRepository
	Source() SourceRepository
	Knowledge() KnowledgeRepository

	// Auth methods
	PutToken(ctx context.Context, token *auth.Token) error
	GetToken(ctx context.Context, tokenID auth.TokenID) (*auth.Token, error)
	DeleteToken(ctx context.Context, tokenID auth.TokenID) error
}
