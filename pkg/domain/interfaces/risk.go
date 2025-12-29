package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type RiskRepository interface {
	// Create creates a new risk with auto-generated ID
	Create(ctx context.Context, risk *model.Risk) (*model.Risk, error)

	// Get retrieves a risk by ID
	Get(ctx context.Context, id int64) (*model.Risk, error)

	// List retrieves all risks
	List(ctx context.Context) ([]*model.Risk, error)

	// Update updates an existing risk
	Update(ctx context.Context, risk *model.Risk) (*model.Risk, error)

	// Delete deletes a risk by ID
	Delete(ctx context.Context, id int64) error
}
