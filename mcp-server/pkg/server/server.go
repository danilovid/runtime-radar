// Package server wires the MCP transports and the instrumentation endpoint of
// the service into HTTP servers.
package server

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/justinas/alice"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/lib/server/healthcheck"
	"github.com/runtime-radar/runtime-radar/lib/server/middleware"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
)

const (
	readTimeout  = 5 * time.Minute
	writeTimeout = 5 * time.Minute

	// Path the Streamable HTTP transport is served at. It matches the /mcp*
	// route of the reverse proxy, which forwards the path unchanged.
	mcpPath = "/mcp"
)

// New constructs the HTTP server exposing the MCP Streamable HTTP transport.
//
// The transport is stateless: every request carries its own credentials and no
// session state is kept, so the service can be scaled out without any affinity
// between a client and a replica.
func New(httpAddr string, mcpServer *mcp.Server, authorizer *auth.Authorizer, tlsConfig *tls.Config) *http.Server {
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)

	mux := http.NewServeMux()
	mux.Handle(mcpPath, handler)
	mux.Handle(mcpPath+"/", handler)

	h := alice.New(
		requestLog,
		middleware.Recovery,
		authenticated(authorizer),
	).Then(mux)

	return &http.Server{
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		Addr:         httpAddr,
		Handler:      h,
		TLSConfig:    tlsConfig,
	}
}

// NewInstrumentation constructs the HTTP server serving probes and metrics.
func NewInstrumentation(listenAddress string, gatherer prometheus.Gatherer) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/ready", healthcheck.ReadyHandler)
	mux.HandleFunc("/live", healthcheck.LiveHandler)
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))

	h := alice.New(
		middleware.Log,
		middleware.Recovery,
	).Then(mux)

	return &http.Server{
		Addr:    listenAddress,
		Handler: h,
	}
}

// authenticated rejects a request carrying no usable token before it reaches
// the JSON-RPC layer, so that an unauthenticated client gets a plain 401
// instead of a tool error. Per-tool permissions are checked later, once the
// call names the tool it wants.
func authenticated(authorizer *auth.Authorizer) alice.Constructor {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !authorizer.Enabled() {
				next.ServeHTTP(w, r)
				return
			}

			token := auth.TokenFromHeader(r.Header.Get("Authorization"))
			if _, _, err := authorizer.Authorize(r.Context(), token); err != nil {
				log.Warn().Err(err).Str("remote", r.RemoteAddr).Msg("Rejecting unauthenticated MCP request")

				w.Header().Set("WWW-Authenticate", `Bearer realm="runtime-radar"`)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// requestLog logs the request line only. The shared logging middleware dumps
// every header at debug level, which would put callers' bearer tokens in the
// log file.
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug().Str("remote", r.RemoteAddr).Msgf("%s %s request", r.Method, r.URL.Path)

		next.ServeHTTP(w, r)
	})
}
