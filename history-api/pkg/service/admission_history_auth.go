package service

import (
	"context"

	monitor_api "github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/lib/errcommon"
	"github.com/runtime-radar/runtime-radar/lib/security/jwt"
)

// AdmissionHistoryAuth is a layer for jwt-based authentication.
// Base server interface should not be embedded here unlike
// in implementations of other layers.
// All required methods should be explicitly implemented to ensure
// that new methods of the basic server are implemented for auth layer.
type AdmissionHistoryAuth struct {
	// UnsafeAdmissionHistoryServer is embedded to opt out of forward
	// compatibility promised by protobuf library.
	api.UnsafeAdmissionHistoryServer

	// AdmissionHistoryServer is a base server interface to pass
	// response to the next layer.
	AdmissionHistoryServer api.AdmissionHistoryServer
	Verifier               jwt.Verifier
}

func (aha *AdmissionHistoryAuth) Read(ctx context.Context, req *api.ReadAdmissionEventReq) (resp *monitor_api.AdmissionEvent, err error) {
	if err := aha.Verifier.VerifyPermission(ctx, jwt.PermissionEvents, jwt.ActionRead); err != nil {
		return nil, errcommon.PermissionErrorToStatus(err)
	}
	resp, err = aha.AdmissionHistoryServer.Read(ctx, req)
	return
}

func (aha *AdmissionHistoryAuth) ListAdmissionEventSlice(ctx context.Context, req *api.ListAdmissionEventSliceReq) (resp *api.ListAdmissionEventSliceResp, err error) {
	if err := aha.Verifier.VerifyPermission(ctx, jwt.PermissionEvents, jwt.ActionRead); err != nil {
		return nil, errcommon.PermissionErrorToStatus(err)
	}
	resp, err = aha.AdmissionHistoryServer.ListAdmissionEventSlice(ctx, req)
	return
}

func (aha *AdmissionHistoryAuth) FilterAdmissionEventSlice(ctx context.Context, req *api.FilterAdmissionEventSliceReq) (resp *api.ListAdmissionEventSliceResp, err error) {
	if err := aha.Verifier.VerifyPermission(ctx, jwt.PermissionEvents, jwt.ActionRead); err != nil {
		return nil, errcommon.PermissionErrorToStatus(err)
	}
	resp, err = aha.AdmissionHistoryServer.FilterAdmissionEventSlice(ctx, req)
	return
}
