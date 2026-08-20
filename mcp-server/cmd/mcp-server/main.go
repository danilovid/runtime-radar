package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/gops/agent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/lib/logger"
	libmetrics "github.com/runtime-radar/runtime-radar/lib/metrics"
	"github.com/runtime-radar/runtime-radar/lib/security"
	"github.com/runtime-radar/runtime-radar/lib/security/cipher"
	"github.com/runtime-radar/runtime-radar/lib/server/healthcheck"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/build"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/client"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/config"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/docs"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/metrics"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/server"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/tools"
	"go.uber.org/automaxprocs/maxprocs"
)

const (
	// TLS cert file name.
	certFile = "cert.pem"
	// TLS key file name.
	keyFile = "key.pem"
	// CA cert file name.
	caFile = "ca.pem"

	// Timeout on graceful shutdown.
	gracefulTimeout = 15 * time.Second
)

var (
	// Channel for stopping the program.
	shutdown = make(chan struct{})
)

func main() {
	cfg := config.New()
	logger.Init(cfg.LogFile, cfg.LogLevel)

	if cfg.Stdio {
		// stdout carries the JSON-RPC stream in stdio mode, so anything else
		// written there would corrupt the protocol.
		log.Logger = log.Logger.Output(os.Stderr)
	}

	log.Info().Str("build_release", build.Release).Str("build_branch", build.Branch).Str("build_commit", build.Commit).Str("build_date", build.Date).Msgf("-> %s started", build.AppName)
	defer log.Info().Msgf("<- %s exited", build.AppName)

	if _, err := maxprocs.Set(maxprocs.Logger(log.Debug().Msgf)); err != nil {
		log.Warn().Msgf("Can't set maxprocs: %v", err)
	}

	authorizer, err := auth.New(cfg.Auth, cfg.TokenKey)
	if err != nil {
		log.Fatal().Msgf("### Failed to instantiate authorizer: %v", err)
	}

	var tlsConfig *tls.Config
	if cfg.TLS {
		tlsConfig, err = security.LoadTLS(caFile, certFile, keyFile)
		if err != nil {
			log.Fatal().Msgf("### Failed to load TLS config: %v", err)
		}
	}

	// With Public API configured, an agent may authenticate with an MCP key
	// issued there instead of a session token; the key is exchanged for the
	// short-lived JWT of the user it belongs to, and the tools managing API
	// tokens are offered.
	var publicAPI *client.PublicAPI

	if cfg.PublicAPIURL != "" {
		publicAPI = client.NewPublicAPI(cfg.PublicAPIURL, tlsConfig)

		tokenKey, keyErr := cipher.ParseKey(cfg.TokenKey)
		if keyErr != nil && cfg.Auth {
			log.Fatal().Msgf("### Failed to parse token key: %v", keyErr)
		}

		exchanger, keyErr := auth.NewKeyExchanger(cfg.PublicAPIURL, tlsConfig, tokenKey)
		if keyErr != nil {
			log.Fatal().Msgf("### Failed to instantiate mcp key exchanger: %v", keyErr)
		}

		authorizer = authorizer.WithKeyExchanger(exchanger)

		log.Info().Msgf("MCP keys are accepted, exchanged through %s", cfg.PublicAPIURL)
	}

	clients, closeClients, err := client.New(cfg.HistoryAPIGRPCAddr, cfg.EventProcessorGRPCAddr, cfg.PolicyEnforcerGRPCAddr, tlsConfig)
	if err != nil {
		log.Fatal().Msgf("### Failed to connect to gRPC services: %v", err)
	}
	defer closeClients()

	index, err := docs.Load(cfg.DocsDir)
	if err != nil {
		log.Fatal().Msgf("### Failed to index documentation: %v", err)
	}
	log.Info().Msgf("Indexed %d documentation files from %s", index.Len(), cfg.DocsDir)

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    build.AppName,
		Title:   "Runtime Radar",
		Version: build.Release,
	}, &mcp.ServerOptions{
		Instructions: instructions,
	})

	tools.Register(mcpServer, &tools.Deps{
		Clients:     clients,
		Docs:        index,
		PublicAPI:   publicAPI,
		Auth:        authorizer,
		StaticToken: cfg.AuthToken,
	})

	if cfg.Stdio {
		runStdio(mcpServer)
		return
	}

	go signalListener()

	if err := agent.Listen(agent.Options{Addr: cfg.GopsAddr}); err != nil {
		log.Fatal().Msgf("### Failed to start gops agent: %v", err)
	}
	defer agent.Close()

	registry, err := libmetrics.NewRegistry(build.AppName, cfg.ClusterName, metrics.Collectors()...)
	if err != nil {
		log.Fatal().Msgf("### Failed to register metrics: %v", err)
	}

	// Create and Run the instrumentation HTTP server for probes, metrics, etc.
	iSrv := server.NewInstrumentation(cfg.InstrumentationAddr, registry)
	go func() {
		if err := iSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Msgf("### Can't serve instrumentation HTTP requests: %v", err)
		}
	}()
	log.Info().Msgf("Instrumentation HTTP server listening at %v", cfg.InstrumentationAddr)

	httpSrv := server.New(cfg.ListenHTTPAddr, mcpServer, authorizer, tlsConfig)

	go func() {
		if cfg.TLS {
			// httpSrv already has TLS config
			if err := httpSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal().Msgf("### Can't serve HTTP requests: %v", err)
			}
		} else {
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal().Msgf("### Can't serve HTTP requests: %v", err)
			}
		}
	}()
	log.Info().Msgf("MCP Streamable HTTP server listening at %v", httpSrv.Addr)

	if !authorizer.Enabled() {
		log.Warn().Msg("Auth is disabled: every MCP client has full read access to runtime events")
	}

	healthcheck.SetReady() // <-- turn on ready status for k8s

	<-shutdown

	ctx, cancel := context.WithTimeout(context.Background(), gracefulTimeout)
	defer cancel()

	log.Info().Msg("MCP HTTP server stopping gracefully")
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Error")
	}

	log.Info().Msg("Instrumentation HTTP server stopping gracefully")
	_ = iSrv.Shutdown(ctx)
}

// runStdio serves a single client over stdin/stdout. It is meant for local
// debugging: there is no way to authenticate per request, so outgoing calls
// carry the token from the configuration, if any.
func runStdio(mcpServer *mcp.Server) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Info().Msg("Serving MCP over stdio")

	if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal().Msgf("### Can't serve MCP over stdio: %v", err)
	}
}

func signalListener() {
	defer close(shutdown)

	sigTerm := make(chan os.Signal, 10)
	sigIgnore := make(chan os.Signal, 10)

	signal.Notify(sigTerm, os.Interrupt, syscall.SIGTERM)
	signal.Notify(sigIgnore, syscall.SIGHUP)

	// Wait for signals
	for {
		select {
		case s := <-sigTerm:
			log.Info().Str("signal", s.String()).Msg("Signal caught, terminating")
			return
		case s := <-sigIgnore:
			// Ignoring, like with "nohup"
			log.Info().Str("signal", s.String()).Msg("Signal caught, ignoring")
		}
	}
}
