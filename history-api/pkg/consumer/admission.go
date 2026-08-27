package consumer

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/database/clickhouse"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/model"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/model/convert"
	"github.com/runtime-radar/runtime-radar/lib/rabbit"
)

type AdmissionConsumer struct {
	PublishConsumer          rabbit.PublishConsumer
	AdmissionEventRepository clickhouse.AdmissionEventRepository
}

func (c *AdmissionConsumer) Run(stop <-chan struct{}) {
	log.Info().Msgf("Admission events consumer started")
	defer log.Info().Msgf("Admission events consumer stopped")

	for {
		select {
		default:
			ev := &api.AdmissionEvent{}
			if err := c.PublishConsumer.Consume(context.Background(), ev); err != nil {
				log.Error().Msgf("Can't consume admission event: %v", err)
				continue
			}

			m, err := convert.AdmissionEventFromProto(ev)
			if err != nil {
				log.Error().Err(err).Msg("Can't convert admission event to model")
				continue
			}

			if err := c.AdmissionEventRepository.Add(context.Background(), &[]model.AdmissionEvent{m}); err != nil {
				log.Error().Err(err).Msgf("Can't save admission event")
			}

		case <-stop:
			return
		}
	}
}
