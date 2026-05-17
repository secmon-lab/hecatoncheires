package slackid_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/slackid"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty string passes through", in: "", want: ""},
		{name: "bare user id is preserved", in: "U12345", want: "U12345"},
		{name: "bare workspace user id is preserved", in: "W12345", want: "W12345"},
		{name: "user-first composite", in: "U12345-T98765", want: "U12345"},
		{name: "team-first composite", in: "T98765-U12345", want: "U12345"},
		{name: "user-first composite with W prefix", in: "W12345-T98765", want: "W12345"},
		{name: "no user-id chunk falls back to raw", in: "T98765-T11111", want: "T98765-T11111"},
		{name: "non-slack input falls back to raw", in: "foo-bar", want: "foo-bar"},
		{name: "alphanumeric mixed user id", in: "UABC123def-T1", want: "UABC123def"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := slackid.Normalize(c.in)
			gt.Value(t, got).Equal(c.want)
		})
	}
}

func TestIsUserID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "single letter U", in: "U", want: false},
		{name: "minimum valid U id", in: "U1", want: true},
		{name: "minimum valid W id", in: "W1", want: true},
		{name: "team prefix is not a user id", in: "T1", want: false},
		{name: "uppercase digits user id", in: "U12345", want: true},
		{name: "mixed-case alphanumeric user id", in: "UABC123def", want: true},
		{name: "user id with hyphen is rejected", in: "U-123", want: false},
		{name: "user id with non-alphanumeric is rejected", in: "U!23", want: false},
		{name: "unknown prefix is rejected", in: "X12345", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := slackid.IsUserID(c.in)
			gt.Value(t, got).Equal(c.want)
		})
	}
}
