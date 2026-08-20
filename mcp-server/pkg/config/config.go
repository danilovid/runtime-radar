package config

import (
	"flag"

	"github.com/runtime-radar/runtime-radar/lib/config"
)

// Config represents system configuration.
type Config struct {
	LogLevel               string // log level can be INFO, WARN, ERROR, FATAL, DEBUG or ALL
	LogFile                string // path to log file
	Stdio                  bool   // serve MCP over stdin/stdout instead of HTTP
	ListenHTTPAddr         string // address "[host]:port" that Streamable HTTP MCP server should be listening on
	InstrumentationAddr    string // address "[host]:port" that instrumentation server should be listening for health checks and metrics
	GopsAddr               string // gops listen address
	TLS                    bool   // is TLS enabled?
	TokenKey               string // key for jwt token
	Auth                   bool   // is auth enabled?
	AuthToken              string // jwt token to authenticate outgoing calls with when the client presents none
	HistoryAPIGRPCAddr     string // History API address in host[:port] format
	EventProcessorGRPCAddr string // Event Processor address in host[:port] format
	DocsDir                string // directory to load product documentation from
	ClusterName            string // cluster name to label metrics with
}

// New reads config from environment and returns pointer to a new Config.
func New() *Config {
	c := &Config{}

	flag.StringVar(&c.LogLevel, "logLevel", config.LookupEnvString("LOG_LEVEL", "TRACE"), "Set log level (DEBUG, INFO, WARN, ERROR, FATAL, any other value means TRACE).")
	flag.StringVar(&c.LogFile, "logFile", config.LookupEnvString("LOG_FILE", ""), "Tail logs to this file. Leave empty to log to stdout without tailing.")
	flag.BoolVar(&c.Stdio, "stdio", config.LookupEnvBool("STDIO", false), "Set to serve MCP over stdin/stdout instead of Streamable HTTP. Intended for local debugging only.")
	flag.StringVar(&c.ListenHTTPAddr, "listenHTTPAddr", config.LookupEnvString("LISTEN_HTTP_ADDR", ":9000"), `Address in form of "[host]:port" that HTTP server should be listening on.`)
	flag.StringVar(&c.InstrumentationAddr, "listenInstrumentationAddr", config.LookupEnvString("LISTEN_INSTRUMENTATION_ADDR", ":9090"), `Address in form of "[host]:port" that instrumentation HTTP server should be listening on.`)
	flag.StringVar(&c.GopsAddr, "listenGopsAddr", config.LookupEnvString("LISTEN_GOPS_ADDR", "127.0.0.1:7000"), `Address in form of "[host]:port" that gops agent should be listening on. It's not safe to listen to interfaces other than loopback in production.`)
	flag.BoolVar(&c.TLS, "tls", config.LookupEnvBool("TLS", false), "Set to enable TLS.")
	flag.StringVar(&c.TokenKey, "tokenKey", config.LookupEnvString("TOKEN_KEY", ""), "Hex encoded token key to verify jwt token. Supported key sizes are 16, 24 and 32 bytes.")
	flag.BoolVar(&c.Auth, "auth", config.LookupEnvBool("AUTH", false), "Set to enable JWT auth.")
	flag.StringVar(&c.AuthToken, "authToken", config.LookupEnvString("AUTH_TOKEN", ""), "JWT token to authenticate outgoing calls with when a client presents none of its own: every call in stdio mode, and HTTP calls when auth is disabled. With auth enabled the HTTP transport always uses the caller's own token.")
	flag.StringVar(&c.HistoryAPIGRPCAddr, "historyAPIGRPCAddr", config.LookupEnvString("HISTORY_API_GRPC_ADDR", "127.0.0.1:8000"), "History API gRPC address in host[:port] format.")
	flag.StringVar(&c.EventProcessorGRPCAddr, "eventProcessorGRPCAddr", config.LookupEnvString("EVENT_PROCESSOR_GRPC_ADDR", "127.0.0.1:8000"), "Event Processor gRPC address in host[:port] format.")
	flag.StringVar(&c.DocsDir, "docsDir", config.LookupEnvString("DOCS_DIR", "docs"), "Set directory to load product documentation (*.md) from.")
	flag.StringVar(&c.ClusterName, "clusterName", config.LookupEnvString("CLUSTER_NAME", ""), "Set cluster name to label metrics with.")

	flag.Parse()

	return c
}
