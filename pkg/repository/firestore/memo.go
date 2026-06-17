package firestore

import (
	"context"
	"fmt"
	"sort"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type memoRepository struct {
	client *firestore.Client
}

func newMemoRepository(client *firestore.Client) *memoRepository {
	return &memoRepository{
		client: client,
	}
}

// memosCollection returns the subcollection ref for memos under a specific case.
// Path: workspaces/{workspaceID}/cases/{caseID}/memos
func (r *memoRepository) memosCollection(workspaceID string, caseID int64) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", caseID)).
		Collection("memos")
}

func (r *memoRepository) Create(ctx context.Context, workspaceID string, memo *model.Memo) (*model.Memo, error) {
	if err := memo.Validate(); err != nil {
		return nil, goerr.Wrap(err, "memo validation failed before create")
	}

	docRef := r.memosCollection(workspaceID, memo.CaseID).Doc(string(memo.ID))
	if _, err := docRef.Set(ctx, memo); err != nil {
		return nil, goerr.Wrap(err, "failed to create memo",
			goerr.V("memo_id", memo.ID),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", memo.CaseID),
		)
	}

	return memo, nil
}

func (r *memoRepository) Get(ctx context.Context, workspaceID string, caseID int64, id model.MemoID) (*model.Memo, error) {
	docSnap, err := r.memosCollection(workspaceID, caseID).Doc(string(id)).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "memo not found",
				goerr.V("memo_id", id),
				goerr.V("workspace_id", workspaceID),
				goerr.V("case_id", caseID),
			)
		}
		return nil, goerr.Wrap(err, "failed to get memo",
			goerr.V("memo_id", id),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
		)
	}

	var m model.Memo
	if err := docSnap.DataTo(&m); err != nil {
		return nil, goerr.Wrap(err, "failed to decode memo", goerr.V("doc_id", docSnap.Ref.ID))
	}

	return &m, nil
}

func (r *memoRepository) GetByIDs(ctx context.Context, workspaceID string, caseID int64, ids []model.MemoID) (map[model.MemoID]*model.Memo, error) {
	result := make(map[model.MemoID]*model.Memo, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	col := r.memosCollection(workspaceID, caseID)
	refs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		refs[i] = col.Doc(string(id))
	}

	snaps, err := r.client.GetAll(ctx, refs)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to batch get memos",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", caseID),
		)
	}

	for _, snap := range snaps {
		if !snap.Exists() {
			continue
		}
		var m model.Memo
		if err := snap.DataTo(&m); err != nil {
			return nil, goerr.Wrap(err, "failed to decode memo", goerr.V("doc_id", snap.Ref.ID))
		}
		result[m.ID] = &m
	}

	return result, nil
}

func (r *memoRepository) List(ctx context.Context, workspaceID string, caseID int64, opts interfaces.MemoListOptions) ([]*model.Memo, error) {
	iter := r.memosCollection(workspaceID, caseID).Documents(ctx)
	defer iter.Stop()

	memos := make([]*model.Memo, 0)
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate memos",
				goerr.V("workspace_id", workspaceID),
				goerr.V("case_id", caseID),
			)
		}

		var m model.Memo
		if err := docSnap.DataTo(&m); err != nil {
			return nil, goerr.Wrap(err, "failed to decode memo", goerr.V("doc_id", docSnap.Ref.ID))
		}

		if !opts.ArchiveScope.Allows(m.IsArchived()) {
			continue
		}

		memos = append(memos, &m)
	}

	// Sort by CreatedAt ascending in memory — no composite index needed.
	// Tie-break on ID (UUID v7 is lexicographically time-ordered) so the order
	// is stable when two memos share a CreatedAt timestamp.
	sort.Slice(memos, func(i, j int) bool {
		if memos[i].CreatedAt.Equal(memos[j].CreatedAt) {
			return memos[i].ID < memos[j].ID
		}
		return memos[i].CreatedAt.Before(memos[j].CreatedAt)
	})

	return memos, nil
}

func (r *memoRepository) Update(ctx context.Context, workspaceID string, memo *model.Memo) (*model.Memo, error) {
	if err := memo.Validate(); err != nil {
		return nil, goerr.Wrap(err, "memo validation failed before update")
	}

	docRef := r.memosCollection(workspaceID, memo.CaseID).Doc(string(memo.ID))

	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "memo not found",
				goerr.V("memo_id", memo.ID),
				goerr.V("workspace_id", workspaceID),
				goerr.V("case_id", memo.CaseID),
			)
		}
		return nil, goerr.Wrap(err, "failed to check memo existence",
			goerr.V("memo_id", memo.ID),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", memo.CaseID),
		)
	}

	if _, err := docRef.Set(ctx, memo); err != nil {
		return nil, goerr.Wrap(err, "failed to update memo",
			goerr.V("memo_id", memo.ID),
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", memo.CaseID),
		)
	}

	return memo, nil
}
