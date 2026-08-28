package service

import (
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
