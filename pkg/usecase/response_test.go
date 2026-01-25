package usecase_test

import (
	"context"
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestResponseUseCase_CreateResponse(t *testing.T) {
	ctx := context.Background()

	t.Run("successful creation", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewResponseUseCase(repo)
		response, err := uc.CreateResponse(ctx, "Test Response", "Description", []string{"U123"}, "https://example.com", types.ResponseStatusTodo, nil)
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		if response.ID == 0 {
			t.Error("expected non-zero ID")
		}
		if response.Title != "Test Response" {
			t.Errorf("expected title='Test Response', got %s", response.Title)
		}
		if response.Status != types.ResponseStatusTodo {
			t.Errorf("expected status=%s, got %s", types.ResponseStatusTodo, response.Status)
		}
	})

	t.Run("empty title fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewResponseUseCase(repo)
		_, err := uc.CreateResponse(ctx, "", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)
		if err == nil {
			t.Error("expected error for empty title")
		}
	})

	t.Run("invalid status fails", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewResponseUseCase(repo)
		_, err := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatus("invalid"), nil)
		if err == nil {
			t.Error("expected error for invalid status")
		}
	})

	t.Run("default status to backlog", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewResponseUseCase(repo)
		response, err := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", "", nil)
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		if response.Status != types.ResponseStatusBacklog {
			t.Errorf("expected default status=%s, got %s", types.ResponseStatusBacklog, response.Status)
		}
	})

	t.Run("with risk IDs", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewResponseUseCase(repo)
		// Create a risk first
		risk, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Test Risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		response, err := uc.CreateResponse(ctx, "Linked Response", "Description", []string{"U123"}, "", types.ResponseStatusTodo, []int64{risk.ID})
		if err != nil {
			t.Fatalf("failed to create response with risk IDs: %v", err)
		}

		// Verify the link
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 1 || responses[0].ID != response.ID {
			t.Error("response should be linked to risk")
		}
	})

	t.Run("rollback on link failure", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewResponseUseCase(repo)
		// Try to create response with non-existent risk ID
		nonExistentRiskID := int64(999999)
		_, err := uc.CreateResponse(ctx, "Test Response", "Description", []string{"U123"}, "", types.ResponseStatusTodo, []int64{nonExistentRiskID})
		if err == nil {
			t.Fatal("expected error when linking to non-existent risk")
		}

		// Verify the response was NOT created (rollback succeeded)
		allResponses, err := repo.Response().List(ctx)
		if err != nil {
			t.Fatalf("failed to list responses: %v", err)
		}

		// Check that no response with the title exists
		for _, r := range allResponses {
			if r.Title == "Test Response" {
				t.Error("response should have been rolled back after link failure")
			}
		}
	})
}

