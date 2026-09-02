package service

import (
	"context"

	"github.com/runtime-radar/runtime-radar/lib/errcommon"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
	"github.com/runtime-radar/runtime-radar/notifier/api"
)

// AssistantAuth is a layer for jwt-based authentication.
// Base server interface should not be embedded here unlike
// in implementations of other layers.
// All required methods should be explicitly implemented to ensure
// that new methods of the basic server are implemented for auth layer.
type AssistantAuth struct {
	// UnsafeAssistantControllerServer is embedded to opt out of forward
	// compatibility promised by protobuf library.
	// It merely contains an empty `mustEmbedUnimplementedAssistantControllerServer()`
	// method.
	api.UnsafeAssistantControllerServer

	// AssistantControllerServer is a base server interface to pass
	// response to the next layer.
	AssistantControllerServer api.AssistantControllerServer
	Verifier                  jwt.Verifier
}

// Chat requires the same permission as the rest of the AI features: reading
// integrations. What the assistant may then look up is decided per tool by MCP
// Server, against this same caller's token.
func (aa *AssistantAuth) Chat(req *api.ChatReq, stream api.AssistantController_ChatServer) error {
	if err := aa.Verifier.VerifyPermission(stream.Context(), jwt.PermissionIntegrations, jwt.ActionRead); err != nil {
		return errcommon.PermissionErrorToStatus(err)
	}

	return aa.AssistantControllerServer.Chat(req, stream)
}

// The stored conversations are guarded by the same permission as the assistant
// itself, and then by ownership: the service only ever reads and deletes the
// caller's own, whatever their role.
func (aa *AssistantAuth) ListChats(ctx context.Context, req *api.ListChatsReq) (*api.ListChatsResp, error) {
	if err := aa.Verifier.VerifyPermission(ctx, jwt.PermissionIntegrations, jwt.ActionRead); err != nil {
		return nil, errcommon.PermissionErrorToStatus(err)
	}

	return aa.AssistantControllerServer.ListChats(ctx, req)
}

func (aa *AssistantAuth) ReadChat(ctx context.Context, req *api.ReadChatReq) (*api.ReadChatResp, error) {
	if err := aa.Verifier.VerifyPermission(ctx, jwt.PermissionIntegrations, jwt.ActionRead); err != nil {
		return nil, errcommon.PermissionErrorToStatus(err)
	}

	return aa.AssistantControllerServer.ReadChat(ctx, req)
}

func (aa *AssistantAuth) DeleteChat(ctx context.Context, req *api.DeleteChatReq) (*api.DeleteChatResp, error) {
	if err := aa.Verifier.VerifyPermission(ctx, jwt.PermissionIntegrations, jwt.ActionRead); err != nil {
		return nil, errcommon.PermissionErrorToStatus(err)
	}

	return aa.AssistantControllerServer.DeleteChat(ctx, req)
}
