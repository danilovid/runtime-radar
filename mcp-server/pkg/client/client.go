// Package client dials the gRPC services the MCP tools read from and write to.
// The clients
// carry no credentials of their own: the caller's token travels in the context
// of every call (see pkg/auth), so that downstream RBAC and audit apply to the
// human behind the agent.
package client

import (
	"crypto/tls"
	"fmt"

	monitor_api "github.com/runtime-radar/runtime-radar/admission-monitor/api"
	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	enf_api "github.com/runtime-radar/runtime-radar/policy-enforcer/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// MaxRecvMsgSize is the maximum size of a gRPC response the service accepts.
const MaxRecvMsgSize = 10 * 1024 * 1024 // 10MB

// New dials the services the tools read from and write to, and returns their
// clients along with a function closing every connection.
func New(historyAddr, processorAddr, enforcerAddr, admissionAddr string, tlsConfig *tls.Config) (*Clients, func() error, error) {
	conns := make([]*grpc.ClientConn, 0, 4)

	closeAll := func() error {
		var err error
		for _, conn := range conns {
			if closeErr := conn.Close(); err == nil {
				err = closeErr
			}
		}

		return err
	}

	historyConn, err := dial(historyAddr, tlsConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("can't connect to History API: %w", err)
	}
	conns = append(conns, historyConn)

	processorConn, err := dial(processorAddr, tlsConfig)
	if err != nil {
		_ = closeAll()

		return nil, nil, fmt.Errorf("can't connect to Event Processor: %w", err)
	}
	conns = append(conns, processorConn)

	enforcerConn, err := dial(enforcerAddr, tlsConfig)
	if err != nil {
		_ = closeAll()

		return nil, nil, fmt.Errorf("can't connect to Policy Enforcer: %w", err)
	}
	conns = append(conns, enforcerConn)

	admissionConn, err := dial(admissionAddr, tlsConfig)
	if err != nil {
		_ = closeAll()

		return nil, nil, fmt.Errorf("can't connect to Admission Monitor: %w", err)
	}
	conns = append(conns, admissionConn)

	clients := &Clients{
		RuntimeHistory:   history_api.NewRuntimeHistoryClient(historyConn),
		RuntimeStats:     history_api.NewRuntimeStatsClient(historyConn),
		AdmissionHistory: history_api.NewAdmissionHistoryClient(historyConn),
		Detectors:        processor_api.NewDetectorControllerClient(processorConn),
		Rules:            enf_api.NewRuleControllerClient(enforcerConn),
		AdmissionConfig:  monitor_api.NewConfigControllerClient(admissionConn),
	}

	return clients, closeAll, nil
}

// Clients is the set of gRPC clients the tools are built on.
type Clients struct {
	RuntimeHistory history_api.RuntimeHistoryClient
	RuntimeStats   history_api.RuntimeStatsClient
	Detectors      processor_api.DetectorControllerClient
	Rules          enf_api.RuleControllerClient

	// AdmissionHistory reads the events Kyverno produced, AdmissionConfig reads
	// and writes the set of Kyverno policies admission-monitor keeps applied.
	AdmissionHistory history_api.AdmissionHistoryClient
	AdmissionConfig  monitor_api.ConfigControllerClient
}

func dial(address string, tlsConfig *tls.Config) (*grpc.ClientConn, error) {
	var creds credentials.TransportCredentials
	if tlsConfig != nil {
		creds = credentials.NewTLS(tlsConfig)
	} else {
		creds = insecure.NewCredentials()
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxRecvMsgSize)),
		grpc.WithNoProxy(), // These clients are used for internal interactions where proxy settings are never required.
	}

	return grpc.NewClient(address, opts...)
}
