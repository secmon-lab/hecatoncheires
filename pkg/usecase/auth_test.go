package usecase_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// TestNormalizeSlackUserSub pins the OIDC sub-claim normaliser. Before
// this fix the composite "Uxxx-Txxx" form was stored as the reporter ID
// verbatim, which Slack rejected when activateCase tried to invite the
// reporter to the freshly-created case channel.
func TestNormalizeSlackUserSub(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "raw user id is unchanged",
			in:   "U01ABC",
			want: "U01ABC",
		},
		{
			name: "user-team form extracts the user id",
			in:   "U01ABC-T02XYZ",
			want: "U01ABC",
		},
		{
			name: "team-user form still extracts the user id",
			in:   "T02XYZ-U01ABC",
			want: "U01ABC",
		},
		{
			name: "enterprise grid W-prefix is recognised as a user id",
			in:   "W04ENT-T02XYZ",
			want: "W04ENT",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
		{
			name: "no recognisable user id falls back to the raw value",
			in:   "T02XYZ-T03ABC",
			want: "T02XYZ-T03ABC",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := usecase.NormalizeSlackUserSubForTest(c.in)
			gt.Value(t, got).Equal(c.want)
		})
	}
}
