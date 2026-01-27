package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	gqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type GraphQLResponse struct {
	Data   json.RawMessage          `json:"data,omitempty"`
	Errors []map[string]interface{} `json:"errors,omitempty"`
}

func setupTestServer(t *testing.T) (*httptest.Server, *memory.Repository, *usecase.UseCases) {
	t.Helper()

	repo := memory.New()
	uc := usecase.New(repo)

	resolver := gqlctrl.NewResolver(repo, uc)
	schema := gqlctrl.NewExecutableSchema(gqlctrl.Config{Resolvers: resolver})
	gqlHandler := handler.NewDefaultServer(schema)

	srv, err := httpctrl.New(gqlHandler, httpctrl.WithGraphiQL(false))
	if err != nil {
		t.Fatalf("failed to create http server: %v", err)
	}
	testServer := httptest.NewServer(srv)

	return testServer, repo, uc
}

func executeGraphQL(t *testing.T, serverURL string, query string, variables map[string]interface{}) *GraphQLResponse {
	t.Helper()

	reqBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	resp, err := http.Post(serverURL+"/graphql", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var gqlResp GraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	return &gqlResp
}

func TestResponseQueriesHTTP(t *testing.T) {
	t.Run("responses query returns all responses via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create test responses
		resp1, err := uc.Response.CreateResponse(ctx, "Test Response 1", "Description 1", []string{"U1"}, "", types.ResponseStatusTodo, []int64{})
		if err != nil {
			t.Fatalf("failed to create response 1: %v", err)
		}

		resp2, err := uc.Response.CreateResponse(ctx, "Test Response 2", "Description 2", []string{"U2"}, "", types.ResponseStatusInProgress, []int64{})
		if err != nil {
			t.Fatalf("failed to create response 2: %v", err)
		}

		// Execute GraphQL query
		query := `
			query {
				responses {
					id
					title
					description
					status
				}
			}
		`

		gqlResp := executeGraphQL(t, testServer.URL, query, nil)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			Responses []struct {
				ID          int    `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Status      string `json:"status"`
			} `json:"responses"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		if len(data.Responses) != 2 {
			t.Errorf("expected 2 responses, got %d", len(data.Responses))
		}

		// Verify response data
		var foundResp1, foundResp2 bool
		for _, r := range data.Responses {
			if r.ID == int(resp1.ID) {
				foundResp1 = true
				if r.Title != "Test Response 1" {
					t.Errorf("expected title 'Test Response 1', got %s", r.Title)
				}
				if r.Status != "TODO" {
					t.Errorf("expected status TODO, got %s", r.Status)
				}
			}
			if r.ID == int(resp2.ID) {
				foundResp2 = true
				if r.Title != "Test Response 2" {
					t.Errorf("expected title 'Test Response 2', got %s", r.Title)
				}
				if r.Status != "IN_PROGRESS" {
					t.Errorf("expected status IN_PROGRESS, got %s", r.Status)
				}
			}
		}

		if !foundResp1 || !foundResp2 {
			t.Error("not all created responses were returned by query")
		}
	})

	t.Run("response query returns specific response via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		created, err := uc.Response.CreateResponse(ctx, "Specific Response", "Test description", []string{"U123"}, "https://example.com", types.ResponseStatusBacklog, []int64{})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		query := `
			query GetResponse($id: Int!) {
				response(id: $id) {
					id
					title
					description
					url
					status
				}
			}
		`

		variables := map[string]interface{}{
			"id": int(created.ID),
		}

		gqlResp := executeGraphQL(t, testServer.URL, query, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			Response struct {
				ID          int     `json:"id"`
				Title       string  `json:"title"`
				Description string  `json:"description"`
				URL         *string `json:"url"`
				Status      string  `json:"status"`
			} `json:"response"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		if data.Response.ID != int(created.ID) {
			t.Errorf("expected ID %d, got %d", created.ID, data.Response.ID)
		}
		if data.Response.Title != "Specific Response" {
			t.Errorf("expected title 'Specific Response', got %s", data.Response.Title)
		}
		if data.Response.Description != "Test description" {
			t.Errorf("expected description 'Test description', got %s", data.Response.Description)
		}
		if data.Response.URL == nil || *data.Response.URL != "https://example.com" {
			t.Errorf("expected URL 'https://example.com', got %v", data.Response.URL)
		}
		if data.Response.Status != "BACKLOG" {
			t.Errorf("expected status BACKLOG, got %s", data.Response.Status)
		}
	})

	t.Run("response query returns error for non-existent ID via HTTP", func(t *testing.T) {
		testServer, _, _ := setupTestServer(t)
		defer testServer.Close()

		query := `
			query GetResponse($id: Int!) {
				response(id: $id) {
					id
					title
				}
			}
		`

		variables := map[string]interface{}{
			"id": 99999,
		}

		gqlResp := executeGraphQL(t, testServer.URL, query, variables)

		if len(gqlResp.Errors) == 0 {
			t.Error("expected GraphQL error for non-existent response")
		}
	})
}

