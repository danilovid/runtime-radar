package processor

import (
	"context"

	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	notifier_api "github.com/runtime-radar/runtime-radar/notifier/api"
	enforcer_api "github.com/runtime-radar/runtime-radar/policy-enforcer/api"
	enforcer_model "github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/model"
)

func (p *Processor) notify(ctx context.Context, ev *api.AdmissionEvent, rules []*enforcer_api.Rule, block bool) error {
	msgs := []*notifier_api.Message{}

	sev := enforcer_model.NoneSeverity
	nts := make([]*notifier_api.AdmissionEvent_Event_Threat, 0, len(ev.GetThreats()))

	for _, t := range ev.GetThreats() {
		tSev := enforcer_model.NoneSeverity
		tSev.Set(t.GetSeverity())
		if tSev > sev {
			sev = tSev
		}

		nts = append(nts, &notifier_api.AdmissionEvent_Event_Threat{
			PolicyId:          t.GetPolicy().GetId(),
			PolicyName:        t.GetPolicy().GetName(),
			PolicyRule:        t.GetPolicy().GetRule(),
			PolicyDescription: t.GetPolicy().GetDescription(),
			Severity:          t.GetSeverity(),
		})
	}

	res := ev.GetResource()

	images := make([]string, 0, len(res.GetContainers()))
	for _, c := range res.GetContainers() {
		images = append(images, c.GetImageName())
	}

	innerEvent := &notifier_api.AdmissionEvent_Event{
		Threats:            nts,
		ResourceApiVersion: res.GetApiVersion(),
		ResourceKind:       res.GetKind(),
		ResourceNamespace:  res.GetNamespace(),
		ResourceName:       res.GetName(),
		NodeName:           res.GetNodeName(),
		Images:             images,
	}

	for _, r := range rules {
		for _, id := range r.GetRule().GetNotify().GetTargets() {
			ae := &notifier_api.AdmissionEvent{
				Event:        innerEvent,
				Severity:     sev.String(),
				RegisteredAt: ev.GetRegisteredAt(),
				Block:        block,
				RuleName:     r.GetName(),
				EventId:      ev.GetId(),
			}

			msgs = append(msgs, &notifier_api.Message{
				Event:          &notifier_api.Message_AdmissionEvent{AdmissionEvent: ae},
				NotificationId: id,
			})
		}
	}

	_, err := p.Notifier.Notify(ctx, &notifier_api.NotifyReq{Notifications: msgs})
	return err
}
