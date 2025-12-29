package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ResponseRepository defines the interface for Response data access
type ResponseRepository interface {
	Create(ctx context.Context, response *model.Response) (*model.Response, error)
	Get(ctx context.Context, id int64) (*model.Response, error)
	List(ctx context.Context) ([]*model.Response, error)
	Update(ctx context.Context, response *model.Response) (*model.Response, error)
	Delete(ctx context.Context, id int64) error
}

// RiskResponseRepository defines the interface for RiskResponse data access
type RiskResponseRepository interface {
	Link(ctx context.Context, riskID, responseID int64) error
	Unlink(ctx context.Context, riskID, responseID int64) error
	GetResponsesByRisk(ctx context.Context, riskID int64) ([]*model.Response, error)
	GetResponsesByRisks(ctx context.Context, riskIDs []int64) (map[int64][]*model.Response, error)
	GetRisksByResponse(ctx context.Context, responseID int64) ([]*model.Risk, error)
	DeleteByResponse(ctx context.Context, responseID int64) error
	DeleteByRisk(ctx context.Context, riskID int64) error
}
