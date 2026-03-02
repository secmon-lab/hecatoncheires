package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type caseRepository struct {
	client *firestore.Client
}

func newCaseRepository(client *firestore.Client) *caseRepository {
	return &caseRepository{
		client: client,
	}
}

func (r *caseRepository) casesCollection(workspaceID string) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).Collection("cases")
}

func (r *caseRepository) caseCounterRef(workspaceID string) *firestore.DocumentRef {
	return r.client.Collection("counters").Doc("case").Collection("workspaces").Doc(workspaceID)
}

func (r *caseRepository) getNextID(ctx context.Context, workspaceID string) (int64, error) {
	counterRef := r.caseCounterRef(workspaceID)

	var nextID int64
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(counterRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				nextID = 1
				return tx.Set(counterRef, map[string]interface{}{
					"value": nextID,
				})
			}
			return goerr.Wrap(err, "failed to get counter")
		}

		currentValue, err := doc.DataAt("value")
		if err != nil {
			return goerr.Wrap(err, "failed to get counter value")
		}

		val, ok := currentValue.(int64)
		if !ok {
			return goerr.New("counter value is not of type int64", goerr.V("value", currentValue))
		}
		nextID = val + 1
		return tx.Update(counterRef, []firestore.Update{
			{Path: "value", Value: nextID},
		})
	})

	if err != nil {
		return 0, goerr.Wrap(err, "failed to get next ID")
	}

	return nextID, nil
}

func (r *caseRepository) Create(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error) {
	nextID, err := r.getNextID(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get next ID")
	}

	now := time.Now().UTC()
	created := &model.Case{
		ID:             nextID,
		Title:          c.Title,
		Description:    c.Description,
		Status:         c.Status,
		AssigneeIDs:    c.AssigneeIDs,
		SlackChannelID: c.SlackChannelID,
		IsPrivate:      c.IsPrivate,
		ChannelUserIDs: c.ChannelUserIDs,
		FieldValues:    c.FieldValues,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	docID := fmt.Sprintf("%d", created.ID)

	_, err = r.casesCollection(workspaceID).Doc(docID).Set(ctx, created)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create case", goerr.V("id", created.ID))
	}

	return created, nil
}

func (r *caseRepository) Get(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	docID := fmt.Sprintf("%d", id)
	docSnap, err := r.casesCollection(workspaceID).Doc(docID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get case", goerr.V("id", id))
	}

	var c model.Case
	if err := docSnap.DataTo(&c); err != nil {
		return nil, goerr.Wrap(err, "failed to decode case", goerr.V("id", id))
	}

	return &c, nil
}

func (r *caseRepository) List(ctx context.Context, workspaceID string, opts ...interfaces.ListCaseOption) ([]*model.Case, error) {
	cfg := interfaces.BuildListCaseConfig(opts...)

	query := r.casesCollection(workspaceID).Query
	if statusFilter := cfg.Status(); statusFilter != nil {
		query = query.Where("Status", "==", string(*statusFilter))
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var cases []*model.Case
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate cases")
		}

		var c model.Case
		if err := docSnap.DataTo(&c); err != nil {
			return nil, goerr.Wrap(err, "failed to decode case", goerr.V("doc_id", docSnap.Ref.ID))
		}

		cases = append(cases, &c)
	}

	return cases, nil
}

func (r *caseRepository) Update(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error) {
	docID := fmt.Sprintf("%d", c.ID)
	docRef := r.casesCollection(workspaceID).Doc(docID)

	// Check if document exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", c.ID))
		}
		return nil, goerr.Wrap(err, "failed to check case existence", goerr.V("id", c.ID))
	}

	// Update with new timestamp
	updated := &model.Case{
		ID:             c.ID,
		Title:          c.Title,
		Description:    c.Description,
		Status:         c.Status,
		AssigneeIDs:    c.AssigneeIDs,
		SlackChannelID: c.SlackChannelID,
		IsPrivate:      c.IsPrivate,
		ChannelUserIDs: c.ChannelUserIDs,
		FieldValues:    c.FieldValues,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      time.Now().UTC(),
	}

	_, err = docRef.Set(ctx, updated)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case", goerr.V("id", c.ID))
	}

	return updated, nil
}

func (r *caseRepository) Delete(ctx context.Context, workspaceID string, id int64) error {
	docID := fmt.Sprintf("%d", id)
	docRef := r.casesCollection(workspaceID).Doc(docID)

	// Check if document exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
		}
		return goerr.Wrap(err, "failed to check case existence", goerr.V("id", id))
	}

	_, err = docRef.Delete(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to delete case", goerr.V("id", id))
	}

	return nil
}