func TestResponseUseCase_UpdateResponse(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewResponseUseCase(repo)

	t.Run("successful update", func(t *testing.T) {
		// Create response
		created, err := uc.CreateResponse(ctx, "Original", "Original Description", []string{"U123"}, "", types.ResponseStatusBacklog, nil)
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		// Update response
		title := "Updated"
		desc := "Updated Description"
		url := "https://new.com"
		status := types.ResponseStatusInProgress
		updated, err := uc.UpdateResponse(ctx, created.ID, &title, &desc, []string{"U456"}, &url, &status, nil)
		if err != nil {
			t.Fatalf("failed to update response: %v", err)
		}

		if updated.Title != "Updated" {
			t.Errorf("expected title='Updated', got %s", updated.Title)
		}
		if updated.Status != types.ResponseStatusInProgress {
			t.Errorf("expected status=%s, got %s", types.ResponseStatusInProgress, updated.Status)
		}
		if !updated.CreatedAt.Equal(created.CreatedAt) {
			t.Error("CreatedAt should be preserved")
		}
		// UpdatedAt should be >= CreatedAt (could be same if operations are very fast)
		if updated.UpdatedAt.Before(created.CreatedAt) {
			t.Error("UpdatedAt should not be before CreatedAt")
		}
	})

	t.Run("partial update - status only", func(t *testing.T) {
		created, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)
		status := types.ResponseStatusInProgress
		updated, err := uc.UpdateResponse(ctx, created.ID, nil, nil, nil, nil, &status, nil)
		if err != nil {
			t.Fatalf("failed to update response: %v", err)
		}

		if updated.Title != "Test" {
			t.Error("title should not change")
		}
		if updated.Status != types.ResponseStatusInProgress {
			t.Errorf("expected status=%s, got %s", types.ResponseStatusInProgress, updated.Status)
		}
	})

	t.Run("empty title fails", func(t *testing.T) {
		created, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)
		emptyTitle := ""
		_, err := uc.UpdateResponse(ctx, created.ID, &emptyTitle, nil, nil, nil, nil, nil)
		if err == nil {
			t.Error("expected error for empty title")
		}
	})

	t.Run("invalid status fails", func(t *testing.T) {
		created, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)
		invalidStatus := types.ResponseStatus("invalid")
		_, err := uc.UpdateResponse(ctx, created.ID, nil, nil, nil, nil, &invalidStatus, nil)
		if err == nil {
			t.Error("expected error for invalid status")
		}
	})

	t.Run("non-existent response fails", func(t *testing.T) {
		status := types.ResponseStatusTodo
		_, err := uc.UpdateResponse(ctx, 99999, nil, nil, nil, nil, &status, nil)
		if err == nil {
			t.Error("expected error for non-existent response")
		}
	})

	t.Run("update risk associations", func(t *testing.T) {
		// Create risks
		risk1, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Risk 1"})
		risk2, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Risk 2"})
		risk3, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Risk 3"})

		// Create response linked to risk1 and risk2
		response, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, []int64{risk1.ID, risk2.ID})

		// Update to link to risk2 and risk3 (remove risk1, add risk3)
		riskIDs := []int64{risk2.ID, risk3.ID}
		_, err := uc.UpdateResponse(ctx, response.ID, nil, nil, nil, nil, nil, riskIDs)
		if err != nil {
			t.Fatalf("failed to update response risk associations: %v", err)
		}

		// Verify risk1 is no longer linked
		responses1, _ := repo.RiskResponse().GetResponsesByRisk(ctx, risk1.ID)
		if len(responses1) != 0 {
			t.Error("risk1 should not be linked")
		}

		// Verify risk2 is still linked
		responses2, _ := repo.RiskResponse().GetResponsesByRisk(ctx, risk2.ID)
		if len(responses2) != 1 || responses2[0].ID != response.ID {
			t.Error("risk2 should be linked")
		}

		// Verify risk3 is now linked
		responses3, _ := repo.RiskResponse().GetResponsesByRisk(ctx, risk3.ID)
		if len(responses3) != 1 || responses3[0].ID != response.ID {
			t.Error("risk3 should be linked")
		}
	})
}

func TestResponseUseCase_DeleteResponse(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewResponseUseCase(repo)

	t.Run("successful deletion", func(t *testing.T) {
		created, err := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)
		if err != nil {
			t.Fatalf("failed to create response: %v", err)
		}

		err = uc.DeleteResponse(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to delete response: %v", err)
		}

		// Verify deletion
		_, err = uc.GetResponse(ctx, created.ID)
		if err == nil {
			t.Error("expected error when getting deleted response")
		}
	})

	t.Run("delete with linked risks", func(t *testing.T) {
		// Create risk and response
		risk, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Test Risk"})
		response, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, []int64{risk.ID})

		// Delete response
		err := uc.DeleteResponse(ctx, response.ID)
		if err != nil {
			t.Fatalf("failed to delete response: %v", err)
		}

		// Verify link is also deleted
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}
		if len(responses) != 0 {
			t.Error("expected no responses after deletion")
		}
	})

	t.Run("non-existent response fails", func(t *testing.T) {
		err := uc.DeleteResponse(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent response")
		}
	})
}

func TestResponseUseCase_GetResponse(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewResponseUseCase(repo)

	t.Run("successful get", func(t *testing.T) {
		created, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)

		retrieved, err := uc.GetResponse(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get response: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID=%d, got %d", created.ID, retrieved.ID)
		}
	})

	t.Run("non-existent response fails", func(t *testing.T) {
		_, err := uc.GetResponse(ctx, 99999)
		if err == nil {
			t.Error("expected error for non-existent response")
		}
	})
}

func TestResponseUseCase_ListResponses(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewResponseUseCase(repo)

	t.Run("list all responses", func(t *testing.T) {
		// Create multiple responses
		_, _ = uc.CreateResponse(ctx, "Response 1", "Description 1", []string{"U123"}, "", types.ResponseStatusTodo, nil)
		_, _ = uc.CreateResponse(ctx, "Response 2", "Description 2", []string{"U456"}, "", types.ResponseStatusInProgress, nil)

		responses, err := uc.ListResponses(ctx)
		if err != nil {
			t.Fatalf("failed to list responses: %v", err)
		}

		if len(responses) < 2 {
			t.Errorf("expected at least 2 responses, got %d", len(responses))
		}
	})
}

