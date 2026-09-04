package rest

import (
	"context"

	"github.com/gabinante/flywheel/api/generated"
	apierrors "github.com/gabinante/flywheel/internal/errors"
)

// GetHarnessStatus reports install and sign-in state for Claude Code and Codex.
func (s *StrictServer) GetHarnessStatus(ctx context.Context, _ generated.GetHarnessStatusRequestObject) (generated.GetHarnessStatusResponseObject, error) {
	if err := requireAgent(ctx, s.AgentStore); err != nil {
		return generated.GetHarnessStatus401JSONResponse(seToGen(err)), nil
	}
	if s.HarnessRunner == nil {
		return nil, apierrors.New(apierrors.CodeInternal, "harness runner not configured", false)
	}
	var out struct {
		Items []generated.HarnessStatus `json:"items"`
	}
	out.Items = []generated.HarnessStatus{}
	for _, st := range s.HarnessRunner.Statuses(ctx) {
		g := generated.HarnessStatus{Harness: st.Harness, Bin: st.Bin, Installed: st.Installed, LoggedIn: st.LoggedIn, LoginCommand: st.LoginCommand, CheckedAt: st.CheckedAt}
		if st.ResolvedPath != "" {
			v := st.ResolvedPath
			g.ResolvedPath = &v
		}
		if st.Version != "" {
			v := st.Version
			g.Version = &v
		}
		if st.Account != "" {
			v := st.Account
			g.Account = &v
		}
		if st.Detail != "" {
			v := st.Detail
			g.Detail = &v
		}
		out.Items = append(out.Items, g)
	}
	return generated.GetHarnessStatus200JSONResponse(out), nil
}
