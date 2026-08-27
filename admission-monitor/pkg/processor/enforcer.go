package processor

import (
	"context"

	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/build"
	enforcer_api "github.com/runtime-radar/runtime-radar/policy-enforcer/api"
)

func (p *Processor) evaluatePolicy(ctx context.Context, ev *api.AdmissionEvent) (*enforcer_api.EvaluatePolicyAdmissionReq, error) {
	events := []*enforcer_api.EvaluatePolicyAdmissionReq_Result_Event{}
	for _, t := range ev.GetThreats() {
		events = append(events, &enforcer_api.EvaluatePolicyAdmissionReq_Result_Event{
			PolicyId: t.GetPolicy().GetId(),
			Severity: t.GetSeverity(),
		})
	}

	res := ev.GetResource()

	containers := []*enforcer_api.EvaluatePolicyAdmissionReq_Action_Args_Container{}
	for _, c := range res.GetContainers() {
		containers = append(containers, &enforcer_api.EvaluatePolicyAdmissionReq_Action_Args_Container{
			Name:      c.GetName(),
			ImageName: c.GetImageName(),
			Registry:  c.GetRegistry(),
		})
	}

	req := &enforcer_api.EvaluatePolicyAdmissionReq{
		Actor: build.AppName,
		Action: &enforcer_api.EvaluatePolicyAdmissionReq_Action{
			Type: actionType,
			Args: &enforcer_api.EvaluatePolicyAdmissionReq_Action_Args{
				Namespace: res.GetNamespace(),
				// A policy report is scoped to a resource which is not necessarily a pod, but pod is
				// the only name-like scope policy-enforcer rules have, so the resource name goes there.
				Pod:        res.GetName(),
				Node:       res.GetNodeName(),
				Containers: containers,
			},
		},
		Result: &enforcer_api.EvaluatePolicyAdmissionReq_Result{
			Events: events,
		},
	}

	return p.Enforcer.EvaluatePolicyAdmission(ctx, req)
}
