package client

import (
	"crypto/tls"

	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/build"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func NewAdmissionHistory(address string, tlsConfig *tls.Config, tokenKey []byte) (history_api.AdmissionHistoryClient, func() error, error) {
	var creds credentials.TransportCredentials
	if tlsConfig != nil {
		creds = credentials.NewTLS(tlsConfig)
	} else {
		creds = insecure.NewCredentials()
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(server.MaxRecvMsgSize)),
		grpc.WithNoProxy(), // This client is used for internal interactions where proxy settings are never required.
	}

	if len(tokenKey) > 0 {
		rp := &jwt.RolePermissions{
			Events: &jwt.Permission{
				Actions: []jwt.Action{jwt.ActionRead},
			},
		}

		creds, err := jwt.GeneratePerRPCCredentials(tokenKey, build.AppName, rp)
		if err != nil {
			return nil, nil, err
		}

		opts = append(opts, grpc.WithPerRPCCredentials(creds))
	}

	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, nil, err
	}

	client := history_api.NewAdmissionHistoryClient(conn)

	return client, conn.Close, nil
}
