package graphql

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// toGraphQLSource converts domain Source to GraphQL Source
func toGraphQLSource(source *model.Source) (*graphql1.Source, error) {
	if source == nil {
		return nil, nil
	}

	sourceType, err := toGraphQLSourceType(source.SourceType)
	if err != nil {
		return nil, err
	}

	gqlSource := &graphql1.Source{
		ID:          string(source.ID),
		Name:        source.Name,
		SourceType:  sourceType,
		Description: source.Description,
		Enabled:     source.Enabled,
		CreatedAt:   source.CreatedAt,
		UpdatedAt:   source.UpdatedAt,
	}

	if source.NotionDBConfig != nil {
		gqlSource.Config = &graphql1.NotionDBConfig{
			DatabaseID:    source.NotionDBConfig.DatabaseID,
			DatabaseTitle: source.NotionDBConfig.DatabaseTitle,
			DatabaseURL:   source.NotionDBConfig.DatabaseURL,
		}
	}

	if source.SlackConfig != nil {
		channels := make([]*graphql1.SlackChannel, len(source.SlackConfig.Channels))
		for i, ch := range source.SlackConfig.Channels {
			channels[i] = &graphql1.SlackChannel{
				ID:   ch.ID,
				Name: ch.Name,
			}
		}
		gqlSource.Config = &graphql1.SlackConfig{
			Channels: channels,
		}
	}

	return gqlSource, nil
}

// toGraphQLSourceType converts domain SourceType to GraphQL SourceType
func toGraphQLSourceType(st model.SourceType) (graphql1.SourceType, error) {
	switch st {
	case model.SourceTypeNotionDB:
		return graphql1.SourceTypeNotionDb, nil
	case model.SourceTypeSlack:
		return graphql1.SourceTypeSLACk, nil
	default:
		return "", goerr.New("unsupported source type", goerr.V("sourceType", st))
	}
}

// toUseCaseCreateNotionDBSourceInput converts GraphQL input to UseCase input
func toUseCaseCreateNotionDBSourceInput(input graphql1.CreateNotionDBSourceInput) usecase.CreateNotionDBSourceInput {
	ucInput := usecase.CreateNotionDBSourceInput{
		DatabaseID: input.DatabaseID,
	}
	if input.Name != nil {
		ucInput.Name = *input.Name
	}
	if input.Description != nil {
		ucInput.Description = *input.Description
	}
	if input.Enabled != nil {
		ucInput.Enabled = *input.Enabled
	} else {
		ucInput.Enabled = true
	}
	return ucInput
}

// toUseCaseUpdateSourceInput converts GraphQL input to UseCase input
func toUseCaseUpdateSourceInput(input graphql1.UpdateSourceInput) usecase.UpdateSourceInput {
	return usecase.UpdateSourceInput{
		ID:          model.SourceID(input.ID),
		Name:        input.Name,
		Description: input.Description,
		Enabled:     input.Enabled,
	}
}

// toUseCaseCreateSlackSourceInput converts GraphQL input to UseCase input
func toUseCaseCreateSlackSourceInput(input graphql1.CreateSlackSourceInput) usecase.CreateSlackSourceInput {
	ucInput := usecase.CreateSlackSourceInput{
		ChannelIDs: input.ChannelIDs,
	}
	if input.Name != nil {
		ucInput.Name = *input.Name
	}
	if input.Description != nil {
		ucInput.Description = *input.Description
	}
	if input.Enabled != nil {
		ucInput.Enabled = *input.Enabled
	} else {
		ucInput.Enabled = true
	}
	return ucInput
}

// toUseCaseUpdateSlackSourceInput converts GraphQL input to UseCase input
func toUseCaseUpdateSlackSourceInput(input graphql1.UpdateSlackSourceInput) usecase.UpdateSlackSourceInput {
	return usecase.UpdateSlackSourceInput{
		ID:          model.SourceID(input.ID),
		Name:        input.Name,
		Description: input.Description,
		ChannelIDs:  input.ChannelIDs,
		Enabled:     input.Enabled,
	}
}