func TestResponseMutationsHTTP(t *testing.T) {
	t.Run("createResponse mutation creates new response via HTTP", func(t *testing.T) {
		testServer, _, _ := setupTestServer(t)
		defer testServer.Close()

		mutation := `
			mutation CreateResponse($input: CreateResponseInput!) {
				createResponse(input: $input) {
					id
					title
					description
					url
					status
					responders {
						id
					}
				}
			}
		`

		variables := map[string]interface{}{
			"input": map[string]interface{}{
				"title":        "New Response",
				"description":  "Test creating response",
				"responderIDs": []string{"U111", "U222"},
				"url":          "https://test.com",
				"status":       "TODO",
				"riskIDs":      []int{},
			},
		}

		gqlResp := executeGraphQL(t, testServer.URL, mutation, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			CreateResponse struct {
				ID          int    `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				URL         string `json:"url"`
				Status      string `json:"status"`
				Responders  []struct {
					ID string `json:"id"`
				} `json:"responders"`
			} `json:"createResponse"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		if data.CreateResponse.ID == 0 {
			t.Error("expected non-zero ID")
		}
		if data.CreateResponse.Title != "New Response" {
			t.Errorf("expected title 'New Response', got %s", data.CreateResponse.Title)
		}
		if data.CreateResponse.Description != "Test creating response" {
			t.Errorf("expected description 'Test creating response', got %s", data.CreateResponse.Description)
		}
		if len(data.CreateResponse.Responders) != 2 {
			t.Errorf("expected 2 responders, got %d", len(data.CreateResponse.Responders))
		}
		if data.CreateResponse.URL != "https://test.com" {
			t.Errorf("expected URL 'https://test.com', got %s", data.CreateResponse.URL)
		}
		if data.CreateResponse.Status != "TODO" {
			t.Errorf("expected status TODO, got %s", data.CreateResponse.Status)
		}
	})

	t.Run("updateResponse mutation updates existing response via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create initial response
		created, err := uc.Response.CreateResponse(ctx, "Original", "Original desc", []string{"U1"}, "", types.ResponseStatusTodo, []int64{})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		mutation := `
			mutation UpdateResponse($input: UpdateResponseInput!) {
				updateResponse(input: $input) {
					id
					title
					description
					url
					status
					responders {
						id
					}
				}
			}
		`

		variables := map[string]interface{}{
			"input": map[string]interface{}{
				"id":           int(created.ID),
				"title":        "Updated Title",
				"description":  "Updated description",
				"responderIDs": []string{"U2", "U3"},
				"url":          "https://updated.com",
				"status":       "COMPLETED",
				"riskIDs":      []int{},
			},
		}

		gqlResp := executeGraphQL(t, testServer.URL, mutation, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			UpdateResponse struct {
				ID          int    `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				URL         string `json:"url"`
				Status      string `json:"status"`
				Responders  []struct {
					ID string `json:"id"`
				} `json:"responders"`
			} `json:"updateResponse"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		if data.UpdateResponse.ID != int(created.ID) {
			t.Errorf("expected ID %d, got %d", created.ID, data.UpdateResponse.ID)
		}
		if data.UpdateResponse.Title != "Updated Title" {
			t.Errorf("expected title 'Updated Title', got %s", data.UpdateResponse.Title)
		}
		if data.UpdateResponse.Description != "Updated description" {
			t.Errorf("expected description 'Updated description', got %s", data.UpdateResponse.Description)
		}
		if len(data.UpdateResponse.Responders) != 2 {
			t.Errorf("expected 2 responders, got %d", len(data.UpdateResponse.Responders))
		}
		if data.UpdateResponse.URL != "https://updated.com" {
			t.Errorf("expected URL 'https://updated.com', got %s", data.UpdateResponse.URL)
		}
		if data.UpdateResponse.Status != "COMPLETED" {
			t.Errorf("expected status COMPLETED, got %s", data.UpdateResponse.Status)
		}
	})

	t.Run("deleteResponse mutation removes response via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create response
		created, err := uc.Response.CreateResponse(ctx, "To Delete", "Will be deleted", []string{"U1"}, "", types.ResponseStatusAbandoned, []int64{})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		mutation := `
			mutation DeleteResponse($id: Int!) {
				deleteResponse(id: $id)
			}
		`

		variables := map[string]interface{}{
			"id": int(created.ID),
		}

		gqlResp := executeGraphQL(t, testServer.URL, mutation, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			DeleteResponse bool `json:"deleteResponse"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		if !data.DeleteResponse {
			t.Error("expected deleteResponse=true")
		}

		// Verify deletion
		query := `
			query GetResponse($id: Int!) {
				response(id: $id) {
					id
				}
			}
		`

		verifyVars := map[string]interface{}{
			"id": int(created.ID),
		}

		verifyResp := executeGraphQL(t, testServer.URL, query, verifyVars)
		if len(verifyResp.Errors) == 0 {
			t.Error("expected error when querying deleted response")
		}
	})
}

func TestRiskResponsesFieldResolverHTTP(t *testing.T) {
	t.Run("Risk.responses field returns related responses via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create a risk
		risk, err := uc.Risk.CreateRisk(ctx, "Test Risk", "Risk description", []types.CategoryID{"cat-1"}, "Impact description", types.LikelihoodID("likelihood-1"), types.ImpactID("impact-1"), []types.TeamID{"team-1"}, []string{"U1"}, "Detection indicators")
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		// Create responses linked to this risk
		resp1, err := uc.Response.CreateResponse(ctx, "Response 1", "First response", []string{"U1"}, "", types.ResponseStatusTodo, []int64{risk.ID})
		if err != nil {
			t.Fatalf("failed to create response 1: %v", err)
		}

		resp2, err := uc.Response.CreateResponse(ctx, "Response 2", "Second response", []string{"U2"}, "", types.ResponseStatusInProgress, []int64{risk.ID})
		if err != nil {
			t.Fatalf("failed to create response 2: %v", err)
		}

		// Create unrelated response
		_, err = uc.Response.CreateResponse(ctx, "Unrelated", "Not linked", []string{"U3"}, "", types.ResponseStatusBacklog, []int64{})
		if err != nil {
			t.Fatalf("failed to create unrelated response: %v", err)
		}

		// Query the risk with responses field
		query := `
			query GetRisk($id: Int!) {
				risk(id: $id) {
					id
					name
					responses {
						id
						title
						status
					}
				}
			}
		`

		variables := map[string]interface{}{
			"id": int(risk.ID),
		}

		gqlResp := executeGraphQL(t, testServer.URL, query, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			Risk struct {
				ID        int    `json:"id"`
				Name      string `json:"name"`
				Responses []struct {
					ID     int    `json:"id"`
					Title  string `json:"title"`
					Status string `json:"status"`
				} `json:"responses"`
			} `json:"risk"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		// Should only return the 2 linked responses
		if len(data.Risk.Responses) != 2 {
			t.Errorf("expected 2 responses, got %d", len(data.Risk.Responses))
		}

		// Verify correct responses are returned
		var foundResp1, foundResp2 bool
		for _, r := range data.Risk.Responses {
			if r.ID == int(resp1.ID) {
				foundResp1 = true
				if r.Title != "Response 1" {
					t.Errorf("expected title 'Response 1', got %s", r.Title)
				}
			}
			if r.ID == int(resp2.ID) {
				foundResp2 = true
				if r.Title != "Response 2" {
					t.Errorf("expected title 'Response 2', got %s", r.Title)
				}
			}
		}

		if !foundResp1 || !foundResp2 {
			t.Error("not all linked responses were returned")
		}
	})

	t.Run("responsesByRisk query returns responses for specific risk via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create a risk
		risk, err := uc.Risk.CreateRisk(ctx, "Risk for Query Test", "Description", []types.CategoryID{"cat-1"}, "Impact", types.LikelihoodID("likelihood-1"), types.ImpactID("impact-1"), []types.TeamID{"team-1"}, []string{"U1"}, "Indicators")
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		// Create responses for this risk
		_, err = uc.Response.CreateResponse(ctx, "Response A", "Desc A", []string{"U1"}, "", types.ResponseStatusTodo, []int64{risk.ID})
		if err != nil {
			t.Fatalf("failed to create response A: %v", err)
		}

		_, err = uc.Response.CreateResponse(ctx, "Response B", "Desc B", []string{"U2"}, "", types.ResponseStatusCompleted, []int64{risk.ID})
		if err != nil {
			t.Fatalf("failed to create response B: %v", err)
		}

		query := `
			query GetResponsesByRisk($riskID: Int!) {
				responsesByRisk(riskID: $riskID) {
					id
					title
					status
				}
			}
		`

		variables := map[string]interface{}{
			"riskID": int(risk.ID),
		}

		gqlResp := executeGraphQL(t, testServer.URL, query, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			ResponsesByRisk []struct {
				ID     int    `json:"id"`
				Title  string `json:"title"`
				Status string `json:"status"`
			} `json:"responsesByRisk"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		if len(data.ResponsesByRisk) != 2 {
			t.Errorf("expected 2 responses, got %d", len(data.ResponsesByRisk))
		}
	})

	t.Run("Risk.responses returns empty array when no responses linked via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create a risk with no responses
		risk, err := uc.Risk.CreateRisk(ctx, "Risk Without Responses", "No responses yet", []types.CategoryID{"cat-1"}, "Impact", types.LikelihoodID("likelihood-1"), types.ImpactID("impact-1"), []types.TeamID{"team-1"}, []string{"U1"}, "Indicators")
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		query := `
			query GetRisk($id: Int!) {
				risk(id: $id) {
					id
					name
					responses {
						id
						title
					}
				}
			}
		`

		variables := map[string]interface{}{
			"id": int(risk.ID),
		}

		gqlResp := executeGraphQL(t, testServer.URL, query, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			Risk struct {
				ID        int    `json:"id"`
				Name      string `json:"name"`
				Responses []struct {
					ID    int    `json:"id"`
					Title string `json:"title"`
				} `json:"responses"`
			} `json:"risk"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		// Should return empty array, not nil
		if data.Risk.Responses == nil {
			t.Error("expected empty array, got nil")
		}
		if len(data.Risk.Responses) != 0 {
			t.Errorf("expected 0 responses, got %d", len(data.Risk.Responses))
		}
	})
}

func TestSlackUserFieldResolversHTTP(t *testing.T) {
	t.Run("Response.responders field returns Slack user info via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create response with responders
		resp, err := uc.Response.CreateResponse(ctx, "Test Response", "Description", []string{"U001", "U002"}, "", types.ResponseStatusTodo, []int64{})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		query := `
			query GetResponse($id: Int!) {
				response(id: $id) {
					id
					responderIDs
					responders {
						id
					}
				}
			}
		`

		variables := map[string]interface{}{
			"id": int(resp.ID),
		}

		gqlResp := executeGraphQL(t, testServer.URL, query, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			Response struct {
				ID           int      `json:"id"`
				ResponderIDs []string `json:"responderIDs"`
				Responders   []struct {
					ID string `json:"id"`
				} `json:"responders"`
			} `json:"response"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		// Verify responderIDs
		if len(data.Response.ResponderIDs) != 2 {
			t.Errorf("expected 2 responderIDs, got %d", len(data.Response.ResponderIDs))
		}

		// Verify responders field resolver works
		if len(data.Response.Responders) != 2 {
			t.Errorf("expected 2 responders, got %d", len(data.Response.Responders))
		}

		// Verify IDs match
		expectedIDs := map[string]bool{"U001": true, "U002": true}
		for _, responder := range data.Response.Responders {
			if !expectedIDs[responder.ID] {
				t.Errorf("unexpected responder ID: %s", responder.ID)
			}
		}
	})

	t.Run("Risk.assignees field returns Slack user info via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create risk with assignees
		risk, err := uc.Risk.CreateRisk(ctx, "Test Risk", "Description", []types.CategoryID{"cat-1"}, "Impact", types.LikelihoodID("likelihood-1"), types.ImpactID("impact-1"), []types.TeamID{"team-1"}, []string{"U101", "U102", "U103"}, "Indicators")
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		query := `
			query GetRisk($id: Int!) {
				risk(id: $id) {
					id
					assigneeIDs
					assignees {
						id
					}
				}
			}
		`

		variables := map[string]interface{}{
			"id": int(risk.ID),
		}

		gqlResp := executeGraphQL(t, testServer.URL, query, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			Risk struct {
				ID          int      `json:"id"`
				AssigneeIDs []string `json:"assigneeIDs"`
				Assignees   []struct {
					ID string `json:"id"`
				} `json:"assignees"`
			} `json:"risk"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		// Verify assigneeIDs
		if len(data.Risk.AssigneeIDs) != 3 {
			t.Errorf("expected 3 assigneeIDs, got %d", len(data.Risk.AssigneeIDs))
		}

		// Verify assignees field resolver works
		if len(data.Risk.Assignees) != 3 {
			t.Errorf("expected 3 assignees, got %d", len(data.Risk.Assignees))
		}

		// Verify IDs match
		expectedIDs := map[string]bool{"U101": true, "U102": true, "U103": true}
		for _, assignee := range data.Risk.Assignees {
			if !expectedIDs[assignee.ID] {
				t.Errorf("unexpected assignee ID: %s", assignee.ID)
			}
		}
	})

	t.Run("Responders and Assignees return empty arrays when no users specified via HTTP", func(t *testing.T) {
		testServer, _, uc := setupTestServer(t)
		defer testServer.Close()

		ctx := context.Background()

		// Create response with no responders
		resp, err := uc.Response.CreateResponse(ctx, "No Responders", "Description", []string{}, "", types.ResponseStatusTodo, []int64{})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		// Create risk with no assignees
		risk, err := uc.Risk.CreateRisk(ctx, "No Assignees", "Description", []types.CategoryID{"cat-1"}, "Impact", types.LikelihoodID("likelihood-1"), types.ImpactID("impact-1"), []types.TeamID{"team-1"}, []string{}, "Indicators")
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		query := `
			query GetData($responseID: Int!, $riskID: Int!) {
				response(id: $responseID) {
					responders {
						id
					}
				}
				risk(id: $riskID) {
					assignees {
						id
					}
				}
			}
		`

		variables := map[string]interface{}{
			"responseID": int(resp.ID),
			"riskID":     int(risk.ID),
		}

		gqlResp := executeGraphQL(t, testServer.URL, query, variables)

		if len(gqlResp.Errors) > 0 {
			t.Fatalf("GraphQL errors: %v", gqlResp.Errors)
		}

		var data struct {
			Response struct {
				Responders []struct {
					ID string `json:"id"`
				} `json:"responders"`
			} `json:"response"`
			Risk struct {
				Assignees []struct {
					ID string `json:"id"`
				} `json:"assignees"`
			} `json:"risk"`
		}

		if err := json.Unmarshal(gqlResp.Data, &data); err != nil {
			t.Fatalf("failed to unmarshal data: %v", err)
		}

		// Verify empty arrays
		if data.Response.Responders == nil {
			t.Error("expected empty array for responders, got nil")
		}
		if len(data.Response.Responders) != 0 {
			t.Errorf("expected 0 responders, got %d", len(data.Response.Responders))
		}

		if data.Risk.Assignees == nil {
			t.Error("expected empty array for assignees, got nil")
		}
		if len(data.Risk.Assignees) != 0 {
			t.Errorf("expected 0 assignees, got %d", len(data.Risk.Assignees))
		}
	})
}
