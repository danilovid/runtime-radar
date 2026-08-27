package convert

import (
	"fmt"

	"github.com/google/uuid"
	monitor_api "github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/model"
	enf_model "github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/model"
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
