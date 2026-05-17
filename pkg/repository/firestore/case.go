package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
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
	// Validate at the persistence boundary — the only safe place to
	// catch an unattributable write before it lands in storage. The
	// caller (usecase) is responsible for everything else, including
	// CreatedAt / UpdatedAt; the repository assigns the storage-side
	// ID directly onto the caller's struct and persists the model
	// verbatim. NEVER rebuild via a field-by-field struct literal
	// or value-copy — those patterns silently drop any field added
	// to model.Case without an exhaustive search of every repo
	// Create / Update site.
	if err := c.Validate(); err != nil {
		return nil, goerr.Wrap(err, "case validation failed before create")
	}

	nextID, err := r.getNextID(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get next ID")
	}
	c.ID = nextID

	docID := fmt.Sprintf("%d", c.ID)
	if _, err := r.casesCollection(workspaceID).Doc(docID).Set(ctx, c); err != nil {
		return nil, goerr.Wrap(err, "failed to create case", goerr.V("id", c.ID))
	}

	return c, nil
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
	} else {
		// Default listings never include drafts. An `in` filter on Status
		// uses a single-field index — no composite index is required.
		// The empty string is included because legacy Firestore documents
		// predate the DRAFT status and stored an empty Status that
		// CaseStatus.Normalize() resolves to OPEN; excluding it here
		// would silently hide those rows from the default Cases view.
		query = query.Where("Status", "in", []string{
			"",
			string(types.CaseStatusOpen),
			string(types.CaseStatusClosed),
		})
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

func (r *caseRepository) ListDrafts(ctx context.Context, workspaceID string) ([]*model.Case, error) {
	// Single-field index on Status only; private-draft access control is
	// applied by the usecase layer, not by extra Where clauses (which would
	// require a composite index).
	iter := r.casesCollection(workspaceID).
		Where("Status", "==", string(types.CaseStatusDraft)).
		Documents(ctx)
	defer iter.Stop()

	drafts := make([]*model.Case, 0)
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate drafts")
		}

		var c model.Case
		if err := docSnap.DataTo(&c); err != nil {
			return nil, goerr.Wrap(err, "failed to decode draft", goerr.V("doc_id", docSnap.Ref.ID))
		}
		drafts = append(drafts, &c)
	}

	return drafts, nil
}

func (r *caseRepository) Update(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error) {
	if err := c.Validate(); err != nil {
		return nil, goerr.Wrap(err, "case validation failed before update")
	}

	docID := fmt.Sprintf("%d", c.ID)
	docRef := r.casesCollection(workspaceID).Doc(docID)

	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", c.ID))
		}
		return nil, goerr.Wrap(err, "failed to check case existence", goerr.V("id", c.ID))
	}

	if _, err := docRef.Set(ctx, c); err != nil {
		return nil, goerr.Wrap(err, "failed to update case", goerr.V("id", c.ID))
	}

	return c, nil
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

func (r *caseRepository) GetByRequestKey(ctx context.Context, workspaceID string, key string) (*model.Case, error) {
	iter := r.casesCollection(workspaceID).
		Where("RequestKey", "==", key).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	docSnap, err := iter.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, goerr.Wrap(err, "failed to query case by request key",
			goerr.V("key", key))
	}

	var c model.Case
	if err := docSnap.DataTo(&c); err != nil {
		return nil, goerr.Wrap(err, "failed to decode case",
			goerr.V("key", key))
	}

	return &c, nil
}

func (r *caseRepository) CountFieldValues(ctx context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (int64, int64, error) {
	// Earlier this method ran two independent aggregation queries — one
	// counting documents whose `FieldValues.<id>.Type == fieldType`, the
	// other counting documents whose `FieldValues.<id>.Value` was in the
	// validValues list. The second query did not constrain Type, so a
	// text-typed field whose value collided with a valid select option
	// was counted as `valid` even though the corresponding `total` did
	// not include it. Adding a composite index to chain
	// `Where("Type", "==").Where("Value", "in")` is forbidden by
	// firestore.md, so the repo now iterates the type-filtered documents
	// once and counts valid entries in Go using the model's
	// IsValueInSet logic — which already handles the `[]interface{}`
	// vs `[]string` shape mismatch the Firestore decoder produces.
	//
	// Two micro-optimisations keep per-document cost low so the O(N)
	// scan does not become a wire-bandwidth or CPU hot path:
	//   1. `Select(fieldTypePath, fieldValuePath)` so Firestore only
	//      returns the two map entries the count needs — not the entire
	//      case document.
	//   2. The receiver of `DataTo` is a minimal local struct that
	//      decodes only `FieldValues`. Decoding the full `model.Case`
	//      pulls in title / description / assignees / channel members
	//      that the count does not look at.
	// (Document-count scaling is still O(workspace size); analytical
	// counts that need true O(1) would have to relax the firestore.md
	// composite-index ban, which is a project-policy decision.)
	col := r.casesCollection(workspaceID)
	fieldTypePath := fmt.Sprintf("FieldValues.%s.Type", fieldID)
	fieldValuePath := fmt.Sprintf("FieldValues.%s.Value", fieldID)

	validSet := make(map[string]bool, len(validValues))
	for _, v := range validValues {
		validSet[v] = true
	}

	iter := col.
		Where(fieldTypePath, "==", string(fieldType)).
		Select(fieldTypePath, fieldValuePath).
		Documents(ctx)
	defer iter.Stop()

	// fieldValuesOnly is a minimal projection target so DataTo does not
	// allocate the full Case. The map only carries the entries the
	// Select() above requested.
	type fieldValuesOnly struct {
		FieldValues map[string]model.FieldValue
	}

	var totalCount, validCount int64
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return 0, 0, goerr.Wrap(err, "failed to iterate cases for field-value count",
				goerr.V("field_id", fieldID))
		}

		var partial fieldValuesOnly
		if err := doc.DataTo(&partial); err != nil {
			return 0, 0, goerr.Wrap(err, "failed to decode case for field-value count",
				goerr.V("field_id", fieldID))
		}
		fv, ok := partial.FieldValues[fieldID]
		if !ok || fv.Type != fieldType {
			continue
		}
		totalCount++
		if fv.IsValueInSet(fieldType, validSet) {
			validCount++
		}
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
