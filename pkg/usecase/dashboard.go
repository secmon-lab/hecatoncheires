package usecase

import (
	"context"
	_ "embed"
	mrand "math/rand/v2"
	"slices"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"golang.org/x/sync/errgroup"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// ErrUnauthenticated is returned by the dashboard "my *" queries when no auth
// token is present. Unlike ListCases (which treats a missing token as a
// system/bot context that bypasses per-user filtering), the home aggregation is
// meaningless without an identity, so it fails loudly instead.
var ErrUnauthenticated = goerr.New("unauthenticated")

// homeMessageFreshWindow is how long a generated home message is reused before a
// fresh one is generated. Intentionally non-configurable (product decision):
// history accumulates rather than being TTL-deleted, and freshness is judged
// here against this window.
const homeMessageFreshWindow = time.Hour

// homeMessageHistorySize is how many recent messages are read — both to find the
// latest for the freshness check and to feed the generator as "avoid repeating".
const homeMessageHistorySize = 5

// DashboardUseCase serves the login home dashboard: cross-workspace aggregation
// of the caller's own open Cases and incomplete Actions, favorite-workspace
// preferences, and the LLM-generated greeting. It is a dedicated usecase because
// it spans Case, Action, and UserPreference/HomeMessage repositories plus the
// workspace registry.
type DashboardUseCase struct {
	repo           interfaces.Repository
	registry       *model.WorkspaceRegistry
	staleThreshold time.Duration
	// homeMessageLLM generates the greeting. Nil disables the greeting
	// (GenerateHomeMessage returns "").
	homeMessageLLM gollem.LLMClient
}

func newDashboardUseCase(repo interfaces.Repository, registry *model.WorkspaceRegistry, staleThreshold time.Duration, homeMessageLLM gollem.LLMClient) *DashboardUseCase {
	return &DashboardUseCase{
		repo:           repo,
		registry:       registry,
		staleThreshold: staleThreshold,
		homeMessageLLM: homeMessageLLM,
	}
}

// ListMyOpenCases returns, across every workspace, the open Cases the caller is
// assigned to and may access, newest activity / stalled first.
func (uc *DashboardUseCase) ListMyOpenCases(ctx context.Context) ([]*model.MyOpenCase, error) {
	token, err := auth.TokenFromContext(ctx)
	if err != nil {
		return nil, goerr.Wrap(ErrUnauthenticated, "list my open cases")
	}

	entries := uc.registry.List()
	partial := make([][]*model.MyOpenCase, len(entries))

	if err := uc.fanOut(ctx, entries, func(fctx context.Context, i int, entry *model.WorkspaceEntry) error {
		cases, err := uc.repo.Case().List(fctx, entry.Workspace.ID, interfaces.WithStatus(types.CaseStatusOpen))
		if err != nil {
			return goerr.Wrap(err, "failed to list open cases", goerr.V("workspace_id", entry.Workspace.ID))
		}
		rows := make([]*model.MyOpenCase, 0)
		for _, c := range cases {
			if !slices.Contains(c.AssigneeIDs, token.Sub) {
				continue
			}
			if !model.IsCaseAccessible(c, token.Sub) {
				continue
			}
			rows = append(rows, &model.MyOpenCase{
				WorkspaceID:   entry.Workspace.ID,
				WorkspaceName: entry.Workspace.Name,
				Case:          c,
				Stalled:       uc.isStalled(c.UpdatedAt),
			})
		}
		partial[i] = rows
		return nil
	}); err != nil {
		return nil, err
	}

	result := make([]*model.MyOpenCase, 0)
	for _, rows := range partial {
		result = append(result, rows...)
	}
	// Stalled first, then most-recently-updated first.
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Stalled != result[j].Stalled {
			return result[i].Stalled
		}
		return result[i].Case.UpdatedAt.After(result[j].Case.UpdatedAt)
	})
	return result, nil
}

