package sample

import "context"

type okKernel struct{}

func (okKernel) Serve(context.Context, ...any) error       { return nil }
func (okKernel) GetProcess(context.Context) error          { return nil }
func serveThroughWrapper(context.Context, okKernel) error  { return nil }

type httpServer struct{}

func (httpServer) ListenAndServe() error { return nil }
func (httpServer) Serve() error          { return nil }

// The wrapper is the only sanctioned entry point, and unrelated .Serve calls —
// an HTTP server's — are not agent workers.
func good(ctx context.Context) {
	var agentKernel okKernel
	_ = serveThroughWrapper(ctx, agentKernel)
	_ = agentKernel.GetProcess(ctx)

	var server httpServer
	_ = server.ListenAndServe()
	_ = server.Serve()
}
