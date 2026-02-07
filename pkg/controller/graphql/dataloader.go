package graphql

import (
	"context"
	"fmt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
)

// DataLoaders holds all the data loaders used in the GraphQL resolvers
type DataLoaders struct {
	repo                   interfaces.Repository
	SlackUserLoader        *SlackUserLoader
	ActionLoader           *ActionLoader
	ActionsByCaseLoader    *ActionsByCaseLoader
	CaseLoader             *CaseLoader
	KnowledgeLoader        *KnowledgeLoader
	KnowledgesByCaseLoader *KnowledgesByCaseLoader
}

// NewDataLoaders creates a new DataLoaders instance
func NewDataLoaders(repo interfaces.Repository) *DataLoaders {
	return &DataLoaders{
		repo:                   repo,
		SlackUserLoader:        NewSlackUserLoader(repo),
		ActionLoader:           NewActionLoader(repo),
		ActionsByCaseLoader:    NewActionsByCaseLoader(repo),
		CaseLoader:             NewCaseLoader(repo),
		KnowledgeLoader:        NewKnowledgeLoader(repo),
		KnowledgesByCaseLoader: NewKnowledgesByCaseLoader(repo),
	}
}

// GetDataLoaders retrieves DataLoaders from context
func GetDataLoaders(ctx context.Context) *DataLoaders {
	loaders, ok := ctx.Value(dataLoadersKey).(*DataLoaders)
	if !ok {
		panic("dataloaders not found in context")
	}
	return loaders
}

type dataLoadersKeyType string

const dataLoadersKey dataLoadersKeyType = "dataloaders"

// WithDataLoaders adds DataLoaders to context
func WithDataLoaders(ctx context.Context, loaders *DataLoaders) context.Context {
	return context.WithValue(ctx, dataLoadersKey, loaders)
}

// SlackUserLoader loads Slack users by ID
type SlackUserLoader struct {
	repo interfaces.Repository
}

func NewSlackUserLoader(repo interfaces.Repository) *SlackUserLoader {
	return &SlackUserLoader{repo: repo}
}

func (l *SlackUserLoader) Load(ctx context.Context, ids []string) ([]*graphql1.SlackUser, error) {
	// Convert []string to []model.SlackUserID
	userIDs := make([]model.SlackUserID, len(ids))
	for i, id := range ids {
		userIDs[i] = model.SlackUserID(id)
	}

	users, err := l.repo.SlackUser().GetByIDs(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load slack users: %w", err)
	}

	userMap := make(map[string]*graphql1.SlackUser)
	for _, user := range users {
		imageURL := user.ImageURL
		var imageURLPtr *string
		if imageURL != "" {
			imageURLPtr = &imageURL
		}
		userMap[string(user.ID)] = &graphql1.SlackUser{
			ID:       string(user.ID),
			Name:     user.Name,
			RealName: user.RealName,
			ImageURL: imageURLPtr,
		}
	}

	// Filter out nil entries for IDs that don't exist in the repository.
	// The GraphQL schema declares assignees as [SlackUser!]! (non-null elements),
	// so returning nil elements would cause a marshaling error.
	result := make([]*graphql1.SlackUser, 0, len(ids))
	for _, id := range ids {
		if user, ok := userMap[id]; ok {
			result = append(result, user)
		}
	}

	return result, nil
}

// ActionLoader loads actions by ID
type ActionLoader struct {
	repo interfaces.Repository
}

func NewActionLoader(repo interfaces.Repository) *ActionLoader {
	return &ActionLoader{repo: repo}
}

func (l *ActionLoader) Load(ctx context.Context, ids []int64) ([]*model.Action, error) {
	// For now, load individually
	// Could be optimized with a batch Get method in the repository
	actions := make([]*model.Action, len(ids))
	for i, id := range ids {
		action, err := l.repo.Action().Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to load action %d: %w", id, err)
		}
		actions[i] = action
	}

	return actions, nil
}

// ActionsByCaseLoader loads actions by case ID
type ActionsByCaseLoader struct {
	repo interfaces.Repository
}

func NewActionsByCaseLoader(repo interfaces.Repository) *ActionsByCaseLoader {
	return &ActionsByCaseLoader{repo: repo}
}

func (l *ActionsByCaseLoader) Load(ctx context.Context, caseIDs []int64) (map[int64][]*model.Action, error) {
	actions, err := l.repo.Action().GetByCases(ctx, caseIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load actions by case: %w", err)
	}

	return actions, nil
}

// CaseLoader loads cases by ID
type CaseLoader struct {
	repo interfaces.Repository
}

func NewCaseLoader(repo interfaces.Repository) *CaseLoader {
	return &CaseLoader{repo: repo}
}

func (l *CaseLoader) Load(ctx context.Context, ids []int64) ([]*model.Case, error) {
	// For now, load individually
	// Could be optimized with a batch Get method in the repository
	cases := make([]*model.Case, len(ids))
	for i, id := range ids {
		c, err := l.repo.Case().Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to load case %d: %w", id, err)
		}
		cases[i] = c
	}

	return cases, nil
}

// KnowledgeLoader loads knowledge by ID
type KnowledgeLoader struct {
	repo interfaces.Repository
}

func NewKnowledgeLoader(repo interfaces.Repository) *KnowledgeLoader {
	return &KnowledgeLoader{repo: repo}
}

func (l *KnowledgeLoader) Load(ctx context.Context, ids []model.KnowledgeID) ([]*model.Knowledge, error) {
	// For now, load individually
	// Could be optimized with a batch Get method in the repository
	knowledges := make([]*model.Knowledge, len(ids))
	for i, id := range ids {
		k, err := l.repo.Knowledge().Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to load knowledge %s: %w", id, err)
		}
		knowledges[i] = k
	}

	return knowledges, nil
}

// KnowledgesByCaseLoader loads knowledges by case ID
type KnowledgesByCaseLoader struct {
	repo interfaces.Repository
}

func NewKnowledgesByCaseLoader(repo interfaces.Repository) *KnowledgesByCaseLoader {
	return &KnowledgesByCaseLoader{repo: repo}
}

func (l *KnowledgesByCaseLoader) Load(ctx context.Context, caseIDs []int64) (map[int64][]*model.Knowledge, error) {
	knowledges, err := l.repo.Knowledge().ListByCaseIDs(ctx, caseIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load knowledges by case: %w", err)
	}

	return knowledges, nil
}