// ListMyDueActions returns, across every workspace, the caller's incomplete
// Actions on accessible open Cases, ordered by due date (overdue first, no due
// date last).
func (uc *DashboardUseCase) ListMyDueActions(ctx context.Context) ([]*model.MyDueAction, error) {
	token, err := auth.TokenFromContext(ctx)
	if err != nil {
		return nil, goerr.Wrap(ErrUnauthenticated, "list my due actions")
	}

	entries := uc.registry.List()
	partial := make([][]*model.MyDueAction, len(entries))

	if err := uc.fanOut(ctx, entries, func(fctx context.Context, i int, entry *model.WorkspaceEntry) error {
		cases, err := uc.repo.Case().List(fctx, entry.Workspace.ID, interfaces.WithStatus(types.CaseStatusOpen))
		if err != nil {
			return goerr.Wrap(err, "failed to list open cases", goerr.V("workspace_id", entry.Workspace.ID))
		}
		caseByID := make(map[int64]*model.Case, len(cases))
		caseIDs := make([]int64, 0, len(cases))
		for _, c := range cases {
			if !model.IsCaseAccessible(c, token.Sub) {
				continue
			}
			caseByID[c.ID] = c
			caseIDs = append(caseIDs, c.ID)
		}
		if len(caseIDs) == 0 {
			partial[i] = nil
			return nil
		}

		actionsByCase, err := uc.repo.Action().GetByCases(fctx, entry.Workspace.ID, caseIDs, interfaces.ActionListOptions{})
		if err != nil {
			return goerr.Wrap(err, "failed to get actions", goerr.V("workspace_id", entry.Workspace.ID))
		}

		statusSet := entry.ActionStatusSet
		if statusSet == nil {
			statusSet = model.DefaultActionStatusSet()
		}

		rows := make([]*model.MyDueAction, 0)
		for caseID, actions := range actionsByCase {
			parent, ok := caseByID[caseID]
			if !ok {
				continue
			}
			for _, a := range actions {
				if a.AssigneeID != token.Sub {
					continue
				}
				if statusSet.IsClosed(string(a.Status)) {
					continue
				}
				rows = append(rows, &model.MyDueAction{
					WorkspaceID:   entry.Workspace.ID,
					WorkspaceName: entry.Workspace.Name,
					Action:        a,
					CaseID:        parent.ID,
					CaseTitle:     parent.Title,
				})
			}
		}
		partial[i] = rows
		return nil
	}); err != nil {
		return nil, err
	}

	result := make([]*model.MyDueAction, 0)
	for _, rows := range partial {
		result = append(result, rows...)
	}
	sort.SliceStable(result, func(i, j int) bool {
		di, dj := result[i].Action.DueDate, result[j].Action.DueDate
		// Actions with a due date come before those without.
		if (di == nil) != (dj == nil) {
			return di != nil
		}
		// Both have a due date: earlier (overdue) first.
		if di != nil && dj != nil && !di.Equal(*dj) {
			return di.Before(*dj)
		}
		// Tie-break: oldest updated first.
		return result[i].Action.UpdatedAt.Before(result[j].Action.UpdatedAt)
	})
	return result, nil
}

// GetFavoriteWorkspaces returns the caller's favorite workspace IDs, filtered to
// those that still exist in the registry.
func (uc *DashboardUseCase) GetFavoriteWorkspaces(ctx context.Context) ([]string, error) {
	token, err := auth.TokenFromContext(ctx)
	if err != nil {
		return nil, goerr.Wrap(ErrUnauthenticated, "get favorite workspaces")
	}

	pref, err := uc.repo.UserPreference().Get(ctx, token.Sub)
	if err != nil {
		if isRepoNotFound(err) {
			return []string{}, nil
		}
		return nil, goerr.Wrap(err, "failed to get user preference")
	}

	result := make([]string, 0, len(pref.FavoriteWorkspaceIDs))
	for _, id := range pref.FavoriteWorkspaceIDs {
		if _, err := uc.registry.Get(id); err == nil {
			result = append(result, id)
		}
	}
	return result, nil
}

