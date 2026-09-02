package service

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/lib/server/interceptor"
	"github.com/runtime-radar/runtime-radar/notifier/api"
)

type AssistantLogging struct {
	api.AssistantControllerServer
}

// Chat logs the shape of the conversation, never its content: what a user asks
// the assistant is theirs, and the answer quotes untrusted telemetry.
func (al *AssistantLogging) Chat(req *api.ChatReq, stream api.AssistantController_ChatServer) (err error) {
	defer func(t0 time.Time) {
		corrID, _ := interceptor.CorrelationIDFromContext(stream.Context())

		log.Err(err).Str("delay", time.Since(t0).String()).
			Bool("audit", true).
			Str("integration_id", req.GetIntegrationId()).
			Str("event_id", req.GetEventId()).
			Int("messages", len(req.GetConversation())).
			Stringer("correlation_id", corrID).
			Msg("Called AssistantControllerServer.Chat")
	}(time.Now())

	err = al.AssistantControllerServer.Chat(req, stream)

	return
}

// The stored conversations are audited by their shape too: the identifier of
// the chat is logged, never a title or a message.
func (al *AssistantLogging) ListChats(ctx context.Context, req *api.ListChatsReq) (resp *api.ListChatsResp, err error) {
	defer func(t0 time.Time) {
		corrID, _ := interceptor.CorrelationIDFromContext(ctx)

		log.Err(err).Str("delay", time.Since(t0).String()).
			Bool("audit", true).
			Int("chats", len(resp.GetChats())).
			Stringer("correlation_id", corrID).
			Msg("Called AssistantControllerServer.ListChats")
	}(time.Now())

	resp, err = al.AssistantControllerServer.ListChats(ctx, req)

	return resp, err
}

func (al *AssistantLogging) ReadChat(ctx context.Context, req *api.ReadChatReq) (resp *api.ReadChatResp, err error) {
	defer func(t0 time.Time) {
		corrID, _ := interceptor.CorrelationIDFromContext(ctx)

		log.Err(err).Str("delay", time.Since(t0).String()).
			Bool("audit", true).
			Str("chat_id", req.GetId()).
			Stringer("correlation_id", corrID).
			Msg("Called AssistantControllerServer.ReadChat")
	}(time.Now())

	resp, err = al.AssistantControllerServer.ReadChat(ctx, req)

	return resp, err
}

func (al *AssistantLogging) DeleteChat(
	ctx context.Context, req *api.DeleteChatReq,
) (resp *api.DeleteChatResp, err error) {
	defer func(t0 time.Time) {
		corrID, _ := interceptor.CorrelationIDFromContext(ctx)

		log.Err(err).Str("delay", time.Since(t0).String()).
			Bool("audit", true).
			Str("chat_id", req.GetId()).
			Stringer("correlation_id", corrID).
			Msg("Called AssistantControllerServer.DeleteChat")
	}(time.Now())

	resp, err = al.AssistantControllerServer.DeleteChat(ctx, req)

	return resp, err
}
