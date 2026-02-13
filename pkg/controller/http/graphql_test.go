package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/vektah/gqlparser/v2/gqlerror"

	gqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

const testWorkspaceID = "test-ws"

// setupGraphQLServer creates a test GraphQL server with HTTP handler
func setupGraphQLServer(repo interfaces.Repository) (http.Handler, error) {
	uc := usecase.New(repo, nil)
	resolver := gqlctrl.NewResolver(repo, uc)
	srv := handler.NewDefaultServer(
		gqlctrl.NewExecutableSchema(gqlctrl.Config{Resolvers: resolver}),
	)

	// Configure error presenter with stack traces (same as serve.go)
	srv.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
		gqlErr := graphql.DefaultErrorPresenter(ctx, err)
		wrappedErr := goerr.Wrap(err, "GraphQL error")
		logging.Default().Error("GraphQL error occurred", "error", wrappedErr)
		return gqlErr
	})

	// Configure panic handler (same as serve.go)
	srv.SetRecoverFunc(func(ctx context.Context, panicValue interface{}) error {
		var panicErr error
		switch e := panicValue.(type) {
		case error:
			panicErr = e
		case string:
			panicErr = goerr.New(e)
		default:
			panicErr = goerr.New("panic occurred", goerr.V("panic", panicValue))
		}

		wrappedErr := goerr.Wrap(panicErr, "GraphQL panic")
		logging.Default().Error("GraphQL panic occurred", "error", wrappedErr)
		return wrappedErr
	})

	// Wrap with dataloader middleware (same as serve.go)
	gqlHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loaders := gqlctrl.NewDataLoaders(repo)
		ctx := gqlctrl.WithDataLoaders(r.Context(), loaders)
		srv.ServeHTTP(w, r.WithContext(ctx))
	})

	return httpctrl.New(gqlHandler)
}

// GraphQL request/response structures
type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data,omitempty"`
	Errors []graphQLError  `json:"errors,omitempty"`
}

type graphQLError struct {
	Message string        `json:"message"`
	Path    []interface{} `json:"path,omitempty"`
}

// executeGraphQLRequest sends a GraphQL request through the HTTP handler
func executeGraphQLRequest(t *testing.T, handler http.Handler, query string, variables map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()

	reqBody := graphQLRequest{
		Query:     query,
		Variables: variables,
	}

	body, err := json.Marshal(reqBody)
	gt.NoError(t, err).Required()

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

// parseGraphQLResponse parses the GraphQL response
func parseGraphQLResponse(t *testing.T, rec *httptest.ResponseRecorder) *graphQLResponse {
	t.Helper()

	var resp graphQLResponse
	gt.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp)).Required()

	return &resp
}

