package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type riskResponseDocument struct {
	RiskID     int64     `firestore:"risk_id"`
	ResponseID int64     `firestore:"response_id"`
	CreatedAt  time.Time `firestore:"created_at"`
}

type riskResponseRepository struct {
	client           *firestore.Client
	responseRepo     *responseRepository
	riskRepo         *riskRepository
	collectionPrefix string
}

func newRiskResponseRepository(client *firestore.Client, responseRepo *responseRepository, riskRepo *riskRepository) *riskResponseRepository {
	return &riskResponseRepository{
		client:           client,
		responseRepo:     responseRepo,
		riskRepo:         riskRepo,
		collectionPrefix: "",
	}
}

func (r *riskResponseRepository) riskResponsesCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_risk_responses"
	}
	return "risk_responses"
}

func (r *riskResponseRepository) getLinkDocID(riskID, responseID int64) string {
	return fmt.Sprintf("%d_%d", riskID, responseID)
}

func (r *riskResponseRepository) Link(ctx context.Context, riskID, responseID int64) error {
	// Verify that both risk and response exist
	if _, err := r.riskRepo.Get(ctx, riskID); err != nil {
		return goerr.Wrap(err, "risk not found", goerr.V("riskID", riskID))
	}

	if _, err := r.responseRepo.Get(ctx, responseID); err != nil {
		return goerr.Wrap(err, "response not found", goerr.V("responseID", responseID))
	}

	docID := r.getLinkDocID(riskID, responseID)
	docRef := r.client.Collection(r.riskResponsesCollection()).Doc(docID)

	// Check if the link already exists
	_, err := docRef.Get(ctx)
	if err == nil {
		// Link already exists, return success
		return nil
	}
	if status.Code(err) != codes.NotFound {
		return goerr.Wrap(err, "failed to check existing link",
			goerr.V("riskID", riskID),
			goerr.V("responseID", responseID))
	}

	// Create the link
	doc := &riskResponseDocument{
		RiskID:     riskID,
		ResponseID: responseID,
		CreatedAt:  time.Now().UTC(),
	}

	_, err = docRef.Set(ctx, doc)
	if err != nil {
		return goerr.Wrap(err, "failed to create link",
			goerr.V("riskID", riskID),
			goerr.V("responseID", responseID))
	}

	return nil
}

func (r *riskResponseRepository) Unlink(ctx context.Context, riskID, responseID int64) error {
	docID := r.getLinkDocID(riskID, responseID)
	docRef := r.client.Collection(r.riskResponsesCollection()).Doc(docID)

	// Check if the link exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "risk-response link not found",
				goerr.V("riskID", riskID),
				goerr.V("responseID", responseID))
		}
		return goerr.Wrap(err, "failed to check link existence",
			goerr.V("riskID", riskID),
			goerr.V("responseID", responseID))
	}

	// Delete the link
	_, err = docRef.Delete(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to delete link",
			goerr.V("riskID", riskID),
			goerr.V("responseID", responseID))
	}

	return nil
}

func (r *riskResponseRepository) GetResponsesByRisk(ctx context.Context, riskID int64) ([]*model.Response, error) {
	query := r.client.Collection(r.riskResponsesCollection()).Where("risk_id", "==", riskID)
	iter := query.Documents(ctx)
	defer iter.Stop()

	var responseIDs []int64
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate risk-response links", goerr.V("riskID", riskID))
		}

		var linkDoc riskResponseDocument
		if err := doc.DataTo(&linkDoc); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal risk-response link")
		}

		responseIDs = append(responseIDs, linkDoc.ResponseID)
	}

	// Fetch all responses
	responses := make([]*model.Response, 0, len(responseIDs))
	for _, respID := range responseIDs {
		resp, err := r.responseRepo.Get(ctx, respID)
		if err != nil {
			// Skip if response was deleted
			continue
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

func (r *riskResponseRepository) GetResponsesByRisks(ctx context.Context, riskIDs []int64) (map[int64][]*model.Response, error) {
	if len(riskIDs) == 0 {
		return make(map[int64][]*model.Response), nil
	}

	// Firestore has a limit of 30 items in an IN query, so we need to batch
	const batchSize = 30
	result := make(map[int64][]*model.Response)

	// Initialize result map
	for _, riskID := range riskIDs {
		result[riskID] = make([]*model.Response, 0)
	}

	for i := 0; i < len(riskIDs); i += batchSize {
		end := i + batchSize
		if end > len(riskIDs) {
			end = len(riskIDs)
		}

		batch := riskIDs[i:end]
		query := r.client.Collection(r.riskResponsesCollection()).Where("risk_id", "in", batch)
		iter := query.Documents(ctx)

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				iter.Stop()
				return nil, goerr.Wrap(err, "failed to iterate risk-response links")
			}

			var linkDoc riskResponseDocument
			if err := doc.DataTo(&linkDoc); err != nil {
				iter.Stop()
				return nil, goerr.Wrap(err, "failed to unmarshal risk-response link")
			}

			resp, err := r.responseRepo.Get(ctx, linkDoc.ResponseID)
			if err != nil {
				// Skip if response was deleted
				continue
			}

			result[linkDoc.RiskID] = append(result[linkDoc.RiskID], resp)
		}
		iter.Stop()
	}

	return result, nil
}

