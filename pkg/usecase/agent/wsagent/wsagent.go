package wsagent

import (
	"github.com/m-mizutani/goerr/v2"
)

func validateRequest(req *TurnRequest) error {
	if req == nil {
		return goerr.New("request is nil")
	}
	if req.Session == nil {
		return goerr.New("Session is required")
	}
	if req.Workspace == nil {
		return goerr.New("Workspace is required")
	}
	if req.ActorID == "" {
		return goerr.New("ActorID is required (the mentioning user is the access actor)")
	}
	return nil
}
