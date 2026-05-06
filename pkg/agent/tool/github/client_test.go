package github_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
)

func TestPullRequestFields(t *testing.T) {
	t.Parallel()

	pr := &github.PullRequest{
		Number:    42,
		Title:     "Add feature X",
		Body:      "This PR adds feature X",
		Author:    "alice",
		State:     "OPEN",
		URL:       "https://github.com/owner/repo/pull/42",
		Labels:    []string{"enhancement", "ready"},
		CreatedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Comments: []github.Comment{
			{
				Author:    "bob",
				Body:      "Looks good!",
				CreatedAt: time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC),
				URL:       "https://github.com/owner/repo/pull/42#issuecomment-1",
			},
		},
		Reviews: []github.Review{
			{
				Author:    "carol",
				Body:      "LGTM",
				State:     "APPROVED",
				CreatedAt: time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	gt.Number(t, pr.Number).Equal(42)
	gt.String(t, pr.Title).Equal("Add feature X")
	gt.Array(t, pr.Labels).Length(2)
	gt.Array(t, pr.Comments).Length(1).Required()
	gt.String(t, pr.Comments[0].Author).Equal("bob")
	gt.Array(t, pr.Reviews).Length(1).Required()
	gt.String(t, pr.Reviews[0].State).Equal("APPROVED")
}

func TestIssueFields(t *testing.T) {
	t.Parallel()

	closed := time.Date(2025, 2, 5, 9, 0, 0, 0, time.UTC)
	issue := &github.Issue{
		Number:    10,
		Title:     "Bug report",
		Body:      "Something is broken",
		Author:    "dave",
		State:     "CLOSED",
		URL:       "https://github.com/owner/repo/issues/10",
		Labels:    []string{"bug"},
		CreatedAt: time.Date(2025, 2, 1, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 2, 5, 9, 0, 0, 0, time.UTC),
		ClosedAt:  &closed,
		Comments: []github.Comment{
			{
				Author:    "eve",
				Body:      "Can reproduce",
				CreatedAt: time.Date(2025, 2, 1, 9, 0, 0, 0, time.UTC),
			},
		},
	}

	gt.Number(t, issue.Number).Equal(10)
	gt.String(t, issue.State).Equal("CLOSED")
	gt.Array(t, issue.Comments).Length(1)
	gt.Value(t, issue.ClosedAt).NotNil().Required()
	gt.Bool(t, issue.ClosedAt.Equal(closed)).True()
	gt.Bool(t, issue.UpdatedAt.Equal(time.Date(2025, 2, 5, 9, 0, 0, 0, time.UTC))).True()
}

func TestIssueWithCommentsNewMarker(t *testing.T) {
	t.Parallel()

	since := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)

	iwc := &github.IssueWithComments{
		Number:    5,
		Title:     "Discussion thread",
		Body:      "Let's discuss",
		Author:    "frank",
		State:     "OPEN",
		URL:       "https://github.com/owner/repo/issues/5",
		IsPR:      false,
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Since:     since,
		Comments: []github.Comment{
			{
				Author:    "grace",
				Body:      "Old comment",
				CreatedAt: time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC),
			},
			{
				Author:    "heidi",
				Body:      "New comment",
				CreatedAt: time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	oldCount := 0
	newCount := 0
	for _, c := range iwc.Comments {
		if c.CreatedAt.Before(iwc.Since) {
			oldCount++
		} else {
			newCount++
		}
	}

	gt.Number(t, oldCount).Equal(1)
	gt.Number(t, newCount).Equal(1)
}

func TestRepositoryValidation(t *testing.T) {
	t.Parallel()

	valid := &github.RepositoryValidation{
		Valid:                true,
		Owner:                "secmon-lab",
		Repo:                 "hecatoncheires",
		FullName:             "secmon-lab/hecatoncheires",
		Description:          "AI-native case management",
		IsPrivate:            false,
		PullRequestCount:     42,
		IssueCount:           10,
		CanFetchPullRequests: true,
		CanFetchIssues:       true,
	}

	gt.Bool(t, valid.Valid).True()
	gt.String(t, valid.FullName).Equal("secmon-lab/hecatoncheires")

	invalid := &github.RepositoryValidation{
		Valid:        false,
		Owner:        "nonexistent",
		Repo:         "repo",
		ErrorMessage: "repository not found",
	}

	gt.Bool(t, invalid.Valid).False()
	gt.String(t, invalid.ErrorMessage).Equal("repository not found")
}
