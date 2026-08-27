package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/gops/agent"
	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/build"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/client"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/config"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/database"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/monitor"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/monitor/updater"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/processor"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/server"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/service"
	"github.com/runtime-radar/runtime-radar/lib/logger"
	"github.com/runtime-radar/runtime-radar/lib/rabbit"
	"github.com/runtime-radar/runtime-radar/lib/security"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"github.com/runtime-radar/runtime-radar/lib/server/healthcheck"
	"github.com/runtime-radar/runtime-radar/lib/server/interceptor"
	notifier_api "github.com/runtime-radar/runtime-radar/notifier/api"
	enforcer_api "github.com/runtime-radar/runtime-radar/policy-enforcer/api"
	"go.uber.org/automaxprocs/maxprocs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"gorm.io/gorm"
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

	log.Info().Str("build_release", build.Release).Str("build_branch", build.Branch).Str("build_commit", build.Commit).Str("build_date", build.Date).Msgf("-> %s started", build.AppName)
	defer log.Info().Msgf("<- %s exited", build.AppName)

	if _, err := maxprocs.Set(maxprocs.Logger(log.Debug().Msgf)); err != nil {
		log.Warn().Msgf("Can't set maxprocs: %v", err)
	}

	if err := agent.Listen(agent.Options{
		Addr: cfg.GopsAddr,
	}); err != nil {
		log.Fatal().Msgf("### Failed to start gops agent: %v", err)
	}
	defer agent.Close()

	go signalListener()

	lis, err := net.Listen("tcp", cfg.ListenGRPCAddr)
	if err != nil {
		log.Fatal().Msgf("### Failed to listen: %v", err)
	}

	var verifier jwt.Verifier
	var tokenKey []byte
	if cfg.Auth {
		verifier, tokenKey, err = jwt.NewKeyVerifier(cfg.TokenKey)
		if err != nil {
			log.Fatal().Msgf("### Failed to instantiate key verifier: %v", err)
		}
	}

	// Connect to DB
	db, closeDB, err := database.New(cfg.PostgresAddr, cfg.PostgresDB, cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresSSLMode, cfg.PostgresSSLCheckCert)
	if err != nil {
		log.Fatal().Msgf("### Failed to open DB: %v", err)
	}
	defer closeDB()

	// Recreate DB from scratch, or migrate automatically when needed
	if err := database.Migrate(db, cfg.NewDB); err != nil {
		log.Fatal().Msgf("### Failed to migrate DB: %v", err)
	}

	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(interceptor.Recovery, interceptor.Correlation),
		grpc.MaxRecvMsgSize(server.MaxRecvMsgSize),
	}

	var tlsConfig *tls.Config
	if cfg.TLS {
		// Load TLS config
		tlsConfig, err = security.LoadTLS(caFile, certFile, keyFile)
		if err != nil {
			log.Fatal().Msgf("### Failed to load TLS config: %v", err)
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	kyverno, closeKyverno, err := getKyverno(db, cfg.KyvernoNamespace, cfg.KyvernoEventsBuffer)
	if err != nil {
		log.Fatal().Msgf("### Can't initialize Kyverno: %v", err)
	}
	defer closeKyverno()
	log.Info().Msgf("Connected to kyverno version %s in namespace %s", kyverno.Version, cfg.KyvernoNamespace)

	enforcer, closePE, err := client.NewPolicyEnforcer(cfg.PolicyEnforcerGRPCAddr, tlsConfig, tokenKey)
	if err != nil {
		log.Fatal().Msgf("### Failed to connect to Policy Enforcer: %v", err)
	}
	defer closePE()

	notifier, closeNotifier, err := client.NewNotifier(cfg.NotifierGRPCAddr, tlsConfig, tokenKey)
	if err != nil {
		log.Fatal().Msgf("### Failed to connect to Notifier: %v", err)
	}
	defer closeNotifier()

	historyMB, err := rabbit.NewMessageBroker(cfg.RabbitAddr, cfg.RabbitUser, cfg.RabbitPassword, cfg.RabbitHistoryQueue)
	if err != nil {
		log.Fatal().Msgf("### Failed to initialize Message Broker: %v", err)
	}
	defer historyMB.Close()

	grpcSrv := grpc.NewServer(opts...)
	configSvc := composeServices(db, kyverno, verifier, cfg.Auth)

	api.RegisterConfigControllerServer(grpcSrv, configSvc)

	// Register reflection service on gRPC server
	reflection.Register(grpcSrv)

	// Create and Run the instrumentation HTTP server for probes, etc.
	iSrv := server.NewInstrumentation(cfg.InstrumentationAddr)
	go func() {
		if err := iSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Msgf("### Can't serve instrumentation HTTP requests: %v", err)
		}
	}()
	log.Info().Msgf("Instrumentation HTTP server listening at %v", cfg.InstrumentationAddr)

	// Run gRPC server
	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatal().Msgf("### Can't serve gRPC requests: %v", err)
		}
	}()
	log.Info().Msgf("gRPC server listening at %v", lis.Addr())

	httpSrv, err := server.New(cfg.ListenHTTPAddr, cfg.ListenGRPCAddr, tlsConfig)
	if err != nil {
		log.Fatal().Msgf("### Can't setup HTTP server: %v", err)
	}

	// Run HTTP server
	go func() {
		if cfg.TLS {
			if err := httpSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Fatal().Msgf("### Can't serve HTTP requests: %v", err)
			}
		} else {
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal().Msgf("### Can't serve HTTP requests: %v", err)
			}
		}
	}()
	log.Info().Msgf("HTTP server listening at %v", httpSrv.Addr)

	// Run kyverno monitor
	go func() {
		if err := kyverno.Run(shutdown); err != nil {
			log.Error().Msgf("Failed to run monitor: %v", err)
			// Signal the whole system to stop if it's not yet the case, such as when we discover that
			// Kyverno was uninstalled from the cluster.
			closeIfNotClosed(shutdown)
		}
	}()

	// Run events processor
	go eventsProcessor(kyverno, historyMB, enforcer, notifier)

	// Check kyverno config for periodic updates
	go kyvernoUpdater(cfg.ConfigUpdateInterval, kyverno, db)

	healthcheck.SetReady() // <-- turn on ready status for k8s

	<-shutdown

	log.Info().Msg("gRPC server stopping gracefully")
	grpcSrv.GracefulStop()

	ctx, cancel := context.WithTimeout(context.Background(), gracefulTimeout)
	defer cancel()

	log.Info().Msg("HTTP server stopping gracefully")
	httpSrv.Shutdown(ctx) // we don't care about errors here

	log.Info().Msg("Instrumentation HTTP server stopping gracefully")
	_ = iSrv.Shutdown(ctx)
}

