// Package agent contains the Slack-independent pieces shared by the agent
// hosts (`casebound`, `threadcase`, `proposal`, `wsagent`, `job`): the toolset
// resolver every host builds its sub-agent tools from, and the LLM call counter
// the plan-execute runtime reports through.
//
// Slack SDK / pkg/service/slack imports are forbidden inside this package;
// hosts communicate with their Slack side through their own small Host
// interfaces (e.g. proposal.Host, threadcase.Host).
package agent
