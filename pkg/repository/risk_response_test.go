package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runRiskResponseRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Link creates risk-response association", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a risk
		risk, err := repo.Risk().Create(ctx, &model.Risk{
			Name:        "Test Risk",
			Description: "A test risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		// Create a response
		response, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Test Response",
			Description:  "A test response",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		// Link them
		err = repo.RiskResponse().Link(ctx, risk.ID, response.ID)
		if err != nil {
			t.Fatalf("failed to link risk and response: %v", err)
		}

		// Verify the link
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 1 {
			t.Errorf("expected 1 response, got %d", len(responses))
		}
		if len(responses) > 0 && responses[0].ID != response.ID {
			t.Errorf("expected response ID=%d, got %d", response.ID, responses[0].ID)
		}
	})

	t.Run("Link is idempotent", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Test Risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		response, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Test Response",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		// Link twice
		err = repo.RiskResponse().Link(ctx, risk.ID, response.ID)
		if err != nil {
			t.Fatalf("failed first link: %v", err)
		}

		err = repo.RiskResponse().Link(ctx, risk.ID, response.ID)
		if err != nil {
			t.Fatalf("failed second link: %v", err)
		}

		// Should still only have one link
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 1 {
			t.Errorf("expected 1 response after duplicate link, got %d", len(responses))
		}
	})

	t.Run("Unlink removes risk-response association", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Test Risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		response, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Test Response",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		// Link
		err = repo.RiskResponse().Link(ctx, risk.ID, response.ID)
		if err != nil {
			t.Fatalf("failed to link: %v", err)
		}

		// Unlink
		err = repo.RiskResponse().Unlink(ctx, risk.ID, response.ID)
		if err != nil {
			t.Fatalf("failed to unlink: %v", err)
		}

		// Verify unlink
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 0 {
			t.Errorf("expected 0 responses after unlink, got %d", len(responses))
		}
	})

	t.Run("Unlink returns error for non-existent link", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		err := repo.RiskResponse().Unlink(ctx, 99999, 88888)
		if err == nil {
			t.Error("expected error for non-existent link")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("GetResponsesByRisk returns multiple responses", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Test Risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		response1, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Response 1",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response 1: %v", err)
		}

		response2, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Response 2",
			ResponderIDs: []string{"U2"},
			Status:       types.ResponseStatusInProgress,
		})
		if err != nil {
			t.Fatalf("failed to create response 2: %v", err)
		}

		response3, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Response 3",
			ResponderIDs: []string{"U3"},
			Status:       types.ResponseStatusCompleted,
		})
		if err != nil {
			t.Fatalf("failed to create response 3: %v", err)
		}

		// Link all three responses to the risk
		err = repo.RiskResponse().Link(ctx, risk.ID, response1.ID)
		if err != nil {
			t.Fatalf("failed to link response 1: %v", err)
		}
		err = repo.RiskResponse().Link(ctx, risk.ID, response2.ID)
		if err != nil {
			t.Fatalf("failed to link response 2: %v", err)
		}
		err = repo.RiskResponse().Link(ctx, risk.ID, response3.ID)
		if err != nil {
			t.Fatalf("failed to link response 3: %v", err)
		}

		// Get all responses for the risk
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 3 {
			t.Errorf("expected 3 responses, got %d", len(responses))
		}

		// Verify all response IDs are present
		responseIDs := make(map[int64]bool)
		for _, r := range responses {
			responseIDs[r.ID] = true
		}

		if !responseIDs[response1.ID] || !responseIDs[response2.ID] || !responseIDs[response3.ID] {
			t.Errorf("not all response IDs found in result")
		}
	})

	t.Run("GetResponsesByRisks returns responses for multiple risks", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk1, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Risk 1",
		})
		if err != nil {
			t.Fatalf("failed to create risk 1: %v", err)
		}

		risk2, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Risk 2",
		})
		if err != nil {
			t.Fatalf("failed to create risk 2: %v", err)
		}

		response1, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Response 1",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response 1: %v", err)
		}

		response2, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Response 2",
			ResponderIDs: []string{"U2"},
			Status:       types.ResponseStatusInProgress,
		})
		if err != nil {
			t.Fatalf("failed to create response 2: %v", err)
		}

		// Link response1 to risk1, response2 to risk2
		err = repo.RiskResponse().Link(ctx, risk1.ID, response1.ID)
		if err != nil {
			t.Fatalf("failed to link response 1 to risk 1: %v", err)
		}
		err = repo.RiskResponse().Link(ctx, risk2.ID, response2.ID)
		if err != nil {
			t.Fatalf("failed to link response 2 to risk 2: %v", err)
		}

		// Get responses for both risks
		responsesMap, err := repo.RiskResponse().GetResponsesByRisks(ctx, []int64{risk1.ID, risk2.ID})
		if err != nil {
			t.Fatalf("failed to get responses by risks: %v", err)
		}

		if len(responsesMap) != 2 {
			t.Errorf("expected 2 entries in map, got %d", len(responsesMap))
		}

		if len(responsesMap[risk1.ID]) != 1 || responsesMap[risk1.ID][0].ID != response1.ID {
			t.Errorf("risk1 should have response1")
		}

		if len(responsesMap[risk2.ID]) != 1 || responsesMap[risk2.ID][0].ID != response2.ID {
			t.Errorf("risk2 should have response2")
		}
	})

	t.Run("GetRisksByResponse returns multiple risks", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk1, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Risk 1",
		})
		if err != nil {
			t.Fatalf("failed to create risk 1: %v", err)
		}

		risk2, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Risk 2",
		})
		if err != nil {
			t.Fatalf("failed to create risk 2: %v", err)
		}

		response, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Shared Response",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		// Link both risks to the response
		err = repo.RiskResponse().Link(ctx, risk1.ID, response.ID)
		if err != nil {
			t.Fatalf("failed to link risk 1: %v", err)
		}
		err = repo.RiskResponse().Link(ctx, risk2.ID, response.ID)
		if err != nil {
			t.Fatalf("failed to link risk 2: %v", err)
		}

		// Get all risks for the response
		risks, err := repo.RiskResponse().GetRisksByResponse(ctx, response.ID)
		if err != nil {
			t.Fatalf("failed to get risks by response: %v", err)
		}

		if len(risks) != 2 {
			t.Errorf("expected 2 risks, got %d", len(risks))
		}

		// Verify both risk IDs are present
		riskIDs := make(map[int64]bool)
		for _, r := range risks {
			riskIDs[r.ID] = true
		}

		if !riskIDs[risk1.ID] || !riskIDs[risk2.ID] {
			t.Errorf("not all risk IDs found in result")
		}
	})

	t.Run("DeleteByResponse removes all links for a response", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk1, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Risk 1",
		})
		if err != nil {
			t.Fatalf("failed to create risk 1: %v", err)
		}

		risk2, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Risk 2",
		})
		if err != nil {
			t.Fatalf("failed to create risk 2: %v", err)
		}

		response, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Response to Delete",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		// Link both risks
		err = repo.RiskResponse().Link(ctx, risk1.ID, response.ID)
		if err != nil {
			t.Fatalf("failed to link risk 1: %v", err)
		}
		err = repo.RiskResponse().Link(ctx, risk2.ID, response.ID)
		if err != nil {
			t.Fatalf("failed to link risk 2: %v", err)
		}

		// Delete all links for the response
		err = repo.RiskResponse().DeleteByResponse(ctx, response.ID)
		if err != nil {
			t.Fatalf("failed to delete by response: %v", err)
		}

		// Verify all links are gone
		risks, err := repo.RiskResponse().GetRisksByResponse(ctx, response.ID)
		if err != nil {
			t.Fatalf("failed to get risks by response: %v", err)
		}

		if len(risks) != 0 {
			t.Errorf("expected 0 risks after delete, got %d", len(risks))
		}
	})

	t.Run("DeleteByRisk removes all links for a risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Risk to Delete",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		response1, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Response 1",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response 1: %v", err)
		}

		response2, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Response 2",
			ResponderIDs: []string{"U2"},
			Status:       types.ResponseStatusInProgress,
		})
		if err != nil {
			t.Fatalf("failed to create response 2: %v", err)
		}

		// Link both responses
		err = repo.RiskResponse().Link(ctx, risk.ID, response1.ID)
		if err != nil {
			t.Fatalf("failed to link response 1: %v", err)
		}
		err = repo.RiskResponse().Link(ctx, risk.ID, response2.ID)
		if err != nil {
			t.Fatalf("failed to link response 2: %v", err)
		}

		// Delete all links for the risk
		err = repo.RiskResponse().DeleteByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to delete by risk: %v", err)
		}

		// Verify all links are gone
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 0 {
			t.Errorf("expected 0 responses after delete, got %d", len(responses))
		}
	})

	t.Run("GetResponsesByRisk returns empty for risk with no responses", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		risk, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Lonely Risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 0 {
			t.Errorf("expected 0 responses, got %d", len(responses))
		}
	})

	t.Run("GetRisksByResponse returns empty for response with no risks", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		response, err := repo.Response().Create(ctx, &model.Response{
			Title:        "Lonely Response",
			ResponderIDs: []string{"U1"},
			Status:       types.ResponseStatusTodo,
		})
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		risks, err := repo.RiskResponse().GetRisksByResponse(ctx, response.ID)
		if err != nil {
			t.Fatalf("failed to get risks by response: %v", err)
		}

		if len(risks) != 0 {
			t.Errorf("expected 0 risks, got %d", len(risks))
		}
	})
}

func TestMemoryRiskResponseRepository(t *testing.T) {
	runRiskResponseRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreRiskResponseRepository(t *testing.T) {
	runRiskResponseRepositoryTest(t, newFirestoreRepository)
}
