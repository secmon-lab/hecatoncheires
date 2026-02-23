package github_test

import (
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/service/github"
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

	if pr.Number != 42 {
		t.Errorf("expected Number 42, got %d", pr.Number)
	}
	if pr.Title != "Add feature X" {
		t.Errorf("expected Title 'Add feature X', got %q", pr.Title)
	}
	if len(pr.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(pr.Labels))
	}
	if len(pr.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(pr.Comments))
	}
	if pr.Comments[0].Author != "bob" {
		t.Errorf("expected comment author 'bob', got %q", pr.Comments[0].Author)
	}
	if len(pr.Reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(pr.Reviews))
	}
	if pr.Reviews[0].State != "APPROVED" {
		t.Errorf("expected review state 'APPROVED', got %q", pr.Reviews[0].State)
	}
}

func TestIssueFields(t *testing.T) {
	t.Parallel()

	issue := &github.Issue{
		Number:    10,
		Title:     "Bug report",
		Body:      "Something is broken",
		Author:    "dave",
		State:     "OPEN",
		URL:       "https://github.com/owner/repo/issues/10",
		Labels:    []string{"bug"},
		CreatedAt: time.Date(2025, 2, 1, 8, 0, 0, 0, time.UTC),
		Comments: []github.Comment{
			{
				Author:    "eve",
				Body:      "Can reproduce",
				CreatedAt: time.Date(2025, 2, 1, 9, 0, 0, 0, time.UTC),
			},
		},
	}

	if issue.Number != 10 {
		t.Errorf("expected Number 10, got %d", issue.Number)
	}
	if issue.State != "OPEN" {
		t.Errorf("expected State 'OPEN', got %q", issue.State)
	}
	if len(issue.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(issue.Comments))
	}
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

	// Verify the Since field is used to distinguish new vs old comments
	oldCount := 0
	newCount := 0
	for _, c := range iwc.Comments {
		if c.CreatedAt.Before(iwc.Since) {
			oldCount++
		} else {
			newCount++
		}
	}

	if oldCount != 1 {
		t.Errorf("expected 1 old comment, got %d", oldCount)
	}
	if newCount != 1 {
		t.Errorf("expected 1 new comment, got %d", newCount)
	}
}

func TestRepositoryValidation(t *testing.T) {
	t.Parallel()

	// Test valid result
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

	if !valid.Valid {
		t.Error("expected Valid to be true")
	}
	if valid.FullName != "secmon-lab/hecatoncheires" {
		t.Errorf("expected FullName 'secmon-lab/hecatoncheires', got %q", valid.FullName)
	}

	// Test invalid result
	invalid := &github.RepositoryValidation{
		Valid:        false,
		Owner:        "nonexistent",
		Repo:         "repo",
		ErrorMessage: "repository not found",
	}

	if invalid.Valid {
		t.Error("expected Valid to be false")
	}
	if invalid.ErrorMessage != "repository not found" {
		t.Errorf("expected error message 'repository not found', got %q", invalid.ErrorMessage)
	}
}
