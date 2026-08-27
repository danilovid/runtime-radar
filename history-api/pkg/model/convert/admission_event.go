package convert

import (
	"fmt"

	"github.com/google/uuid"
	monitor_api "github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/model"
	enf_model "github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/model"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func AdmissionEventFromProto(proto *monitor_api.AdmissionEvent) (model.AdmissionEvent, error) {
	id, err := uuid.Parse(proto.GetId())
	if err != nil {
		return model.AdmissionEvent{}, fmt.Errorf("can't parse id: %w", err)
	}

	incidentSeverity := enf_model.NoneSeverity
	incidentSeverity.Set(proto.GetIncidentSeverity())

	res := proto.GetResource()

	event := model.AdmissionEvent{
		ID:             id,
		RegisteredAt:   proto.GetRegisteredAt().AsTime(),
		KyvernoVersion: proto.GetKyvernoVersion(),
		SourceResource: (*model.AdmissionResourceJSON)(res),

		ResourceAPIVersion: res.GetApiVersion(),
		ResourceKind:       res.GetKind(),
		ResourceNamespace:  res.GetNamespace(),
		ResourceName:       res.GetName(),
		ResourceUID:        res.GetUid(),
		NodeName:           res.GetNodeName(),

		Blocked:          proto.GetBlocked(),
		IsIncident:       proto.GetIsIncident(),
		IncidentSeverity: incidentSeverity,
		BlockBy:          proto.GetBlockBy(),
		NotifyBy:         proto.GetNotifyBy(),
	}

	for _, c := range res.GetContainers() {
		event.ContainerNames = append(event.ContainerNames, c.GetName())
		event.ImageNames = append(event.ImageNames, c.GetImageName())
		event.Registries = append(event.Registries, c.GetRegistry())
	}

	if ts := proto.GetThreats(); len(ts) > 0 {
		event.Threats = model.AdmissionEventThreats(ts)

		ps := make([]string, 0, len(ts))
		for _, t := range ts {
			ps = append(ps, t.GetPolicy().GetId())
		}
		event.ThreatsPolicies = ps
	}

	return event, nil
}

func AdmissionEventToProto(event *model.AdmissionEvent) *monitor_api.AdmissionEvent {
	return &monitor_api.AdmissionEvent{
		Id:               event.ID.String(),
		KyvernoVersion:   event.KyvernoVersion,
		RegisteredAt:     timestamppb.New(event.RegisteredAt),
		Resource:         (*monitor_api.Resource)(event.SourceResource),
		Threats:          event.Threats,
		Blocked:          event.Blocked,
		IsIncident:       event.IsIncident,
		IncidentSeverity: event.IncidentSeverity.String(),
		BlockBy:          event.BlockBy,
		NotifyBy:         event.NotifyBy,
	}
}

func AdmissionEventsToProto(events []*model.AdmissionEvent) []*monitor_api.AdmissionEvent {
	res := make([]*monitor_api.AdmissionEvent, 0, len(events))
	for _, event := range events {
		res = append(res, AdmissionEventToProto(event))
	}

	return res
}
