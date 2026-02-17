package usecase

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

// BuildThreadedMarkdown is exported for testing
var BuildThreadedMarkdown = buildThreadedMarkdown

// BuildSlackSourceURLs is exported for testing
var BuildSlackSourceURLs = buildSlackSourceURLs

// Type aliases for testing
type SlackMessage = slackmodel.Message
type SlackChannel = model.SlackChannel
