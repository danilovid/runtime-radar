package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	monitor_api "github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/database/clickhouse"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/model"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/model/convert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type AdmissionHistoryGeneric struct {
	api.UnimplementedAdmissionHistoryServer

	AdmissionEventRepository clickhouse.AdmissionEventRepository
}

func (ahg *AdmissionHistoryGeneric) Read(ctx context.Context, req *api.ReadAdmissionEventReq) (*monitor_api.AdmissionEvent, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse ID: %v", err)
	}

	e, err := ahg.AdmissionEventRepository.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "admission event not found")
		}
		return nil, status.Errorf(codes.Internal, "can't read admission event: %v", err)
	}

	return convert.AdmissionEventToProto(e), nil
}

func (ahg *AdmissionHistoryGeneric) ListAdmissionEventSlice(ctx context.Context, req *api.ListAdmissionEventSliceReq) (*api.ListAdmissionEventSliceResp, error) {
	if reason, ok := ahg.validateSlice(req.GetDirection(), req.GetCursor()); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	events, err := ahg.getSlice(ctx, req.GetDirection(), req.GetCursor(), nil, int(req.GetSliceSize()))
	if err != nil {
		return nil, fmt.Errorf("can't get events from db: %w", err)
	}

	return admissionSliceResp(events), nil
}

func (ahg *AdmissionHistoryGeneric) FilterAdmissionEventSlice(ctx context.Context, req *api.FilterAdmissionEventSliceReq) (*api.ListAdmissionEventSliceResp, error) {
	if reason, ok := ahg.validateSlice(req.GetDirection(), req.GetCursor()); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	if reason, ok := ahg.validateAdmissionEventFilter(req.GetFilter()); !ok {
		return nil, status.Errorf(codes.InvalidArgument, "filter is invalid: %s", reason)
	}

	filter, err := makeAdmissionEventFilter(req.GetFilter())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't make admission filter: %v", err)
	}

	events, err := ahg.getSlice(ctx, req.GetDirection(), req.GetCursor(), filter, int(req.GetSliceSize()))
	if err != nil {
		return nil, fmt.Errorf("can't get events from db: %w", err)
	}

	return admissionSliceResp(events), nil
}

func (ahg *AdmissionHistoryGeneric) getSlice(
	ctx context.Context,
	direction string,
	cursor *timestamppb.Timestamp,
	filter any,
	sliceSize int,
) ([]*model.AdmissionEvent, error) {
	if sliceSize == 0 {
		sliceSize = defaultSliceSize
	}

	if direction == directionLeft {
		return ahg.AdmissionEventRepository.GetLeftSlice(ctx, cursor.AsTime(), filter, sliceSize)
	}

	return ahg.AdmissionEventRepository.GetRightSlice(ctx, cursor.AsTime(), filter, sliceSize)
}

func admissionSliceResp(events []*model.AdmissionEvent) *api.ListAdmissionEventSliceResp {
	var leftCursor, rightCursor *timestamppb.Timestamp
	if len(events) != 0 {
		leftCursor = timestamppb.New(events[0].RegisteredAt)
		rightCursor = timestamppb.New(events[len(events)-1].RegisteredAt)
	}

	return &api.ListAdmissionEventSliceResp{
		LeftCursor:      leftCursor,
		RightCursor:     rightCursor,
		AdmissionEvents: convert.AdmissionEventsToProto(events),
	}
}

func (ahg *AdmissionHistoryGeneric) validateSlice(direction string, cursor *timestamppb.Timestamp) (string, bool) {
	if direction != directionLeft && direction != directionRight {
		return fmt.Sprintf("unsupported direction: %s", direction), false
	}

	if cursor == nil {
		return reasonMissingCursor, false
	}

	if err := cursor.CheckValid(); err != nil {
		return fmt.Sprintf("cursor is invalid: %s", err.Error()), false
	}

	return "", true
}

func (ahg *AdmissionHistoryGeneric) validateAdmissionEventFilter(af *api.AdmissionFilter) (reason string, ok bool) {
	if len(af.GetResourceKind()) == 0 &&
		len(af.GetResourceNamespace()) == 0 &&
		len(af.GetResourceName()) == 0 &&
		len(af.GetNodeName()) == 0 &&
		len(af.GetContainerNames()) == 0 &&
		len(af.GetImageNames()) == 0 &&
		len(af.GetThreatsPolicies()) == 0 &&
		len(af.GetRules()) == 0 &&
		af.GetPeriod().GetFrom() == nil &&
		af.GetPeriod().GetTo() == nil &&
		af.Blocked == nil &&
		af.HasIncident == nil {
		return reasonEmptyFilter, false
	}

	for i, r := range af.GetRules() {
		if _, err := uuid.Parse(r); err != nil {
			return fmt.Sprintf("can't parse rules[%d]: %v", i, err), false
		}
	}

	return "", true
}
