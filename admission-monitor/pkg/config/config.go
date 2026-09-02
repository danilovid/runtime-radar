package config

import (
	"flag"
	"time"

	"github.com/runtime-radar/runtime-radar/lib/config"
)

// Config represents system configuration.
type Config struct {
	NewDB                  bool          // forces recreation of DB
	PostgresAddr           string        // Postgres address in host[:port] format
	PostgresDB             string        // Postgres db name
	PostgresUser           string        // Postgres user
	PostgresPassword       string        // Postgres password
	PostgresSSLMode        bool          // Postgres SSL mode
	PostgresSSLCheckCert   bool          // Check postgres SSL cert
	LogLevel               string        // log level can be INFO, WARN, ERROR, FATAL, DEBUG or ALL
	LogFile                string        // path to log file
	ListenGRPCAddr         string        // address "[host]:port" that server should be listening on
	ListenHTTPAddr         string        // address "[host]:port" that server should be listening for health checks
	InstrumentationAddr    string        // address "[host]:port" that instrumentation server should be listening for health checks and metrics
	TLS                    bool          // is TLS enabled?
	TokenKey               string        // key for jwt token
	Auth                   bool          // is auth enabled?
	KyvernoNamespace       string        // namespace Kyverno is installed in, its own events are ignored
	KyvernoEventsBuffer    int           // size of Kyverno events buffer
	ConfigUpdateInterval   time.Duration // interval for Kyverno policies periodic update check
	PolicyEnforcerGRPCAddr string        // Policy Enforcer address in host[:port] format
	NotifierGRPCAddr       string        // Notifier address in host[:port] format
	RabbitAddr             string        // RabbitMQ address in host[:port] format
	RabbitUser             string        // RabbitMQ user
	RabbitPassword         string        // RabbitMQ password
	RabbitHistoryQueue     string        // RabbitMQ queue name to publish admission events to
	GopsAddr               string        // gops listen address
}

// New reads config from environment and returns pointer to a new Config.
func New() *Config {
	c := &Config{}

	flag.BoolVar(&c.NewDB, "newDB", config.LookupEnvBool("NEW_DB", false), "Set this flag to recreate DB from scratch. ALL EXISTING DATA WILL BE LOST.")
	flag.StringVar(&c.PostgresAddr, "postgresAddr", config.LookupEnvString("POSTGRES_ADDR", "127.0.0.1:5432"), "Set PostgreSQL address as host:port, where port is optional (without TLS).")
	flag.StringVar(&c.PostgresDB, "postgresDB", config.LookupEnvString("POSTGRES_DB", "cs"), "Set PostgreSQL DB.")
	flag.StringVar(&c.PostgresUser, "postgresUser", config.LookupEnvString("POSTGRES_USER", "cs"), "Set PostgreSQL user.")
	flag.StringVar(&c.PostgresPassword, "postgresPassword", config.LookupEnvString("POSTGRES_PASSWORD", "cs"), "Set PostgreSQL password.")
	flag.BoolVar(&c.PostgresSSLMode, "postgresSSLMode", config.LookupEnvBool("POSTGRES_SSL_MODE", false), "Set to enable PostgreSQL SSL mode.")
	flag.BoolVar(&c.PostgresSSLCheckCert, "postgresSSLCheckCert", config.LookupEnvBool("POSTGRES_SSL_CHECK_CERT", false), "Set to check PostgreSQL SSL cert.")
	flag.StringVar(&c.LogLevel, "logLevel", config.LookupEnvString("LOG_LEVEL", "TRACE"), "Set log level (DEBUG, INFO, WARN, ERROR, FATAL, any other value means TRACE).")
	flag.StringVar(&c.LogFile, "logFile", config.LookupEnvString("LOG_FILE", ""), "Tail logs to this file. Leave empty to log to stdout without tailing.")
	flag.StringVar(&c.ListenGRPCAddr, "listenGRPCAddr", config.LookupEnvString("LISTEN_GRPC_ADDR", ":8000"), `Address in form of "[host]:port" that gRPC server should be listening on.`)
	flag.StringVar(&c.ListenHTTPAddr, "listenHTTPAddr", config.LookupEnvString("LISTEN_HTTP_ADDR", ":9000"), `Address in form of "[host]:port" that HTTP server should be listening on.`)
	flag.BoolVar(&c.TLS, "tls", config.LookupEnvBool("TLS", false), "Set to enable TLS.")
	flag.StringVar(&c.TokenKey, "tokenKey", config.LookupEnvString("TOKEN_KEY", ""), "Hex encoded token key to verify jwt token. Supported key sizes are 16, 24 and 32 bytes.")
	flag.BoolVar(&c.Auth, "auth", config.LookupEnvBool("AUTH", false), "Set to enable JWT auth.")
	flag.StringVar(&c.KyvernoNamespace, "kyvernoNamespace", config.LookupEnvString("KYVERNO_NAMESPACE", "kyverno"), "Set the namespace Kyverno is installed in.")
	flag.IntVar(&c.KyvernoEventsBuffer, "kyvernoEventsBuffer", config.LookupEnvInt("KYVERNO_EVENTS_BUFFER", 1000), "Set size of Kyverno events buffer.")
	flag.DurationVar(&c.ConfigUpdateInterval, "configUpdateInterval", config.LookupEnvDuration("CONFIG_UPDATE_INTERVAL", 30*time.Second), "Set interval for Kyverno policies periodic update check.")
	flag.StringVar(&c.PolicyEnforcerGRPCAddr, "policyEnforcerGRPCAddr", config.LookupEnvString("POLICY_ENFORCER_GRPC_ADDR", "127.0.0.1:10000"), "Policy Enforcer gRPC address in host[:port] format.")
	flag.StringVar(&c.NotifierGRPCAddr, "notifierGRPCAddr", config.LookupEnvString("NOTIFIER_GRPC_ADDR", "127.0.0.1:10000"), "Notifier gRPC address in host[:port] format.")
	flag.StringVar(&c.RabbitAddr, "rabbitAddr", config.LookupEnvString("RABBIT_ADDR", "rabbitmq.default:5672"), "Set RabbitMQ address in host[:port] format.")
	flag.StringVar(&c.RabbitUser, "rabbitUser", config.LookupEnvString("RABBIT_USER", "guest"), "Set RabbitMQ user.")
	flag.StringVar(&c.RabbitPassword, "rabbitPassword", config.LookupEnvString("RABBIT_PASSWORD", "guest"), "Set RabbitMQ password.")
	flag.StringVar(&c.RabbitHistoryQueue, "rabbitHistoryQueue", config.LookupEnvString("RABBIT_ADMISSION_EVENTS_QUEUE", "admission_events"), "Set RabbitMQ queue name to publish admission events to.")
	flag.StringVar(&c.GopsAddr, "listenGopsAddr", config.LookupEnvString("LISTEN_GOPS_ADDR", "127.0.0.1:7000"), `Address in form of "[host]:port" that gops agent should be listening on. It's not safe to listen to interfaces other than loopback in production.`)
	flag.StringVar(&c.InstrumentationAddr, "listenInstrumentationAddr", config.LookupEnvString("LISTEN_INSTRUMENTATION_ADDR", ":9090"), `Address in form of "[host]:port" that instrumentation HTTP server should be listening on.`)

	flag.Parse()

	return c
}
