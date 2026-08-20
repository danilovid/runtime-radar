// Package client dials the gRPC services the MCP tools read from. The clients
// carry no credentials of their own: the caller's token travels in the context
// of every call (see pkg/auth), so that downstream RBAC and audit apply to the
// human behind the agent.
package client

import (
	"crypto/tls"
	"fmt"

	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// MaxRecvMsgSize is the maximum size of a gRPC response the service accepts.
const MaxRecvMsgSize = 10 * 1024 * 1024 // 10MB

// New dials address and returns the History API and Event Processor clients
// sharing that connection's settings, along with a function closing both.
func New(historyAddr, processorAddr string, tlsConfig *tls.Config) (*Clients, func() error, error) {
	historyConn, err := dial(historyAddr, tlsConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("can't connect to History API: %w", err)
	}

	processorConn, err := dial(processorAddr, tlsConfig)
	if err != nil {
		_ = historyConn.Close()
		return nil, nil, fmt.Errorf("can't connect to Event Processor: %w", err)
	}

	clients := &Clients{
		RuntimeHistory: history_api.NewRuntimeHistoryClient(historyConn),
		RuntimeStats:   history_api.NewRuntimeStatsClient(historyConn),
		Detectors:      processor_api.NewDetectorControllerClient(processorConn),
	}

	closeAll := func() error {
		err := historyConn.Close()
		if closeErr := processorConn.Close(); err == nil {
			err = closeErr
		}

		return err
	}

	return clients, closeAll, nil
}

// Clients is the set of gRPC clients the tools are built on.
type Clients struct {
	RuntimeHistory history_api.RuntimeHistoryClient
	RuntimeStats   history_api.RuntimeStatsClient
	Detectors      processor_api.DetectorControllerClient
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