func (r *riskResponseRepository) GetRisksByResponse(ctx context.Context, responseID int64) ([]*model.Risk, error) {
	query := r.client.Collection(r.riskResponsesCollection()).Where("response_id", "==", responseID)
	iter := query.Documents(ctx)
	defer iter.Stop()

	var riskIDs []int64
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate risk-response links", goerr.V("responseID", responseID))
		}

		var linkDoc riskResponseDocument
		if err := doc.DataTo(&linkDoc); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal risk-response link")
		}

		riskIDs = append(riskIDs, linkDoc.RiskID)
	}

	// Fetch all risks
	risks := make([]*model.Risk, 0, len(riskIDs))
	for _, riskID := range riskIDs {
		risk, err := r.riskRepo.Get(ctx, riskID)
		if err != nil {
			// Skip if risk was deleted
			continue
		}
		risks = append(risks, risk)
	}

	return risks, nil
}

func (r *riskResponseRepository) GetRisksByResponses(ctx context.Context, responseIDs []int64) (map[int64][]*model.Risk, error) {
	if len(responseIDs) == 0 {
		return make(map[int64][]*model.Risk), nil
	}

	// Firestore has a limit of 30 items in an IN query, so we need to batch
	const batchSize = 30
	result := make(map[int64][]*model.Risk)

	// Initialize result map
	for _, responseID := range responseIDs {
		result[responseID] = make([]*model.Risk, 0)
	}

	for i := 0; i < len(responseIDs); i += batchSize {
		end := i + batchSize
		if end > len(responseIDs) {
			end = len(responseIDs)
		}

		batch := responseIDs[i:end]
		query := r.client.Collection(r.riskResponsesCollection()).Where("response_id", "in", batch)
		iter := query.Documents(ctx)

		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				iter.Stop()
				return nil, goerr.Wrap(err, "failed to iterate risk-response links")
			}

			var linkDoc riskResponseDocument
			if err := doc.DataTo(&linkDoc); err != nil {
				iter.Stop()
				return nil, goerr.Wrap(err, "failed to unmarshal risk-response link")
			}

			risk, err := r.riskRepo.Get(ctx, linkDoc.RiskID)
			if err != nil {
				// Skip if risk was deleted
				continue
			}

			result[linkDoc.ResponseID] = append(result[linkDoc.ResponseID], risk)
		}
		iter.Stop()
	}

	return result, nil
}

func (r *riskResponseRepository) DeleteByResponse(ctx context.Context, responseID int64) error {
	query := r.client.Collection(r.riskResponsesCollection()).Where("response_id", "==", responseID)
	iter := query.Documents(ctx)
	defer iter.Stop()

	bulkWriter := r.client.BulkWriter(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return goerr.Wrap(err, "failed to iterate risk-response links for deletion", goerr.V("responseID", responseID))
		}

		if _, err := bulkWriter.Delete(doc.Ref); err != nil {
			return goerr.Wrap(err, "failed to delete risk-response link", goerr.V("responseID", responseID))
		}
	}

	bulkWriter.End()

	return nil
}

func (r *riskResponseRepository) DeleteByRisk(ctx context.Context, riskID int64) error {
	query := r.client.Collection(r.riskResponsesCollection()).Where("risk_id", "==", riskID)
	iter := query.Documents(ctx)
	defer iter.Stop()

	bulkWriter := r.client.BulkWriter(ctx)

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return goerr.Wrap(err, "failed to iterate risk-response links for deletion", goerr.V("riskID", riskID))
		}

		if _, err := bulkWriter.Delete(doc.Ref); err != nil {
			return goerr.Wrap(err, "failed to delete risk-response link", goerr.V("riskID", riskID))
		}
	}

	bulkWriter.End()

	return nil
}