func getKyverno(db *gorm.DB, namespace string, bufferSize int) (*monitor.Kyverno, func() error, error) {
	ctx := context.Background()

	kyvernoRepo := &database.ConfigDatabase{db}
	kyvernoConfig, err := kyvernoRepo.GetLast(ctx, true) // preload is on
	if err != nil {
		return nil, nil, err
	}

	kyverno, closeKyverno, err := monitor.NewKyverno(namespace, bufferSize)
	if err != nil {
		return nil, nil, err
	}

	if err := kyverno.Init(ctx, kyvernoConfig); err != nil {
		return nil, nil, err
	}

	return kyverno, closeKyverno, nil
}

func kyvernoUpdater(interval time.Duration, kyverno *monitor.Kyverno, db *gorm.DB) {
	u := updater.Updater{
		Interval:         interval,
		ConfigRepository: &database.ConfigDatabase{db},
		Monitor:          kyverno,
	}

	u.Run(shutdown)
}

func eventsProcessor(kyverno *monitor.Kyverno, mb *rabbit.MessageBroker, enforcer enforcer_api.EnforcerClient, notifier notifier_api.NotifierClient) {
	p := &processor.Processor{
		Monitor:  kyverno,
		History:  mb,
		Enforcer: enforcer,
		Notifier: notifier,
	}

	p.Run(shutdown)
}

func composeServices(db *gorm.DB, monitor monitor.Monitor, verifier jwt.Verifier, isAuth bool) (configSvc api.ConfigControllerServer) {
	configSvc = &service.ConfigGeneric{
		ConfigRepository: &database.ConfigDatabase{db},
		Monitor:          monitor,
	}

	if isAuth {
		configSvc = &service.ConfigAuth{
			ConfigControllerServer: configSvc,
			Verifier:               verifier,
		}
	}

	configSvc = &service.ConfigLogging{configSvc}

	return
}

func signalListener() {
	defer closeIfNotClosed(shutdown)

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

// closeIfNotClosed closes ch in case if it wasn't closed already. For doing this it tries to read from channel and analyses
// result indicator. It panics if ch is nil.
func closeIfNotClosed[T any](ch chan T) {
	if ch == nil {
		panic("can't close nil channel")
	}

	ok := false

	defer func() {
		if ok {
			close(ch)
		}
	}()

	select {
	case _, ok = <-ch:
	default:
		ok = true
	}
}