func TestGraphQLHandler_CasesQuery(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	t.Run("empty cases list", func(t *testing.T) {
		query := `
			query($workspaceId: String!) {
				cases(workspaceId: $workspaceId) {
					id
					title
					description
					assigneeIDs
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()
	})

	t.Run("cases list with data", func(t *testing.T) {
		ctx := context.Background()

		// Create test cases
		case1 := &model.Case{
			Title:       "Test Case 1",
			Description: "Test case description 1",
			AssigneeIDs: []string{"U001", "U002"},
		}

		case2 := &model.Case{
			Title:       "Test Case 2",
			Description: "Test case description 2",
			AssigneeIDs: []string{"U003"},
		}

		createdCase1, err := repo.Case().Create(ctx, testWorkspaceID, case1)
		gt.NoError(t, err).Required()

		createdCase2, err := repo.Case().Create(ctx, testWorkspaceID, case2)
		gt.NoError(t, err).Required()

		query := `
			query($workspaceId: String!) {
				cases(workspaceId: $workspaceId) {
					id
					title
					description
					assigneeIDs
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		// Parse cases from response
		var result struct {
			Cases []struct {
				ID          int      `json:"id"`
				Title       string   `json:"title"`
				Description string   `json:"description"`
				AssigneeIDs []string `json:"assigneeIDs"`
			} `json:"cases"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Array(t, result.Cases).Length(2)

		// Verify cases are returned (order may vary based on implementation)
		foundCase1 := false
		foundCase2 := false
		for _, c := range result.Cases {
			if int64(c.ID) == createdCase1.ID && c.Title == "Test Case 1" {
				foundCase1 = true
				gt.Value(t, c.Description).Equal("Test case description 1")
				gt.Array(t, c.AssigneeIDs).Length(2)
			}
			if int64(c.ID) == createdCase2.ID && c.Title == "Test Case 2" {
				foundCase2 = true
				gt.Value(t, c.Description).Equal("Test case description 2")
				gt.Array(t, c.AssigneeIDs).Length(1)
			}
		}

		gt.Bool(t, foundCase1).True()
		gt.Bool(t, foundCase2).True()
	})
}

func TestGraphQLHandler_CaseQuery(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	testCase := &model.Case{
		Title:       "Test Case for Query",
		Description: "Test case description for single query",
		AssigneeIDs: []string{"U123"},
	}

	createdCase, err := repo.Case().Create(ctx, testWorkspaceID, testCase)
	gt.NoError(t, err).Required()

	t.Run("query case by ID", func(t *testing.T) {
		query := `
			query($workspaceId: String!, $id: Int!) {
				case(workspaceId: $workspaceId, id: $id) {
					id
					title
					description
					assigneeIDs
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          int(createdCase.ID),
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			Case struct {
				ID          int      `json:"id"`
				Title       string   `json:"title"`
				Description string   `json:"description"`
				AssigneeIDs []string `json:"assigneeIDs"`
			} `json:"case"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.Case.Title).Equal("Test Case for Query")

		gt.Value(t, result.Case.Description).Equal("Test case description for single query")

		gt.Array(t, result.Case.AssigneeIDs).Length(1)
		gt.Value(t, result.Case.AssigneeIDs[0]).Equal("U123")
	})

	t.Run("query non-existent case", func(t *testing.T) {
		query := `
			query($workspaceId: String!, $id: Int!) {
				case(workspaceId: $workspaceId, id: $id) {
					id
					title
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          999999999,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)
	})
}

func TestGraphQLHandler_CreateCaseMutation(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	t.Run("create new case", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $input: CreateCaseInput!) {
				createCase(workspaceId: $workspaceId, input: $input) {
					id
					title
					description
					assigneeIDs
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"title":       "New Case from Mutation",
				"description": "Case created via GraphQL mutation",
				"assigneeIDs": []string{"U001", "U002"},
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			CreateCase struct {
				ID          int      `json:"id"`
				Title       string   `json:"title"`
				Description string   `json:"description"`
				AssigneeIDs []string `json:"assigneeIDs"`
			} `json:"createCase"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.CreateCase.Title).Equal("New Case from Mutation")

		gt.Value(t, result.CreateCase.Description).Equal("Case created via GraphQL mutation")

		gt.Array(t, result.CreateCase.AssigneeIDs).Length(2)

		// Verify the case was actually saved to repository
		ctx := context.Background()
		savedCase, err := repo.Case().Get(ctx, testWorkspaceID, int64(result.CreateCase.ID))
		gt.NoError(t, err).Required()

		gt.Value(t, savedCase.Title).Equal("New Case from Mutation")

		gt.Array(t, savedCase.AssigneeIDs).Length(2)
	})

	t.Run("create case with missing required fields", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $input: CreateCaseInput!) {
				createCase(workspaceId: $workspaceId, input: $input) {
					id
					title
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"title": "", // Empty title should fail validation
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)
	})
}

func TestGraphQLHandler_UpdateCaseMutation(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	// Create a case first
	caseToUpdate := &model.Case{
		Title:       "Original Title",
		Description: "Original Description",
		AssigneeIDs: []string{"U001"},
	}
	createdCase, err := repo.Case().Create(ctx, testWorkspaceID, caseToUpdate)
	gt.NoError(t, err).Required()

	t.Run("update existing case", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $input: UpdateCaseInput!) {
				updateCase(workspaceId: $workspaceId, input: $input) {
					id
					title
					description
					assigneeIDs
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"id":          createdCase.ID,
				"title":       "Updated Title",
				"description": "Updated Description",
				"assigneeIDs": []string{"U001", "U002", "U003"},
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			UpdateCase struct {
				ID          int      `json:"id"`
				Title       string   `json:"title"`
				Description string   `json:"description"`
				AssigneeIDs []string `json:"assigneeIDs"`
			} `json:"updateCase"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.UpdateCase.Title).Equal("Updated Title")

		gt.Value(t, result.UpdateCase.Description).Equal("Updated Description")

		gt.Array(t, result.UpdateCase.AssigneeIDs).Length(3)

		// Verify the case was actually updated in repository
		updatedCase, err := repo.Case().Get(ctx, testWorkspaceID, createdCase.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, updatedCase.Title).Equal("Updated Title")

		gt.Array(t, updatedCase.AssigneeIDs).Length(3)
	})

	t.Run("update non-existent case", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $input: UpdateCaseInput!) {
				updateCase(workspaceId: $workspaceId, input: $input) {
					id
					title
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"id":          99999,
				"title":       "Should Fail",
				"description": "This should fail",
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)
	})
}

func TestGraphQLHandler_DeleteCaseMutation(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	t.Run("delete existing case", func(t *testing.T) {
		// Create a case to delete
		caseToDelete := &model.Case{
			Title:       "Case to Delete",
			Description: "This case will be deleted",
		}
		createdCase, err := repo.Case().Create(ctx, testWorkspaceID, caseToDelete)
		gt.NoError(t, err).Required()

		mutation := `
			mutation($workspaceId: String!, $id: Int!) {
				deleteCase(workspaceId: $workspaceId, id: $id)
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          createdCase.ID,
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			DeleteCase bool `json:"deleteCase"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Bool(t, result.DeleteCase).True()

		// Verify the case was actually deleted from repository
		_, err = repo.Case().Get(ctx, testWorkspaceID, createdCase.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("delete non-existent case", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $id: Int!) {
				deleteCase(workspaceId: $workspaceId, id: $id)
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          99999,
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)
	})

	t.Run("delete case also deletes associated actions", func(t *testing.T) {
		// Create a case
		caseWithActions := &model.Case{
			Title:       "Case with Actions",
			Description: "This case has actions",
		}
		createdCase, err := repo.Case().Create(ctx, testWorkspaceID, caseWithActions)
		gt.NoError(t, err).Required()

		// Create actions associated with this case
		action1 := &model.Action{
			CaseID:      createdCase.ID,
			Title:       "Action 1",
			Description: "First action",
		}
		action2 := &model.Action{
			CaseID:      createdCase.ID,
			Title:       "Action 2",
			Description: "Second action",
		}

		_, err = repo.Action().Create(ctx, testWorkspaceID, action1)
		gt.NoError(t, err).Required()

		_, err = repo.Action().Create(ctx, testWorkspaceID, action2)
		gt.NoError(t, err).Required()

		mutation := `
			mutation($workspaceId: String!, $id: Int!) {
				deleteCase(workspaceId: $workspaceId, id: $id)
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          createdCase.ID,
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		// Verify associated actions were also deleted
		actions, err := repo.Action().GetByCase(ctx, testWorkspaceID, createdCase.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(0)
	})
}

func TestGraphQLHandler_FrontendCasesQuery(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	// Create test case with all fields
	testCase := &model.Case{
		Title:          "Frontend Test Case",
		Description:    "Test case for frontend query",
		AssigneeIDs:    []string{"U001"},
		SlackChannelID: "C12345",
	}

	createdCase, err := repo.Case().Create(ctx, testWorkspaceID, testCase)
	gt.NoError(t, err).Required()

	t.Run("frontend GET_CASES query format", func(t *testing.T) {
		// This mimics the query structure used by the frontend
		// Note: assignees field is omitted in this test as it requires SlackUser data
		query := `
			query GetCases($workspaceId: String!) {
				cases(workspaceId: $workspaceId) {
					id
					title
					description
					assigneeIDs
					slackChannelID
					createdAt
					updatedAt
					fields {
						fieldId
						value
					}
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		// Verify the response structure matches frontend expectations
		var result struct {
			Cases []struct {
				ID             int      `json:"id"`
				Title          string   `json:"title"`
				Description    string   `json:"description"`
				AssigneeIDs    []string `json:"assigneeIDs"`
				SlackChannelID string   `json:"slackChannelID"`
				Fields         []struct {
					FieldId string `json:"fieldId"`
					Value   any    `json:"value"`
				} `json:"fields"`
			} `json:"cases"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Array(t, result.Cases).Length(1).Required()

		c := result.Cases[0]
		gt.Value(t, int64(c.ID)).Equal(createdCase.ID)
		gt.Value(t, c.Title).Equal("Frontend Test Case")
		gt.Value(t, c.SlackChannelID).Equal("C12345")
	})

	t.Run("frontend GET_CASE query format", func(t *testing.T) {
		// This mimics the query structure used by the frontend for single case
		// Note: assignees, actions, knowledges fields are omitted in this test
		query := `
			query GetCase($workspaceId: String!, $id: Int!) {
				case(workspaceId: $workspaceId, id: $id) {
					id
					title
					description
					assigneeIDs
					slackChannelID
					createdAt
					updatedAt
					fields {
						fieldId
						value
					}
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          int(createdCase.ID),
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()
	})

	t.Run("verify slackChannelName field exists", func(t *testing.T) {
		query := `
			query($workspaceId: String!) {
				cases(workspaceId: $workspaceId) {
					id
					slackChannelName
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)
		gt.Value(t, resp.Data).NotNil()
	})

	t.Run("verify fieldID is case-sensitive (fieldId is correct)", func(t *testing.T) {
		// This should fail because fieldID (capital D) is incorrect
		query := `
			query($workspaceId: String!) {
				cases(workspaceId: $workspaceId) {
					id
					fields {
						fieldID
						value
					}
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)

		// Check that the error message mentions the invalid field
		if len(resp.Errors) > 0 {
			gt.Value(t, resp.Errors[0].Message).Equal(`Cannot query field "fieldID" on type "FieldValue". Did you mean "fieldId"?`)
		}
	})
}

func TestGraphQLHandler_FrontendKnowledgesQuery(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	// Create a test case first
	testCase := &model.Case{
		Title:       "Test Case for Knowledge",
		Description: "Case for knowledge testing",
		AssigneeIDs: []string{"U001"},
	}

	createdCase, err := repo.Case().Create(ctx, testWorkspaceID, testCase)
	gt.NoError(t, err).Required()

	// Create test knowledge linked to the case
	now := time.Now()
	testKnowledge := &model.Knowledge{
		ID:        model.NewKnowledgeID(),
		CaseID:    createdCase.ID,
		SourceID:  "source-001",
		SourceURL: "https://example.com/source",
		Title:     "Test Knowledge Title",
		Summary:   "Test knowledge summary",
		SourcedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	createdKnowledge, err := repo.Knowledge().Create(ctx, testWorkspaceID, testKnowledge)
	gt.NoError(t, err).Required()

	t.Run("frontend GET_KNOWLEDGES query format", func(t *testing.T) {
		// This mimics the query structure used by the frontend
		query := `
			query GetKnowledges($workspaceId: String!) {
				knowledges(workspaceId: $workspaceId, limit: 100, offset: 0) {
					items {
						id
						caseID
						case {
							id
							title
							description
						}
						sourceID
						sourceURL
						title
						summary
						sourcedAt
						createdAt
						updatedAt
					}
					totalCount
					hasMore
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			Knowledges struct {
				Items []struct {
					ID     string `json:"id"`
					CaseID int    `json:"caseID"`
					Case   *struct {
						ID          int    `json:"id"`
						Title       string `json:"title"`
						Description string `json:"description"`
					} `json:"case"`
					SourceID  string `json:"sourceID"`
					SourceURL string `json:"sourceURL"`
					Title     string `json:"title"`
					Summary   string `json:"summary"`
				} `json:"items"`
				TotalCount int  `json:"totalCount"`
				HasMore    bool `json:"hasMore"`
			} `json:"knowledges"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Array(t, result.Knowledges.Items).Length(1).Required()

		k := result.Knowledges.Items[0]
		gt.Value(t, k.ID).Equal(string(createdKnowledge.ID))
		gt.Value(t, int64(k.CaseID)).Equal(createdCase.ID)
		gt.Value(t, k.Title).Equal("Test Knowledge Title")
		gt.Value(t, k.Case).NotNil()
		if k.Case != nil {
			gt.Value(t, int64(k.Case.ID)).Equal(createdCase.ID)
			gt.Value(t, k.Case.Title).Equal("Test Case for Knowledge")
		}
	})

	t.Run("frontend GET_KNOWLEDGE query format", func(t *testing.T) {
		// This mimics the query structure used by the frontend for single knowledge
		query := `
			query GetKnowledge($workspaceId: String!, $id: String!) {
				knowledge(workspaceId: $workspaceId, id: $id) {
					id
					caseID
					case {
						id
						title
						description
					}
					sourceID
					sourceURL
					title
					summary
					sourcedAt
					createdAt
					updatedAt
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          string(createdKnowledge.ID),
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			Knowledge struct {
				ID     string `json:"id"`
				CaseID int    `json:"caseID"`
				Case   *struct {
					ID          int    `json:"id"`
					Title       string `json:"title"`
					Description string `json:"description"`
				} `json:"case"`
				Title string `json:"title"`
			} `json:"knowledge"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.Knowledge.ID).Equal(string(createdKnowledge.ID))

		gt.Value(t, int64(result.Knowledge.CaseID)).Equal(createdCase.ID)

		gt.Value(t, result.Knowledge.Case).NotNil()
	})

	t.Run("verify riskID field does not exist", func(t *testing.T) {
		// This should fail because riskID is not a valid field
		query := `
			query($workspaceId: String!) {
				knowledges(workspaceId: $workspaceId, limit: 10, offset: 0) {
					items {
						id
						riskID
					}
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)

		// Check that the error message mentions the invalid field
		if len(resp.Errors) > 0 {
			gt.Value(t, resp.Errors[0].Message).Equal(`Cannot query field "riskID" on type "Knowledge". Did you mean "caseID"?`)
		}
	})
}

func TestGraphQLHandler_ErrorHandling(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	t.Run("invalid GraphQL syntax", func(t *testing.T) {
		query := `
			query($workspaceId: String!) {
				cases(workspaceId: $workspaceId) {
					invalid syntax here
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)
	})

	t.Run("unknown field", func(t *testing.T) {
		query := `
			query($workspaceId: String!) {
				cases(workspaceId: $workspaceId) {
					id
					nonExistentField
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)
	})

	t.Run("missing required variable", func(t *testing.T) {
		query := `
			query($workspaceId: String!, $id: Int!) {
				case(workspaceId: $workspaceId, id: $id) {
					id
				}
			}
		`

		// Don't provide the required $id variable
		rec := executeGraphQLRequest(t, handler, query, nil)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)
	})
}

func TestGraphQLHandler_ActionMutations(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	// Create a case for actions to belong to
	testCase := &model.Case{
		Title:       "Test Case for Actions",
		Description: "Case for action tests",
	}
	createdCase, err := repo.Case().Create(ctx, testWorkspaceID, testCase)
	gt.NoError(t, err).Required()

	t.Run("create new action", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $input: CreateActionInput!) {
				createAction(workspaceId: $workspaceId, input: $input) {
					id
					caseID
					title
					description
					status
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"caseID":      createdCase.ID,
				"title":       "New Action",
				"description": "Action created via GraphQL",
				"status":      "TODO",
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			CreateAction struct {
				ID          int    `json:"id"`
				CaseID      int    `json:"caseID"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Status      string `json:"status"`
			} `json:"createAction"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.CreateAction.Title).Equal("New Action")

		gt.Value(t, result.CreateAction.CaseID).Equal(int(createdCase.ID))

		// Verify the action was actually saved to repository
		savedAction, err := repo.Action().Get(ctx, testWorkspaceID, int64(result.CreateAction.ID))
		gt.NoError(t, err).Required()

		gt.Value(t, savedAction.Title).Equal("New Action")

		gt.Value(t, savedAction.CaseID).Equal(createdCase.ID)
	})

	t.Run("create action without caseID fails", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $input: CreateActionInput!) {
				createAction(workspaceId: $workspaceId, input: $input) {
					id
					title
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"title":       "Action without Case",
				"description": "Should fail",
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		resp := parseGraphQLResponse(t, rec)

		gt.Number(t, len(resp.Errors)).GreaterOrEqual(1)
	})

	t.Run("update action", func(t *testing.T) {
		// Create an action to update
		actionToUpdate := &model.Action{
			CaseID:      createdCase.ID,
			Title:       "Original Action Title",
			Description: "Original Description",
			Status:      "TODO",
		}
		createdAction, err := repo.Action().Create(ctx, testWorkspaceID, actionToUpdate)
		gt.NoError(t, err).Required()

		mutation := `
			mutation($workspaceId: String!, $input: UpdateActionInput!) {
				updateAction(workspaceId: $workspaceId, input: $input) {
					id
					caseID
					title
					description
					status
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"id":          createdAction.ID,
				"title":       "Updated Action Title",
				"description": "Updated Description",
				"status":      "IN_PROGRESS",
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			UpdateAction struct {
				ID          int    `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Status      string `json:"status"`
			} `json:"updateAction"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.UpdateAction.Title).Equal("Updated Action Title")

		gt.Value(t, result.UpdateAction.Status).Equal("IN_PROGRESS")

		// Verify the action was actually updated in repository
		updatedAction, err := repo.Action().Get(ctx, testWorkspaceID, createdAction.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, updatedAction.Title).Equal("Updated Action Title")
	})

	t.Run("delete action", func(t *testing.T) {
		// Create an action to delete
		actionToDelete := &model.Action{
			CaseID:      createdCase.ID,
			Title:       "Action to Delete",
			Description: "This action will be deleted",
		}
		createdAction, err := repo.Action().Create(ctx, testWorkspaceID, actionToDelete)
		gt.NoError(t, err).Required()

		mutation := `
			mutation($workspaceId: String!, $id: Int!) {
				deleteAction(workspaceId: $workspaceId, id: $id)
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          createdAction.ID,
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			DeleteAction bool `json:"deleteAction"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Bool(t, result.DeleteAction).True()

		// Verify the action was actually deleted from repository
		_, err = repo.Action().Get(ctx, testWorkspaceID, createdAction.ID)
		gt.Value(t, err).NotNil()
	})
}

func TestGraphQLHandler_NoopMutation(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	t.Run("noop mutation returns true", func(t *testing.T) {
		mutation := `
			mutation {
				noop
			}
		`

		rec := executeGraphQLRequest(t, handler, mutation, nil)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			Noop bool `json:"noop"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Bool(t, result.Noop).True()
	})
}

func TestGraphQLHandler_SlackUsersQuery(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	t.Run("empty slack users list", func(t *testing.T) {
		query := `
			query {
				slackUsers {
					id
					name
					realName
					imageUrl
				}
			}
		`

		rec := executeGraphQLRequest(t, handler, query, nil)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			SlackUsers []struct {
				ID       string  `json:"id"`
				Name     string  `json:"name"`
				RealName string  `json:"realName"`
				ImageURL *string `json:"imageUrl"`
			} `json:"slackUsers"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Array(t, result.SlackUsers).Length(0)
	})

	t.Run("slack users list with data", func(t *testing.T) {
		users := []*model.SlackUser{
			{
				ID:       "U001",
				Name:     "john.doe",
				RealName: "John Doe",
				ImageURL: "https://example.com/john.png",
			},
			{
				ID:       "U002",
				Name:     "jane.doe",
				RealName: "Jane Doe",
				ImageURL: "",
			},
		}
		gt.NoError(t, repo.SlackUser().SaveMany(ctx, users)).Required()

		query := `
			query {
				slackUsers {
					id
					name
					realName
					imageUrl
				}
			}
		`

		rec := executeGraphQLRequest(t, handler, query, nil)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			SlackUsers []struct {
				ID       string  `json:"id"`
				Name     string  `json:"name"`
				RealName string  `json:"realName"`
				ImageURL *string `json:"imageUrl"`
			} `json:"slackUsers"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Array(t, result.SlackUsers).Length(2)

		foundJohn := false
		foundJane := false
		for _, u := range result.SlackUsers {
			if u.ID == "U001" {
				foundJohn = true
				gt.Value(t, u.Name).Equal("john.doe")
				gt.Value(t, u.RealName).Equal("John Doe")
				gt.Value(t, u.ImageURL).NotNil()
				gt.Value(t, *u.ImageURL).Equal("https://example.com/john.png")
			}
			if u.ID == "U002" {
				foundJane = true
				gt.Value(t, u.Name).Equal("jane.doe")
				gt.Value(t, u.RealName).Equal("Jane Doe")
				// ImageURL should be nil when empty
				gt.Value(t, u.ImageURL).Nil()
			}
		}

		gt.Bool(t, foundJohn).True()
		gt.Bool(t, foundJane).True()
	})
}

func TestGraphQLHandler_SourceQueries(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	t.Run("empty sources list", func(t *testing.T) {
		query := `
			query($workspaceId: String!) {
				sources(workspaceId: $workspaceId) {
					id
					name
					sourceType
					description
					enabled
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			Sources []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"sources"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Array(t, result.Sources).Length(0)
	})

	t.Run("create and query notion source", func(t *testing.T) {
		source := &model.Source{
			Name:        "Test Notion Source",
			SourceType:  model.SourceTypeNotionDB,
			Description: "Test notion database source",
			Enabled:     true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID:    "db-123",
				DatabaseTitle: "My Database",
				DatabaseURL:   "https://notion.so/db-123",
			},
		}

		createdSource, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		// Test sources list query
		query := `
			query($workspaceId: String!) {
				sources(workspaceId: $workspaceId) {
					id
					name
					sourceType
					description
					enabled
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		var listResult struct {
			Sources []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				SourceType  string `json:"sourceType"`
				Description string `json:"description"`
				Enabled     bool   `json:"enabled"`
			} `json:"sources"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &listResult)).Required()

		gt.Number(t, len(listResult.Sources)).GreaterOrEqual(1)

		found := false
		for _, s := range listResult.Sources {
			if s.ID == string(createdSource.ID) {
				found = true
				gt.Value(t, s.Name).Equal("Test Notion Source")
				gt.Value(t, s.SourceType).Equal("NOTION_DB")
				gt.Bool(t, s.Enabled).True()
			}
		}
		gt.Bool(t, found).True()

		// Test single source query
		singleQuery := `
			query($workspaceId: String!, $id: String!) {
				source(workspaceId: $workspaceId, id: $id) {
					id
					name
					sourceType
					description
					enabled
				}
			}
		`

		singleVariables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          string(createdSource.ID),
		}

		rec = executeGraphQLRequest(t, handler, singleQuery, singleVariables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp = parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		var singleResult struct {
			Source struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				SourceType  string `json:"sourceType"`
				Description string `json:"description"`
				Enabled     bool   `json:"enabled"`
			} `json:"source"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &singleResult)).Required()

		gt.Value(t, singleResult.Source.ID).Equal(string(createdSource.ID))
		gt.Value(t, singleResult.Source.Name).Equal("Test Notion Source")
	})
}

func TestGraphQLHandler_SourceMutations(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	t.Run("create notion source", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $input: CreateNotionDBSourceInput!) {
				createNotionDBSource(workspaceId: $workspaceId, input: $input) {
					id
					name
					sourceType
					description
					enabled
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"name":        "My Notion DB",
				"description": "A test notion source",
				"databaseID":  "notion-db-001",
				"enabled":     true,
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			CreateNotionDBSource struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				SourceType  string `json:"sourceType"`
				Description string `json:"description"`
				Enabled     bool   `json:"enabled"`
			} `json:"createNotionDBSource"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.CreateNotionDBSource.Name).Equal("My Notion DB")
		gt.Value(t, result.CreateNotionDBSource.SourceType).Equal("NOTION_DB")
		gt.Bool(t, result.CreateNotionDBSource.Enabled).True()
	})

	t.Run("create slack source", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $input: CreateSlackSourceInput!) {
				createSlackSource(workspaceId: $workspaceId, input: $input) {
					id
					name
					sourceType
					description
					enabled
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"name":        "My Slack Source",
				"description": "A test slack source",
				"channelIDs":  []string{"C001", "C002"},
				"enabled":     true,
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			CreateSlackSource struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				SourceType  string `json:"sourceType"`
				Description string `json:"description"`
				Enabled     bool   `json:"enabled"`
			} `json:"createSlackSource"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.CreateSlackSource.Name).Equal("My Slack Source")
		gt.Value(t, result.CreateSlackSource.SourceType).Equal("SLACK")
		gt.Bool(t, result.CreateSlackSource.Enabled).True()
	})

	t.Run("update source", func(t *testing.T) {
		source := &model.Source{
			Name:        "Source to Update",
			SourceType:  model.SourceTypeNotionDB,
			Description: "Original description",
			Enabled:     true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID:    "db-update",
				DatabaseTitle: "Update DB",
				DatabaseURL:   "https://notion.so/db-update",
			},
		}
		createdSource, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		mutation := `
			mutation($workspaceId: String!, $input: UpdateSourceInput!) {
				updateSource(workspaceId: $workspaceId, input: $input) {
					id
					name
					description
					enabled
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"id":          string(createdSource.ID),
				"name":        "Updated Source Name",
				"description": "Updated description",
				"enabled":     false,
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		var result struct {
			UpdateSource struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				Enabled     bool   `json:"enabled"`
			} `json:"updateSource"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.UpdateSource.Name).Equal("Updated Source Name")
		gt.Value(t, result.UpdateSource.Description).Equal("Updated description")
		gt.Bool(t, result.UpdateSource.Enabled).False()
	})

	t.Run("update slack source with channels", func(t *testing.T) {
		source := &model.Source{
			Name:        "Slack Source to Update",
			SourceType:  model.SourceTypeSlack,
			Description: "Original slack source",
			Enabled:     true,
			SlackConfig: &model.SlackConfig{
				Channels: []model.SlackChannel{
					{ID: "C001", Name: "C001"},
				},
			},
		}
		createdSource, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		mutation := `
			mutation($workspaceId: String!, $input: UpdateSlackSourceInput!) {
				updateSlackSource(workspaceId: $workspaceId, input: $input) {
					id
					name
					description
					enabled
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"input": map[string]interface{}{
				"id":         string(createdSource.ID),
				"name":       "Updated Slack Source",
				"channelIDs": []string{"C003", "C004"},
			},
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		var result struct {
			UpdateSlackSource struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				Enabled     bool   `json:"enabled"`
			} `json:"updateSlackSource"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Value(t, result.UpdateSlackSource.Name).Equal("Updated Slack Source")

		// Verify the channels were updated in repository
		updatedSource, err := repo.Source().Get(ctx, testWorkspaceID, createdSource.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, updatedSource.SlackConfig).NotNil()
		gt.Array(t, updatedSource.SlackConfig.Channels).Length(2)
	})

	t.Run("delete source", func(t *testing.T) {
		source := &model.Source{
			Name:        "Source to Delete",
			SourceType:  model.SourceTypeNotionDB,
			Description: "Will be deleted",
			Enabled:     true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID:    "db-delete",
				DatabaseTitle: "Delete DB",
				DatabaseURL:   "https://notion.so/db-delete",
			},
		}
		createdSource, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		mutation := `
			mutation($workspaceId: String!, $id: String!) {
				deleteSource(workspaceId: $workspaceId, id: $id)
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"id":          string(createdSource.ID),
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		var result struct {
			DeleteSource bool `json:"deleteSource"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Bool(t, result.DeleteSource).True()

		// Verify the source was actually deleted
		_, err = repo.Source().Get(ctx, testWorkspaceID, createdSource.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("validate notion DB without service", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $databaseID: String!) {
				validateNotionDB(workspaceId: $workspaceId, databaseID: $databaseID) {
					valid
					errorMessage
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"databaseID":  "test-db-id",
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		var result struct {
			ValidateNotionDB struct {
				Valid        bool    `json:"valid"`
				ErrorMessage *string `json:"errorMessage"`
			} `json:"validateNotionDB"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		// Without Notion service configured, validation should return not valid
		gt.Bool(t, result.ValidateNotionDB.Valid).False()
		gt.Value(t, result.ValidateNotionDB.ErrorMessage).NotNil()
	})

	t.Run("validate notion DB with empty ID", func(t *testing.T) {
		mutation := `
			mutation($workspaceId: String!, $databaseID: String!) {
				validateNotionDB(workspaceId: $workspaceId, databaseID: $databaseID) {
					valid
					errorMessage
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"databaseID":  "",
		}

		rec := executeGraphQLRequest(t, handler, mutation, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		var result struct {
			ValidateNotionDB struct {
				Valid        bool    `json:"valid"`
				ErrorMessage *string `json:"errorMessage"`
			} `json:"validateNotionDB"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Bool(t, result.ValidateNotionDB.Valid).False()
	})
}

func TestGraphQLHandler_ActionsByCaseQuery(t *testing.T) {
	repo := memory.New()
	handler, err := setupGraphQLServer(repo)
	gt.NoError(t, err).Required()

	ctx := context.Background()

	// Create a case
	testCase := &model.Case{
		Title:       "Case with Multiple Actions",
		Description: "Testing actions query",
	}
	createdCase, err := repo.Case().Create(ctx, testWorkspaceID, testCase)
	gt.NoError(t, err).Required()

	// Create multiple actions for this case
	action1 := &model.Action{
		CaseID:      createdCase.ID,
		Title:       "First Action",
		Description: "Action 1",
		Status:      "TODO",
	}
	action2 := &model.Action{
		CaseID:      createdCase.ID,
		Title:       "Second Action",
		Description: "Action 2",
		Status:      "IN_PROGRESS",
	}
	action3 := &model.Action{
		CaseID:      createdCase.ID,
		Title:       "Third Action",
		Description: "Action 3",
		Status:      "COMPLETED",
	}

	_, err = repo.Action().Create(ctx, testWorkspaceID, action1)
	gt.NoError(t, err).Required()
	_, err = repo.Action().Create(ctx, testWorkspaceID, action2)
	gt.NoError(t, err).Required()
	_, err = repo.Action().Create(ctx, testWorkspaceID, action3)
	gt.NoError(t, err).Required()

	t.Run("query actions by case ID", func(t *testing.T) {
		query := `
			query($workspaceId: String!, $caseID: Int!) {
				actionsByCase(workspaceId: $workspaceId, caseID: $caseID) {
					id
					caseID
					title
					description
					status
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"caseID":      createdCase.ID,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		gt.Value(t, resp.Data).NotNil().Required()

		var result struct {
			ActionsByCase []struct {
				ID          int    `json:"id"`
				CaseID      int    `json:"caseID"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Status      string `json:"status"`
			} `json:"actionsByCase"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Array(t, result.ActionsByCase).Length(3)

		// Verify all actions belong to the correct case
		for _, action := range result.ActionsByCase {
			gt.Value(t, action.CaseID).Equal(int(createdCase.ID))
		}
	})

	t.Run("query actions for non-existent case returns empty list", func(t *testing.T) {
		query := `
			query($workspaceId: String!, $caseID: Int!) {
				actionsByCase(workspaceId: $workspaceId, caseID: $caseID) {
					id
					title
				}
			}
		`

		variables := map[string]interface{}{
			"workspaceId": testWorkspaceID,
			"caseID":      99999,
		}

		rec := executeGraphQLRequest(t, handler, query, variables)

		gt.Value(t, rec.Code).Equal(http.StatusOK)

		resp := parseGraphQLResponse(t, rec)

		gt.Array(t, resp.Errors).Length(0)

		var result struct {
			ActionsByCase []struct {
				ID    int    `json:"id"`
				Title string `json:"title"`
			} `json:"actionsByCase"`
		}

		gt.NoError(t, json.Unmarshal(resp.Data, &result)).Required()

		gt.Array(t, result.ActionsByCase).Length(0)
	})
}
