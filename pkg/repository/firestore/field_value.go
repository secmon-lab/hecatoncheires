package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
)

type fieldValueRepository struct {
	client           *firestore.Client
	collectionPrefix string
}

func newFieldValueRepository(client *firestore.Client) *fieldValueRepository {
	return &fieldValueRepository{
		client:           client,
		collectionPrefix: "",
	}
}

func (r *fieldValueRepository) fieldValuesCollection() string {
	if r.collectionPrefix != "" {
		return r.collectionPrefix + "_case_field_values"
	}
	return "case_field_values"
}

func (r *fieldValueRepository) docID(caseID int64, fieldID string) string {
	return fmt.Sprintf("%d_%s", caseID, fieldID)
}

func (r *fieldValueRepository) GetByCaseID(ctx context.Context, caseID int64) ([]model.FieldValue, error) {
	iter := r.client.Collection(r.fieldValuesCollection()).
		Where("CaseID", "==", caseID).
		Documents(ctx)
	defer iter.Stop()

	var fieldValues []model.FieldValue
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate field values", goerr.V("case_id", caseID))
		}

		var fv model.FieldValue
		if err := docSnap.DataTo(&fv); err != nil {
			return nil, goerr.Wrap(err, "failed to decode field value", goerr.V("doc_id", docSnap.Ref.ID))
		}

		fieldValues = append(fieldValues, fv)
	}

	return fieldValues, nil
}

func (r *fieldValueRepository) GetByCaseIDs(ctx context.Context, caseIDs []int64) (map[int64][]model.FieldValue, error) {
	// Initialize result map
	result := make(map[int64][]model.FieldValue)
	for _, caseID := range caseIDs {
		result[caseID] = make([]model.FieldValue, 0)
	}

	// Execute parallel queries for each case ID
	// (avoids creating new composite index)
	for _, caseID := range caseIDs {
		fieldValues, err := r.GetByCaseID(ctx, caseID)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to get field values by case", goerr.V("case_id", caseID))
		}
		result[caseID] = fieldValues
	}

	return result, nil
}

func (r *fieldValueRepository) Save(ctx context.Context, fv *model.FieldValue) error {
	docID := r.docID(fv.CaseID, fv.FieldID)

	saved := &model.FieldValue{
		CaseID:    fv.CaseID,
		FieldID:   fv.FieldID,
		Value:     fv.Value,
		UpdatedAt: time.Now().UTC(),
	}

	_, err := r.client.Collection(r.fieldValuesCollection()).Doc(docID).Set(ctx, saved)
	if err != nil {
		return goerr.Wrap(err, "failed to save field value",
			goerr.V("case_id", fv.CaseID),
			goerr.V("field_id", fv.FieldID))
	}

	return nil
}

func (r *fieldValueRepository) DeleteByCaseID(ctx context.Context, caseID int64) error {
	// Query all field values for the case
	iter := r.client.Collection(r.fieldValuesCollection()).
		Where("CaseID", "==", caseID).
		Documents(ctx)
	defer iter.Stop()

	// Collect document references to delete
	var docRefs []*firestore.DocumentRef
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return goerr.Wrap(err, "failed to iterate field values for deletion", goerr.V("case_id", caseID))
		}
		docRefs = append(docRefs, docSnap.Ref)
	}

	// Delete all field values
	for _, docRef := range docRefs {
		_, err := docRef.Delete(ctx)
		if err != nil {
			return goerr.Wrap(err, "failed to delete field value",
				goerr.V("case_id", caseID),
				goerr.V("doc_id", docRef.ID))
		}
	}

	return nil
}