func TestResponseUseCase_LinkResponseToRisk(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewResponseUseCase(repo)

	t.Run("successful link", func(t *testing.T) {
		risk, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Test Risk"})
		response, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)

		err := uc.LinkResponseToRisk(ctx, response.ID, risk.ID)
		if err != nil {
			t.Fatalf("failed to link response to risk: %v", err)
		}

		// Verify link
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}
		if len(responses) != 1 || responses[0].ID != response.ID {
			t.Error("expected response to be linked to risk")
		}
	})

	t.Run("non-existent response fails", func(t *testing.T) {
		risk, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Test Risk"})
		err := uc.LinkResponseToRisk(ctx, 99999, risk.ID)
		if err == nil {
			t.Error("expected error for non-existent response")
		}
	})

	t.Run("non-existent risk fails", func(t *testing.T) {
		response, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)
		err := uc.LinkResponseToRisk(ctx, response.ID, 99999)
		if err == nil {
			t.Error("expected error for non-existent risk")
		}
	})
}

func TestResponseUseCase_UnlinkResponseFromRisk(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewResponseUseCase(repo)

	t.Run("successful unlink", func(t *testing.T) {
		risk, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Test Risk"})
		response, _ := uc.CreateResponse(ctx, "Test", "Description", []string{"U123"}, "", types.ResponseStatusTodo, []int64{risk.ID})

		err := uc.UnlinkResponseFromRisk(ctx, response.ID, risk.ID)
		if err != nil {
			t.Fatalf("failed to unlink response from risk: %v", err)
		}

		// Verify unlink
		responses, err := repo.RiskResponse().GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}
		if len(responses) != 0 {
			t.Error("expected no responses after unlink")
		}
	})

	t.Run("non-existent link fails", func(t *testing.T) {
		err := uc.UnlinkResponseFromRisk(ctx, 99999, 88888)
		if err == nil {
			t.Error("expected error for non-existent link")
		}
	})
}

func TestResponseUseCase_GetResponsesByRisk(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewResponseUseCase(repo)

	t.Run("get responses for risk", func(t *testing.T) {
		risk, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Test Risk"})
		response1, _ := uc.CreateResponse(ctx, "Response 1", "Description", []string{"U123"}, "", types.ResponseStatusTodo, []int64{risk.ID})
		response2, _ := uc.CreateResponse(ctx, "Response 2", "Description", []string{"U456"}, "", types.ResponseStatusInProgress, []int64{risk.ID})

		responses, err := uc.GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 2 {
			t.Errorf("expected 2 responses, got %d", len(responses))
		}

		responseIDs := make(map[int64]bool)
		for _, r := range responses {
			responseIDs[r.ID] = true
		}
		if !responseIDs[response1.ID] || !responseIDs[response2.ID] {
			t.Error("not all response IDs found in result")
		}
	})

	t.Run("empty list for risk with no responses", func(t *testing.T) {
		risk, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Lonely Risk"})

		responses, err := uc.GetResponsesByRisk(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to get responses by risk: %v", err)
		}

		if len(responses) != 0 {
			t.Errorf("expected 0 responses, got %d", len(responses))
		}
	})
}

func TestResponseUseCase_GetRisksByResponse(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewResponseUseCase(repo)

	t.Run("get risks for response", func(t *testing.T) {
		risk1, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Risk 1"})
		risk2, _ := repo.Risk().Create(ctx, &model.Risk{Name: "Risk 2"})
		response, _ := uc.CreateResponse(ctx, "Shared Response", "Description", []string{"U123"}, "", types.ResponseStatusTodo, []int64{risk1.ID, risk2.ID})

		risks, err := uc.GetRisksByResponse(ctx, response.ID)
		if err != nil {
			t.Fatalf("failed to get risks by response: %v", err)
		}

		if len(risks) != 2 {
			t.Errorf("expected 2 risks, got %d", len(risks))
		}

		riskIDs := make(map[int64]bool)
		for _, r := range risks {
			riskIDs[r.ID] = true
		}
		if !riskIDs[risk1.ID] || !riskIDs[risk2.ID] {
			t.Error("not all risk IDs found in result")
		}
	})

	t.Run("empty list for response with no risks", func(t *testing.T) {
		response, _ := uc.CreateResponse(ctx, "Lonely Response", "Description", []string{"U123"}, "", types.ResponseStatusTodo, nil)

		risks, err := uc.GetRisksByResponse(ctx, response.ID)
		if err != nil {
			t.Fatalf("failed to get risks by response: %v", err)
		}

		if len(risks) != 0 {
			t.Errorf("expected 0 risks, got %d", len(risks))
		}
	})
}