func (r *caseRepository) GetBySlackChannelID(ctx context.Context, workspaceID string, channelID string) (*model.Case, error) {
	iter := r.casesCollection(workspaceID).
		Where("SlackChannelID", "==", channelID).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	docSnap, err := iter.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, goerr.Wrap(err, "failed to query case by slack channel ID",
			goerr.V("channel_id", channelID))
	}

	var c model.Case
	if err := docSnap.DataTo(&c); err != nil {
		return nil, goerr.Wrap(err, "failed to decode case",
			goerr.V("channel_id", channelID))
	}

	return &c, nil
}

func (r *caseRepository) CountFieldValues(ctx context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (int64, int64, error) {
	col := r.casesCollection(workspaceID)
	fieldTypePath := fmt.Sprintf("FieldValues.%s.Type", fieldID)
	fieldValuePath := fmt.Sprintf("FieldValues.%s.Value", fieldID)

	// Count total cases with this field type (aggregation, no document data transfer)
	totalQuery := col.Where(fieldTypePath, "==", string(fieldType))
	totalResult, err := totalQuery.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, 0, goerr.Wrap(err, "failed to count total field values",
			goerr.V("field_id", fieldID))
	}
	totalVal, ok := totalResult["total"]
	if !ok {
		return 0, 0, goerr.New("missing total count in aggregation result",
			goerr.V("field_id", fieldID))
	}
	totalPB, ok := totalVal.(*pb.Value)
	if !ok {
		return 0, 0, goerr.New("unexpected total count type",
			goerr.V("field_id", fieldID), goerr.V("type", fmt.Sprintf("%T", totalVal)))
	}
	totalCount := totalPB.GetIntegerValue()

	// Count valid values using chunked "in" queries (max 10 per query)
	var validCount int64
	for i := 0; i < len(validValues); i += 10 {
		end := i + 10
		if end > len(validValues) {
			end = len(validValues)
		}
		chunk := validValues[i:end]

		chunkIface := make([]interface{}, len(chunk))
		for j, v := range chunk {
			chunkIface[j] = v
		}

		chunkQuery := col.Where(fieldValuePath, "in", chunkIface)
		chunkResult, err := chunkQuery.NewAggregationQuery().WithCount("c").Get(ctx)
		if err != nil {
			return 0, 0, goerr.Wrap(err, "failed to count valid field values",
				goerr.V("field_id", fieldID))
		}
		cv, ok := chunkResult["c"]
		if !ok {
			continue
		}
		cvPB, ok := cv.(*pb.Value)
		if !ok {
			continue
		}
		validCount += cvPB.GetIntegerValue()
	}

	return totalCount, validCount, nil
}

func (r *caseRepository) FindCaseWithInvalidFieldValue(ctx context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (*model.Case, error) {
	col := r.casesCollection(workspaceID)

	// For select with <= 10 options: use not-in for exact match
	if fieldType == types.FieldTypeSelect && len(validValues) <= 10 {
		fieldValuePath := fmt.Sprintf("FieldValues.%s.Value", fieldID)
		validIface := make([]interface{}, len(validValues))
		for i, v := range validValues {
			validIface[i] = v
		}

		iter := col.Where(fieldValuePath, "not-in", validIface).Limit(1).Documents(ctx)
		defer iter.Stop()

		docSnap, err := iter.Next()
		if err == iterator.Done {
			return nil, nil
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to query invalid field values",
				goerr.V("field_id", fieldID))
		}

		var found model.Case
		if err := docSnap.DataTo(&found); err != nil {
			return nil, goerr.Wrap(err, "failed to decode case",
				goerr.V("field_id", fieldID))
		}
		return &found, nil
	}

	// For select > 10 options or multi-select: stream and check
	fieldTypePath := fmt.Sprintf("FieldValues.%s.Type", fieldID)
	iter := col.Where(fieldTypePath, "==", string(fieldType)).Documents(ctx)
	defer iter.Stop()

	validSet := make(map[string]bool, len(validValues))
	for _, v := range validValues {
		validSet[v] = true
	}

	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			return nil, nil
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate cases for field validation",
				goerr.V("field_id", fieldID))
		}

		var found model.Case
		if err := docSnap.DataTo(&found); err != nil {
			return nil, goerr.Wrap(err, "failed to decode case",
				goerr.V("field_id", fieldID))
		}

		fv, ok := found.FieldValues[fieldID]
		if !ok {
			continue
		}

		if !fv.IsValueInSet(fieldType, validSet) {
			return &found, nil
		}
	}
}
