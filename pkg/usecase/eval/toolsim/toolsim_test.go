package toolsim_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/toolsim"
)

type fakeCompleter struct{ out string }

func (f *fakeCompleter) Complete(_ context.Context, _, _ string, _ *gollem.Parameter) (string, error) {
	return f.out, nil
}

func TestSlackSearch_GeneratesAndRecords(t *testing.T) {
	rec := toolsim.NewRecorder()
	svc := toolsim.SlackSearch(&fakeCompleter{out: "Found 2 threads about portal 503."}, "background here", rec)

	res, err := svc.SearchMessages(context.Background(), "portal 503", slacktool.SearchOptions{})
	gt.NoError(t, err)
	gt.Number(t, res.Total).Equal(1)
	gt.A(t, res.Messages).Length(1)
	gt.V(t, res.Messages[0].Text).Equal("Found 2 threads about portal 503.")

	recs := rec.Records()
	gt.A(t, recs).Length(1)
	gt.V(t, recs[0].Tool).Equal(toolsim.ToolSlackSearch)
	gt.V(t, recs[0].Mode).Equal("sim")
	gt.Number(t, recs[0].Seq).Equal(1)
}

func TestSlackSearch_EmptyBackgroundYieldsNoResults(t *testing.T) {
	rec := toolsim.NewRecorder()
	svc := toolsim.SlackSearch(&fakeCompleter{out: "should not be used"}, "   ", rec)
	res, err := svc.SearchMessages(context.Background(), "q", slacktool.SearchOptions{})
	gt.NoError(t, err)
	gt.Number(t, res.Total).Equal(0)
	gt.A(t, res.Messages).Length(0)
	gt.A(t, rec.Records()).Length(1) // still recorded
}

func TestNotionSearch_GeneratesItem(t *testing.T) {
	rec := toolsim.NewRecorder()
	cli := toolsim.NotionSearch(&fakeCompleter{out: "Runbook: Portal Incident Response\nsteps..."}, "notion bg", rec)

	res, err := cli.Search(context.Background(), "portal", notiontool.SearchOptions{})
	gt.NoError(t, err)
	gt.A(t, res.Items).Length(1)
	gt.V(t, res.Items[0].Title).Equal("Runbook: Portal Incident Response")

	md, err := cli.GetPageMarkdown(context.Background(), "sim-page")
	gt.NoError(t, err)
	gt.V(t, md.PageID).Equal("sim-page")
	gt.V(t, md.Markdown).NotEqual("")

	gt.A(t, rec.Records()).Length(2)
}

func TestRecorder_SequenceOrder(t *testing.T) {
	rec := toolsim.NewRecorder()
	rec.Record("a", "sim", nil, nil)
	rec.Record("b", "live", nil, nil)
	recs := rec.Records()
	gt.A(t, recs).Length(2)
	gt.Number(t, recs[0].Seq).Equal(1)
	gt.Number(t, recs[1].Seq).Equal(2)
	gt.V(t, recs[1].Mode).Equal("live")
}

func TestSimulatableTools(t *testing.T) {
	got := toolsim.SimulatableTools()
	gt.A(t, got).Length(2)
}