// SetFavoriteWorkspaces replaces the caller's favorite workspace list wholesale.
// Unknown workspace IDs are rejected; duplicates are removed (order preserved).
// Returns the stored list.
func (uc *DashboardUseCase) SetFavoriteWorkspaces(ctx context.Context, workspaceIDs []string) ([]string, error) {
	token, err := auth.TokenFromContext(ctx)
	if err != nil {
		return nil, goerr.Wrap(ErrUnauthenticated, "set favorite workspaces")
	}

	normalized := make([]string, 0, len(workspaceIDs))
	seen := make(map[string]struct{}, len(workspaceIDs))
	for _, id := range workspaceIDs {
		if id == "" {
			return nil, goerr.Wrap(model.ErrUserPreferenceValidation, "favorite workspace id must not be empty")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		if _, err := uc.registry.Get(id); err != nil {
			return nil, goerr.Wrap(model.ErrUserPreferenceValidation, "unknown workspace", goerr.V("workspace_id", id))
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}

	now := time.Now()
	pref, err := uc.repo.UserPreference().Get(ctx, token.Sub)
	if err != nil {
		if !isRepoNotFound(err) {
			return nil, goerr.Wrap(err, "failed to load user preference")
		}
		pref = &model.UserPreference{UserID: token.Sub, CreatedAt: now}
	}
	pref.FavoriteWorkspaceIDs = normalized
	pref.UpdatedAt = now

	if err := uc.repo.UserPreference().Set(ctx, pref); err != nil {
		return nil, goerr.Wrap(err, "failed to save user preference")
	}
	return normalized, nil
}

// fanOut runs fn for each workspace entry concurrently, giving each an index so
// results land in a pre-sized slice without a shared-append race. The first
// error cancels the rest (fn receives the group context, so in-flight
// repository calls are cancelled) and is returned — partial success is never
// swallowed.
func (uc *DashboardUseCase) fanOut(ctx context.Context, entries []*model.WorkspaceEntry, fn func(ctx context.Context, i int, entry *model.WorkspaceEntry) error) error {
	if len(entries) == 0 {
		return nil
	}
	g, gctx := errgroup.WithContext(ctx)
	for i, entry := range entries {
		g.Go(func() error {
			return fn(gctx, i, entry)
		})
	}
	return g.Wait()
}

func (uc *DashboardUseCase) isStalled(updatedAt time.Time) bool {
	if uc.staleThreshold <= 0 {
		return false
	}
	return time.Since(updatedAt) > uc.staleThreshold
}

// homeMessageOutput is the structured shape the greeting LLM must return.
type homeMessageOutput struct {
	Message string `json:"message"`
}

//go:embed prompts/home_message_system.md
var homeMessageSystemPromptText string

//go:embed prompts/home_message_user.md
var homeMessageUserPromptText string

var homeMessageUserTmpl = template.Must(
	template.New("home_message_user").Parse(homeMessageUserPromptText))

// homeMessagePromptInput is the typed input for the home message user prompt.
// The template owns all string assembly (per .claude/rules/prompts.md); Go only
// computes the field values.
type homeMessagePromptInput struct {
	Lang           string
	Date           string
	TimeOfDay      string
	OpenCaseLoad   string
	DueActionLoad  string
	WorkspaceNames []string
	Flavor         string
	Nonce          int
	RecentMessages []string
}

func renderHomeMessagePrompt(input homeMessagePromptInput) (string, error) {
	var buf strings.Builder
	if err := homeMessageUserTmpl.Execute(&buf, input); err != nil {
		return "", goerr.Wrap(err, "render home message prompt")
	}
	return buf.String(), nil
}

// homeMessageFlavors seed tonal variety into the prompt. Randomly chosen per
// generation so cached-hourly messages differ over time.
var homeMessageFlavors = []string{
	"gently encouraging",
	"calm and grounding",
	"lightly curious",
	"matter-of-fact and brief",
	"warm and understated",
	"quietly motivating",
	"friendly with a touch of humor",
	"focused and reassuring",
}

// supportedHomeMessageLangs is the allow-list of greeting languages. Anything
// else normalizes to the first entry. Bounding the set keeps the per-user cache
// to a fixed number of buckets, so a client cannot bypass the freshness window
// (and run up LLM cost / history) by cycling arbitrary lang strings.
var supportedHomeMessageLangs = []string{"en", "ja"}

func normalizeHomeMessageLang(lang string) string {
	if slices.Contains(supportedHomeMessageLangs, lang) {
		return lang
	}
	return supportedHomeMessageLangs[0]
}

// GenerateHomeMessage returns the home greeting for the caller, reusing the most
// recent stored message when it is still fresh (same language, within the
// window) and generating+appending a new one otherwise. Returns "" (no error)
// when no greeting LLM is configured.
func (uc *DashboardUseCase) GenerateHomeMessage(ctx context.Context, clientTime time.Time, lang string) (string, error) {
	token, err := auth.TokenFromContext(ctx)
	if err != nil {
		return "", goerr.Wrap(ErrUnauthenticated, "generate home message")
	}
	if uc.homeMessageLLM == nil {
		return "", nil
	}
	lang = normalizeHomeMessageLang(lang)

	recent, err := uc.repo.HomeMessage().ListRecent(ctx, token.Sub, homeMessageHistorySize)
	if err != nil {
		return "", goerr.Wrap(err, "failed to list recent home messages")
	}
	// Reuse the newest message for this language when it is still fresh. The
	// history is language-mixed, so scan for the most recent same-language entry
	// rather than only inspecting index 0.
	if fresh, ok := freshSameLangMessage(recent, lang); ok {
		return fresh, nil
	}

	signals, err := uc.gatherHomeSignals(ctx)
	if err != nil {
		return "", goerr.Wrap(err, "failed to gather home signals")
	}

	prompt, err := renderHomeMessagePrompt(homeMessagePromptInput{
		Lang:           lang,
		Date:           clientTime.Format("2006-01-02"),
		TimeOfDay:      timeOfDay(clientTime.Hour()),
		OpenCaseLoad:   qualitativeLoad(signals.openCaseCount),
		DueActionLoad:  qualitativeLoad(signals.dueActionCount),
		WorkspaceNames: signals.workspaceNames,
		Flavor:         pickHomeMessageFlavor(),
		Nonce:          homeMessageNonce(),
		RecentMessages: recentMessageTexts(recent),
	})
	if err != nil {
		return "", err
	}

	resp, err := gollem.Query[homeMessageOutput](ctx, uc.homeMessageLLM, prompt,
		gollem.WithQuerySystemPrompt(homeMessageSystemPromptText))
	if err != nil {
		return "", goerr.Wrap(err, "failed to generate home message")
	}

	msg := ""
	if resp != nil && resp.Data != nil {
		msg = strings.TrimSpace(resp.Data.Message)
	}
	if msg == "" {
		return "", goerr.Wrap(model.ErrHomeMessageValidation, "empty home message generated")
	}

	rec := &model.HomeMessage{
		ID:        model.NewHomeMessageID(),
		UserID:    token.Sub,
		Message:   msg,
		Lang:      lang,
		CreatedAt: time.Now(),
	}
	if err := uc.repo.HomeMessage().Add(ctx, rec); err != nil {
		// Non-fatal: the freshly generated message is still returned; only the
		// cache write failed.
		errutil.Handle(ctx, goerr.Wrap(err, "failed to append home message"), "append home message")
	}
	return msg, nil
}

// freshSameLangMessage returns the newest message matching lang that is still
// within the freshness window. recent is newest-first.
func freshSameLangMessage(recent []*model.HomeMessage, lang string) (string, bool) {
	for _, m := range recent {
		if m.Lang != lang {
			continue
		}
		if time.Since(m.CreatedAt) < homeMessageFreshWindow {
			return m.Message, true
		}
		// The newest same-language message is stale; older ones are only older,
		// so stop.
		return "", false
	}
	return "", false
}

// recentMessageTexts extracts the message strings for the anti-repetition list.
func recentMessageTexts(recent []*model.HomeMessage) []string {
	texts := make([]string, 0, len(recent))
	for _, m := range recent {
		texts = append(texts, m.Message)
	}
	return texts
}

// pickHomeMessageFlavor / homeMessageNonce provide the cosmetic prompt variety.
// This randomness is not security-sensitive, so math/rand is intentional.
func pickHomeMessageFlavor() string {
	// #nosec G404 -- greeting variety only, not security-sensitive
	return homeMessageFlavors[mrand.IntN(len(homeMessageFlavors))]
}

func homeMessageNonce() int {
	// #nosec G404 -- greeting variety only, not security-sensitive
	return mrand.IntN(100000)
}

// homeSignals is the lightweight situation summary fed to the greeting LLM.
type homeSignals struct {
	openCaseCount  int
	dueActionCount int
	workspaceNames []string
}

// gatherHomeSignals derives the greeting inputs from the same cross-workspace
// aggregation used by the list queries. It runs only on a cache miss.
func (uc *DashboardUseCase) gatherHomeSignals(ctx context.Context) (homeSignals, error) {
	openCases, err := uc.ListMyOpenCases(ctx)
	if err != nil {
		return homeSignals{}, err
	}
	dueActions, err := uc.ListMyDueActions(ctx)
	if err != nil {
		return homeSignals{}, err
	}

	seen := make(map[string]struct{})
	names := make([]string, 0)
	addName := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, c := range openCases {
		addName(c.WorkspaceName)
	}
	for _, a := range dueActions {
		addName(a.WorkspaceName)
	}

	return homeSignals{
		openCaseCount:  len(openCases),
		dueActionCount: len(dueActions),
		workspaceNames: names,
	}, nil
}

// timeOfDay buckets a local hour into a coarse label for the prompt.
func timeOfDay(hour int) string {
	switch {
	case hour >= 5 && hour < 11:
		return "morning"
	case hour >= 11 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 22:
		return "evening"
	default:
		return "night"
	}
}

// qualitativeLoad maps a count to a vague band so the greeting never quotes an
// exact number (which would drift while the message is cached).
func qualitativeLoad(n int) string {
	switch {
	case n == 0:
		return "none"
	case n <= 2:
		return "a few"
	case n <= 6:
		return "several"
	default:
		return "many"
	}
}
