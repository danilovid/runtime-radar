package service

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	monitor_api "github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/lib/server/interceptor"
)

type AdmissionHistoryLogging struct {
	api.AdmissionHistoryServer
}

func (al *AdmissionHistoryLogging) Read(ctx context.Context, req *api.ReadAdmissionEventReq) (resp *monitor_api.AdmissionEvent, err error) {
	defer func(t0 time.Time) {
		corrID, _ := interceptor.CorrelationIDFromContext(ctx)

		log.Err(err).Str("delay", time.Since(t0).String()).
			Interface("args", req).
			Interface("result", resp).
			Stringer("correlation_id", corrID).
			Msg("Called AdmissionHistoryServer.Read")
	}(time.Now())

	resp, err = al.AdmissionHistoryServer.Read(ctx, req)
	return
}

func (al *AdmissionHistoryLogging) ListAdmissionEventSlice(ctx context.Context, req *api.ListAdmissionEventSliceReq) (resp *api.ListAdmissionEventSliceResp, err error) {
	defer func(t0 time.Time) {
		corrID, _ := interceptor.CorrelationIDFromContext(ctx)

		log.Err(err).Str("delay", time.Since(t0).String()).
			Interface("args", req).
			Interface("result", resp).
			Stringer("correlation_id", corrID).
			Msg("Called AdmissionHistoryServer.ListAdmissionEventSlice")
	}(time.Now())

	resp, err = al.AdmissionHistoryServer.ListAdmissionEventSlice(ctx, req)
	return
}

func (al *AdmissionHistoryLogging) FilterAdmissionEventSlice(ctx context.Context, req *api.FilterAdmissionEventSliceReq) (resp *api.ListAdmissionEventSliceResp, err error) {
	defer func(t0 time.Time) {
		corrID, _ := interceptor.CorrelationIDFromContext(ctx)

		log.Err(err).Str("delay", time.Since(t0).String()).
			Interface("args", req).
			Interface("result", resp).
			Stringer("correlation_id", corrID).
			Msg("Called AdmissionHistoryServer.FilterAdmissionEventSlice")
	}(time.Now())

	resp, err = al.AdmissionHistoryServer.FilterAdmissionEventSlice(ctx, req)
	return
}
