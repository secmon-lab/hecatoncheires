package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/urfave/cli/v3"
)

func runJiraFlags(t *testing.T, args []string) *config.Jira {
	t.Helper()
	var j config.Jira
	cmd := &cli.Command{
		Name:  "test",
		Flags: j.Flags(),
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}
	err := cmd.Run(context.Background(), append([]string{"test"}, args...))
	gt.NoError(t, err).Required()
	return &j
}

func TestJiraIsConfigured(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "all three flags set",
			args: []string{
				"--jira-base-url=https://example.atlassian.net",
				"--jira-email=alice@example.com",
				"--jira-api-token=token123",
			},
			want: true,
		},
		{name: "no flags set", args: nil, want: false},
		{
			name: "missing api token",
			args: []string{
				"--jira-base-url=https://example.atlassian.net",
				"--jira-email=alice@example.com",
			},
			want: false,
		},
		{
			name: "missing email",
			args: []string{
				"--jira-base-url=https://example.atlassian.net",
				"--jira-api-token=token123",
			},
			want: false,
		},
		{
			name: "missing base url",
			args: []string{
				"--jira-email=alice@example.com",
				"--jira-api-token=token123",
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			j := runJiraFlags(t, tc.args)
			if tc.want {
				gt.Bool(t, j.IsConfigured()).True()
			} else {
				gt.Bool(t, j.IsConfigured()).False()
			}
		})
	}
}

func TestJiraConfigure_NotConfigured(t *testing.T) {
	j := runJiraFlags(t, nil)
	tools, err := j.Configure(context.Background())
	gt.NoError(t, err).Required()
	gt.Array(t, tools).Length(0)
}

func TestJiraConfigure_Configured(t *testing.T) {
	j := runJiraFlags(t, []string{
		"--jira-base-url=https://example.atlassian.net",
		"--jira-email=alice@example.com",
		"--jira-api-token=token123",
	})
	tools, err := j.Configure(context.Background())
	gt.NoError(t, err).Required()
	gt.Array(t, tools).Length(3).Required()

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Spec().Name] = true
	}
	gt.Bool(t, names["jira_list_projects"]).True()
	gt.Bool(t, names["jira_search_issues"]).True()
	gt.Bool(t, names["jira_get_issues"]).True()
}

func TestJiraConfigure_PartialConfigIsAnError(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "only base url set",
			args: []string{"--jira-base-url=https://example.atlassian.net"},
		},
		{
			name: "only email set",
			args: []string{"--jira-email=alice@example.com"},
		},
		{
			name: "only api token set",
			args: []string{"--jira-api-token=token123"},
		},
		{
			name: "base url and email set, token missing",
			args: []string{
				"--jira-base-url=https://example.atlassian.net",
				"--jira-email=alice@example.com",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			j := runJiraFlags(t, tc.args)
			tools, err := j.Configure(context.Background())
			gt.Value(t, tools).Nil()
			gt.Value(t, err).NotNil().Required()
		})
	}
}

func TestJiraConfigure_InvalidBaseURL(t *testing.T) {
	j := runJiraFlags(t, []string{
		"--jira-base-url=not a url",
		"--jira-email=alice@example.com",
		"--jira-api-token=token123",
	})
	tools, err := j.Configure(context.Background())
	gt.Value(t, tools).Nil()
	gt.Value(t, err).NotNil().Required()
}

func TestJiraLogAttrsExcludesToken(t *testing.T) {
	j := runJiraFlags(t, []string{
		"--jira-base-url=https://example.atlassian.net",
		"--jira-email=alice@example.com",
		"--jira-api-token=super-secret-token",
	})
	attrs := j.LogAttrs()
	gt.Array(t, attrs).Length(2).Required()

	for _, attr := range attrs {
		gt.String(t, attr.Value.String()).NotEqual("super-secret-token")
	}
	gt.Value(t, attrs[0].Value.String()).Equal("https://example.atlassian.net")
	gt.Value(t, attrs[1].Value.String()).Equal("alice@example.com")
}
