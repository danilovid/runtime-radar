package service

import (
	"context"
	"maps"
	"slices"

	"github.com/gobwas/glob"
	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/policy-enforcer/api"
	"github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/cache"
	"github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/database"
	"github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/model"
	"github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/model/convert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type EnforcerGeneric struct {
	api.UnimplementedEnforcerServer

	RuleMatcher    cache.RuleMatcher
	RuleRepository database.RuleRepository
}

func (eg *EnforcerGeneric) EvaluatePolicyRuntimeEvent(ctx context.Context, req *api.EvaluatePolicyRuntimeEventReq) (*api.EvaluatePolicyRuntimeEventReq, error) {
	if reason, ok := eg.validateRuntimeEventRequest(req); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	// TODO: add cache.WithCluster when we support multiple clusters
	opts := []cache.MatchOption{
		cache.WithNamespace(req.GetAction().GetArgs().GetNamespace()),
		cache.WithImageName(req.GetAction().GetArgs().GetImageName()),
		cache.WithRegistry(req.GetAction().GetArgs().GetRegistry()),
		cache.WithPod(req.GetAction().GetArgs().GetPod()),
		cache.WithContainer(req.GetAction().GetArgs().GetContainer()),
		cache.WithNode(req.GetAction().GetArgs().GetNode()),
	}
	rules, err := eg.RuleMatcher.MatchRules(ctx, model.RuleTypeRuntime, opts...)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't match rules: %v", err)
	}

	rules = filterRulesByFunc(rules, func(r *model.Rule) bool {
		return isBinaryWhitelisted(req.GetAction().GetArgs().GetBinary(), r)
	})

	for _, event := range req.GetResult().GetEvents() {
		var eventSeverity model.Severity
		eventSeverity.Set(event.GetSeverity())

		rs := filterRulesByFunc(rules, func(r *model.Rule) bool {
			return isThreatWhitelisted(event.GetDetectorId(), r)
		})
		block, notify := filterRulesBySeverity(rs, eventSeverity)

		event.Policy = &api.Policy{
			BlockBy:  convert.RulesToProto(block),
			NotifyBy: convert.RulesToProto(notify),
		}
	}

	return req, nil
}

func (eg *EnforcerGeneric) validateRuntimeEventRequest(req *api.EvaluatePolicyRuntimeEventReq) (reason string, ok bool) {
	if a := req.GetAction(); a == nil {
		return reasonNoAction, false
	} else if a.GetArgs() == nil {
		return reasonNoArgs, false
	}

	return "", true
}

func filterRulesByFunc(rules []*model.Rule, filter func(*model.Rule) bool) []*model.Rule {
	cloned := slices.Clone(rules) // avoid modifying outer slice

	j := 0
	for _, r := range cloned {
		if !filter(r) {
			cloned[j] = r
			j++
		}
	}
	cloned = cloned[:j]

	return cloned
}

func filterRulesBySeverity(rs []*model.Rule, severity model.Severity) (block, notify []*model.Rule) {
	for _, r := range rs {
		blockSeverity, notifySeverity := model.UnsetSeverity, model.UnsetSeverity

		if r.Rule.Block != nil {
			blockSeverity.Set(r.Rule.Block.Severity)
		}
		if r.Rule.Notify != nil {
			notifySeverity.Set(r.Rule.Notify.Severity)
		}

		if severity >= blockSeverity {
			block = append(block, r)
		}

		if severity >= notifySeverity {
			notify = append(notify, r)
		}
	}

	return
}

func isThreatWhitelisted(threatID string, r *model.Rule) bool {
	for _, t := range r.Rule.Whitelist.GetThreats() {
		if threatID == t {
			return true
		}
	}
	return false
}

func isBinaryWhitelisted(bin string, r *model.Rule) bool {
	for _, b := range r.Rule.Whitelist.GetBinaries() {
		if g := glob.MustCompile(b); g.Match(bin) {
			return true
		}
	}
	return false
}

func (eg *EnforcerGeneric) EvaluatePolicyAdmission(ctx context.Context, req *api.EvaluatePolicyAdmissionReq) (*api.EvaluatePolicyAdmissionReq, error) {
	if reason, ok := eg.validateAdmissionRequest(req); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	rules, err := eg.matchAdmissionRules(ctx, req.GetAction().GetArgs())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't match rules: %v", err)
	}

	for _, event := range req.GetResult().GetEvents() {
		var eventSeverity model.Severity
		eventSeverity.Set(event.GetSeverity())

		rs := filterRulesByFunc(rules, func(r *model.Rule) bool {
			return isThreatWhitelisted(event.GetPolicyId(), r)
		})
		block, notify := filterRulesBySeverity(rs, eventSeverity)

		event.Policy = &api.Policy{
			BlockBy:  convert.RulesToProto(block),
			NotifyBy: convert.RulesToProto(notify),
		}
	}

	return req, nil
}

// matchAdmissionRules matches admission rules against every container of the resource and returns
// their union: an admission event is a single decision about the whole resource, so a rule scoped to
// an image applies to the event even when only one of the containers uses that image.
// A resource without containers, such as a role binding, is matched by its namespace, name and node only.
func (eg *EnforcerGeneric) matchAdmissionRules(ctx context.Context, args *api.EvaluatePolicyAdmissionReq_Action_Args) ([]*model.Rule, error) {
	// TODO: add cache.WithCluster when we support multiple clusters
	common := []cache.MatchOption{
		cache.WithNamespace(args.GetNamespace()),
		cache.WithPod(args.GetPod()),
		cache.WithNode(args.GetNode()),
	}

	optsPerContainer := [][]cache.MatchOption{common}

	for _, c := range args.GetContainers() {
		optsPerContainer = append(optsPerContainer, append(slices.Clone(common),
			cache.WithContainer(c.GetName()),
			cache.WithImageName(c.GetImageName()),
			cache.WithRegistry(c.GetRegistry()),
		))
	}

	matched := map[uuid.UUID]*model.Rule{}

	for _, opts := range optsPerContainer {
		rules, err := eg.RuleMatcher.MatchRules(ctx, model.RuleTypeAdmission, opts...)
		if err != nil {
			return nil, err
		}

		for _, r := range rules {
			matched[r.ID] = r
		}
	}

	return slices.Collect(maps.Values(matched)), nil
}

func (eg *EnforcerGeneric) validateAdmissionRequest(req *api.EvaluatePolicyAdmissionReq) (reason string, ok bool) {
	if a := req.GetAction(); a == nil {
		return reasonNoAction, false
	} else if a.GetArgs() == nil {
		return reasonNoArgs, false
	}

	return "", true
}
