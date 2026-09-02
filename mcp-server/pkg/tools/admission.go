package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	monitor_api "github.com/runtime-radar/runtime-radar/admission-monitor/api"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// admissionActionAudit and admissionActionBlock are the two modes a source
	// can be in, spelled the way the UI spells them rather than the way the
	// protobuf enum does.
	admissionActionAudit = "audit"
	admissionActionBlock = "block"

	// admissionConfigVersion is the schema version admission-monitor accepts.
	// It describes the product's own format, so this server fills it in.
	admissionConfigVersion = "1"

	// createdSourceNote and changedSourceNote are returned with every change:
	// a source starts or stops working the moment the config is saved, and the
	// person who asked should be told what to look at rather than assume a draft.
	createdSourceNote = "The source is added but switched off. Enable it with set_admission_source once its " +
		"manifest has been reviewed, and start in the audit mode: a source in the block mode denies requests."
	changedSourceNote = "The change is in effect from now on. A source in the block mode makes Kyverno deny " +
		"matching requests, which stops deployments; open Admission → Sources in the UI to review the result."
)

// AdmissionSourceInfo is one Kyverno policy admission-monitor keeps applied.
type AdmissionSourceInfo struct {
	Key         string `json:"key" jsonschema:"stable identifier of the source. Pass it to set_admission_source"`
	Name        string `json:"name" jsonschema:"human readable name of the source"`
	Description string `json:"description,omitempty" jsonschema:"what the source detects"`
	Enabled     bool   `json:"enabled" jsonschema:"whether the policy is applied in the cluster right now"`
	Action      string `json:"action" jsonschema:"audit records the violation and lets the request through, block makes Kyverno deny it"`
	Severity    string `json:"severity" jsonschema:"severity assigned to every finding of this source: low, medium, high or critical"`
	Manifest    string `json:"manifest,omitempty" jsonschema:"the Kyverno policy manifest, returned only when include_manifests is set"`
}

// ListAdmissionSourcesArgs are the arguments of list_admission_sources.
type ListAdmissionSourcesArgs struct {
	IncludeManifests bool `json:"include_manifests,omitempty" jsonschema:"also return the Kyverno manifest of every source. They are long, so ask for them only when the manifest itself is the question"`
}

// ListAdmissionSourcesResult is the answer of list_admission_sources.
type ListAdmissionSourcesResult struct {
	Sources        []AdmissionSourceInfo `json:"sources" jsonschema:"the sources configured in the product"`
	EnabledCount   int                   `json:"enabled_count" jsonschema:"how many of them are applied in the cluster"`
	BlockingCount  int                   `json:"blocking_count" jsonschema:"how many of the enabled ones deny requests instead of only recording them"`
	HistoryControl string                `json:"history_control" jsonschema:"when admission events are stored: NONE, WITH_THREATS or ALL"`
}

// SetAdmissionSourceArgs are the arguments of set_admission_source. Every field
// but the key is optional: what is left out keeps its current value.
type SetAdmissionSourceArgs struct {
	Key      string `json:"key" jsonschema:"identifier of the source, as returned by list_admission_sources. Required"`
	Enabled  *bool  `json:"enabled,omitempty" jsonschema:"true applies the policy in the cluster, false removes it. Omit to keep the current state"`
	Action   string `json:"action,omitempty" jsonschema:"audit records the violation and lets the request through, block makes Kyverno deny it. Omit to keep the current mode"`
	Severity string `json:"severity,omitempty" jsonschema:"severity of the findings of this source: low, medium, high or critical. Omit to keep the current one"`
}

// SetAdmissionSourceResult is the answer of set_admission_source.
type SetAdmissionSourceResult struct {
	Source AdmissionSourceInfo `json:"source" jsonschema:"the source as it is now"`
	Note   string              `json:"note" jsonschema:"what the user should check now that the source changed"`
}

// CreateAdmissionSourceArgs are the arguments of create_admission_source.
type CreateAdmissionSourceArgs struct {
	Key         string `json:"key" jsonschema:"stable identifier of the new source, lowercase with dashes, for example require-run-as-non-root. Required"`
	Name        string `json:"name" jsonschema:"human readable name shown in the UI. Required"`
	Description string `json:"description" jsonschema:"what the source detects, shown next to its name. Required"`
	Manifest    string `json:"manifest" jsonschema:"the Kyverno policy manifest, a ValidatingPolicy or an ImageValidatingPolicy of policies.kyverno.io/v1. Its spec.validationActions is overwritten from the action of the source. Required"`
	Severity    string `json:"severity,omitempty" jsonschema:"severity of the findings of this source: low, medium, high or critical. Defaults to medium"`
}

// CreateAdmissionSourceResult is the answer of create_admission_source.
type CreateAdmissionSourceResult struct {
	Source AdmissionSourceInfo `json:"source" jsonschema:"the source that was added"`
	Note   string              `json:"note" jsonschema:"what the user should do now that the source exists"`
}

func registerAdmissionTools(server *mcp.Server, deps *Deps) {
	addScopedTool(server, deps, auth.ScopeAdmission, &mcp.Tool{
		Name:        "list_admission_sources",
		Annotations: readOnly("List admission sources"),
		Description: "List the admission sources: the Kyverno policies Runtime Radar keeps applied in the " +
			"cluster, whether each is switched on, whether it only records a violation or denies the request, " +
			"and the severity its findings get. Call it before changing anything, to learn the identifiers " +
			"set_admission_source takes and to see what is already covered.",
	}, []auth.Permission{auth.ReadSystemSettings()}, listAdmissionSources(deps))

	addScopedTool(server, deps, auth.ScopeAdmission, &mcp.Tool{
		Name:        "set_admission_source",
		Annotations: write("Enable, disable or reconfigure an admission source", false),
		Description: "Switch an admission source on or off, and change the mode it works in. Switching a " +
			"source to block makes Kyverno deny every request that violates it, which stops deployments and " +
			"can stop a rollout in progress; switching a source off removes the protection it provided. Name " +
			"the source and the effect to the user and get their agreement before calling this.",
	}, []auth.Permission{auth.WriteSystemSettings()}, setAdmissionSource(deps))

	addScopedTool(server, deps, auth.ScopeAdmission, &mcp.Tool{
		Name:        "create_admission_source",
		Annotations: write("Add an admission source", false),
		Description: "Add a Kyverno policy as a new admission source. The source is created switched off, so " +
			"adding one changes nothing in the cluster until it is enabled with set_admission_source. The " +
			"manifest is the user's, not something to invent: ask for it, or offer one and have them confirm " +
			"it reads as intended before calling this.",
	}, []auth.Permission{auth.WriteSystemSettings()}, createAdmissionSource(deps))

	addScopedTool(server, deps, auth.ScopeAdmission, &mcp.Tool{
		Name:        "search_admission_events",
		Annotations: readOnly("Search admission events"),
		Description: "Find what Kyverno reported when resources were admitted to the cluster: which policy " +
			"fired, on which resource, and whether the request was denied or only recorded. Use it to see " +
			"what a source would block before switching it to the block mode.",
	}, []auth.Permission{auth.ReadEvents()}, searchAdmissionEvents(deps))

	addScopedTool(server, deps, auth.ScopeAdmission, &mcp.Tool{
		Name:        "get_admission_event",
		Annotations: readOnly("Explain one admission event"),
		Description: "Return one admission event in full and in plain terms: the resource and its containers, " +
			"every policy that fired with the reason it reported, whether Kyverno denied the request, and " +
			"which response rules turned the finding into an incident. This is the tool to reach for when a " +
			"user asks what an admission event means.",
	}, []auth.Permission{auth.ReadEvents()}, getAdmissionEvent(deps))
}

func listAdmissionSources(deps *Deps) func(context.Context, ListAdmissionSourcesArgs) (ListAdmissionSourcesResult, error) {
	return func(ctx context.Context, args ListAdmissionSourcesArgs) (ListAdmissionSourcesResult, error) {
		config, err := deps.Clients.AdmissionConfig.Read(ctx, &emptypb.Empty{})
		if err != nil {
			return ListAdmissionSourcesResult{}, fmt.Errorf("can't read the admission configuration: %w", err)
		}

		policies := config.GetConfig().GetPolicies()

		result := ListAdmissionSourcesResult{
			Sources:        make([]AdmissionSourceInfo, 0, len(policies)),
			HistoryControl: config.GetConfig().GetHistoryControl().String(),
		}

		for key, policy := range policies {
			info := sourceInfo(key, policy)

			if !args.IncludeManifests {
				info.Manifest = ""
			}

			if info.Enabled {
				result.EnabledCount++

				if info.Action == admissionActionBlock {
					result.BlockingCount++
				}
			}

			result.Sources = append(result.Sources, info)
		}

		// The config is a map, so an order has to be imposed for the answer to
		// be stable between calls.
		sort.Slice(result.Sources, func(i, j int) bool { return result.Sources[i].Key < result.Sources[j].Key })

		return result, nil
	}
}

func setAdmissionSource(deps *Deps) func(context.Context, SetAdmissionSourceArgs) (SetAdmissionSourceResult, error) {
	return func(ctx context.Context, args SetAdmissionSourceArgs) (SetAdmissionSourceResult, error) {
		if strings.TrimSpace(args.Key) == "" {
			return SetAdmissionSourceResult{}, status.Error(codes.InvalidArgument, "key is empty")
		}

		if args.Enabled == nil && args.Action == "" && args.Severity == "" {
			return SetAdmissionSourceResult{}, status.Error(codes.InvalidArgument,
				"nothing to change: pass enabled, action or severity")
		}

		config, err := deps.Clients.AdmissionConfig.Read(ctx, &emptypb.Empty{})
		if err != nil {
			return SetAdmissionSourceResult{}, fmt.Errorf("can't read the admission configuration: %w", err)
		}

		policy, ok := config.GetConfig().GetPolicies()[args.Key]
		if !ok {
			return SetAdmissionSourceResult{}, status.Errorf(codes.NotFound,
				"there is no admission source %q, call list_admission_sources for the ones there are", args.Key)
		}

		if args.Enabled != nil {
			policy.Enabled = *args.Enabled
		}

		if args.Action != "" {
			action, err := admissionAction(args.Action)
			if err != nil {
				return SetAdmissionSourceResult{}, err
			}

			policy.Action = action
		}

		if args.Severity != "" {
			severity, err := admissionSeverity(args.Severity)
			if err != nil {
				return SetAdmissionSourceResult{}, err
			}

			policy.Severity = severity
		}

		// admission-monitor takes the whole configuration, so the answer of the
		// read above is sent back with one source changed.
		config.Config.Version = admissionConfigVersion

		if _, err := deps.Clients.AdmissionConfig.Add(ctx, config); err != nil {
			return SetAdmissionSourceResult{}, fmt.Errorf("can't save the admission configuration: %w", err)
		}

		info := sourceInfo(args.Key, policy)
		info.Manifest = ""

		return SetAdmissionSourceResult{Source: info, Note: changedSourceNote}, nil
	}
}

func createAdmissionSource(deps *Deps) func(context.Context, CreateAdmissionSourceArgs) (CreateAdmissionSourceResult, error) {
	return func(ctx context.Context, args CreateAdmissionSourceArgs) (CreateAdmissionSourceResult, error) {
		key := strings.TrimSpace(args.Key)

		switch {
		case key == "":
			return CreateAdmissionSourceResult{}, status.Error(codes.InvalidArgument, "key is empty")
		case strings.TrimSpace(args.Name) == "":
			return CreateAdmissionSourceResult{}, status.Error(codes.InvalidArgument, "name is empty")
		case strings.TrimSpace(args.Description) == "":
			return CreateAdmissionSourceResult{}, status.Error(codes.InvalidArgument, "description is empty")
		case strings.TrimSpace(args.Manifest) == "":
			return CreateAdmissionSourceResult{}, status.Error(codes.InvalidArgument, "manifest is empty")
		}

		severity := ruleSeverityMedium
		if args.Severity != "" {
			parsed, err := admissionSeverity(args.Severity)
			if err != nil {
				return CreateAdmissionSourceResult{}, err
			}

			severity = parsed
		}

		config, err := deps.Clients.AdmissionConfig.Read(ctx, &emptypb.Empty{})
		if err != nil {
			return CreateAdmissionSourceResult{}, fmt.Errorf("can't read the admission configuration: %w", err)
		}

		if _, taken := config.GetConfig().GetPolicies()[key]; taken {
			return CreateAdmissionSourceResult{}, status.Errorf(codes.AlreadyExists,
				"an admission source %q already exists, change it with set_admission_source instead", key)
		}

		policy := &monitor_api.KyvernoPolicy{
			Name:        strings.TrimSpace(args.Name),
			Description: strings.TrimSpace(args.Description),
			Yaml:        args.Manifest,
			// A new source never starts working on its own: a manifest a model
			// produced is reviewed by a human before it can deny anything.
			Enabled:  false,
			Action:   monitor_api.KyvernoPolicy_AUDIT,
			Severity: severity,
		}

		if config.Config == nil {
			config.Config = &monitor_api.Config_ConfigJSON{}
		}
		if config.Config.Policies == nil {
			config.Config.Policies = map[string]*monitor_api.KyvernoPolicy{}
		}

		config.Config.Policies[key] = policy
		config.Config.Version = admissionConfigVersion

		if _, err := deps.Clients.AdmissionConfig.Add(ctx, config); err != nil {
			return CreateAdmissionSourceResult{}, fmt.Errorf("can't save the admission configuration: %w", err)
		}

		info := sourceInfo(key, policy)
		info.Manifest = ""

		return CreateAdmissionSourceResult{Source: info, Note: createdSourceNote}, nil
	}
}

func sourceInfo(key string, policy *monitor_api.KyvernoPolicy) AdmissionSourceInfo {
	action := admissionActionAudit
	if policy.GetAction() == monitor_api.KyvernoPolicy_ENFORCE {
		action = admissionActionBlock
	}

	return AdmissionSourceInfo{
		Key:         key,
		Name:        policy.GetName(),
		Description: policy.GetDescription(),
		Enabled:     policy.GetEnabled(),
		Action:      action,
		Severity:    policy.GetSeverity(),
		Manifest:    policy.GetYaml(),
	}
}

func admissionAction(value string) (monitor_api.KyvernoPolicy_Action, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case admissionActionAudit:
		return monitor_api.KyvernoPolicy_AUDIT, nil
	case admissionActionBlock:
		return monitor_api.KyvernoPolicy_ENFORCE, nil
	default:
		return 0, status.Errorf(codes.InvalidArgument, "%q is not an action, expected %s or %s",
			value, admissionActionAudit, admissionActionBlock)
	}
}

func admissionSeverity(value string) (string, error) {
	severity := strings.ToLower(strings.TrimSpace(value))

	switch severity {
	case ruleSeverityLow, ruleSeverityMedium, ruleSeverityHigh, ruleSeverityCritical:
		return severity, nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "%q is not a severity, expected %s, %s, %s or %s",
			value, ruleSeverityLow, ruleSeverityMedium, ruleSeverityHigh, ruleSeverityCritical)
	}
}

// AdmissionThreatInfo is one policy that fired on a resource.
type AdmissionThreatInfo struct {
	PolicyID   string `json:"policy_id" jsonschema:"identifier of the finding as <policy name>/<rule name>. A response rule whitelists it by this string"`
	PolicyName string `json:"policy_name" jsonschema:"name of the source the policy belongs to"`
	Rule       string `json:"rule,omitempty" jsonschema:"name of the rule inside the policy that failed"`
	Severity   string `json:"severity" jsonschema:"severity assigned to the source: low, medium, high or critical"`
	Reason     string `json:"reason,omitempty" jsonschema:"what the policy reported, in Kyverno's own words"`
	Category   string `json:"category,omitempty" jsonschema:"category the policy belongs to, when it declares one"`
}

// AdmissionContainerInfo is one container of the resource a policy fired on.
type AdmissionContainerInfo struct {
	Name     string `json:"name" jsonschema:"name of the container"`
	Image    string `json:"image" jsonschema:"image the container runs"`
	Registry string `json:"registry,omitempty" jsonschema:"registry the image comes from. Empty for Docker Hub"`
}

// AdmissionEventSummary is one admission event as a search returns it.
type AdmissionEventSummary struct {
	ID               string   `json:"id" jsonschema:"identifier of the event. Pass it to get_admission_event"`
	RegisteredAt     string   `json:"registered_at" jsonschema:"when the event was recorded, RFC3339"`
	ResourceKind     string   `json:"resource_kind" jsonschema:"kind of the resource the policy fired on, for example Pod or Deployment"`
	ResourceName     string   `json:"resource_name" jsonschema:"name of that resource"`
	Namespace        string   `json:"namespace,omitempty" jsonschema:"namespace of that resource. Empty for a cluster-scoped one"`
	Blocked          bool     `json:"blocked" jsonschema:"true when Kyverno denied the request, false when it only recorded the violation"`
	IsIncident       bool     `json:"is_incident" jsonschema:"true when a response rule matched the finding"`
	IncidentSeverity string   `json:"incident_severity,omitempty" jsonschema:"severity of the incident, when there is one"`
	Policies         []string `json:"policies" jsonschema:"identifiers of the policies that fired, as <policy name>/<rule name>"`
}

// AdmissionEventDetail is one admission event in full.
type AdmissionEventDetail struct {
	ID               string                   `json:"id" jsonschema:"identifier of the event"`
	RegisteredAt     string                   `json:"registered_at" jsonschema:"when the event was recorded, RFC3339"`
	KyvernoVersion   string                   `json:"kyverno_version,omitempty" jsonschema:"version of Kyverno that produced the finding"`
	ResourceKind     string                   `json:"resource_kind" jsonschema:"kind of the resource the policy fired on"`
	ResourceName     string                   `json:"resource_name" jsonschema:"name of that resource"`
	APIVersion       string                   `json:"api_version,omitempty" jsonschema:"API version of that resource"`
	Namespace        string                   `json:"namespace,omitempty" jsonschema:"namespace of that resource"`
	NodeName         string                   `json:"node_name,omitempty" jsonschema:"node the resource is scheduled on, when it is known"`
	Containers       []AdmissionContainerInfo `json:"containers,omitempty" jsonschema:"containers of the resource. Empty for a blocked request: the resource was never created, so there was nothing to read them from"`
	Threats          []AdmissionThreatInfo    `json:"threats" jsonschema:"the policies that fired, with the reason each reported"`
	Blocked          bool                     `json:"blocked" jsonschema:"true when Kyverno denied the request"`
	IsIncident       bool                     `json:"is_incident" jsonschema:"true when a response rule matched the finding"`
	IncidentSeverity string                   `json:"incident_severity,omitempty" jsonschema:"severity of the incident, when there is one"`
	BlockBy          []string                 `json:"block_by,omitempty" jsonschema:"identifiers of the response rules that would block on this finding"`
	NotifyBy         []string                 `json:"notify_by,omitempty" jsonschema:"identifiers of the response rules that notified about this finding"`
}

// SearchAdmissionEventsArgs are the arguments of search_admission_events.
type SearchAdmissionEventsArgs struct {
	Namespace    []string `json:"namespace,omitempty" jsonschema:"namespaces to search in. Supports globs, for example prod-*"`
	ResourceKind []string `json:"resource_kind,omitempty" jsonschema:"kinds of resource to search for, for example Pod or Deployment. Exact match, no globs"`
	ResourceName []string `json:"resource_name,omitempty" jsonschema:"names of the resource. Supports globs, for example nginx-*"`
	ImageName    []string `json:"image_name,omitempty" jsonschema:"container images of the resource. Supports globs. A blocked request has no containers and will not match"`
	PolicyIDs    []string `json:"policy_ids,omitempty" jsonschema:"identifiers of the policies that fired, as <policy name>/<rule name>. Exact match, no globs"`
	Blocked      *bool    `json:"blocked,omitempty" jsonschema:"true returns only requests Kyverno denied, false only recorded violations. Omit to return both"`
	HasIncident  *bool    `json:"has_incident,omitempty" jsonschema:"true returns only findings a response rule matched. Omit to return all"`
	Since        string   `json:"since,omitempty" jsonschema:"start of the time window, RFC3339. Defaults to 24 hours before now"`
	Until        string   `json:"until,omitempty" jsonschema:"end of the time window, RFC3339. Defaults to now"`
	Limit        int      `json:"limit,omitempty" jsonschema:"how many events to return, at most 50. Defaults to 20"`
}

// SearchAdmissionEventsResult is the answer of search_admission_events.
type SearchAdmissionEventsResult struct {
	Events []AdmissionEventSummary `json:"events" jsonschema:"the events found, newest first"`
	Count  int                     `json:"count" jsonschema:"number of events returned"`
	Window SearchWindow            `json:"window" jsonschema:"the time window and limit the search actually used"`
}

// GetAdmissionEventArgs are the arguments of get_admission_event.
type GetAdmissionEventArgs struct {
	ID string `json:"id" jsonschema:"identifier of the event, as returned by search_admission_events. Required"`
}

func searchAdmissionEvents(deps *Deps) func(context.Context, SearchAdmissionEventsArgs) (SearchAdmissionEventsResult, error) {
	return func(ctx context.Context, args SearchAdmissionEventsArgs) (SearchAdmissionEventsResult, error) {
		req, window, err := args.request(deps.now())
		if err != nil {
			return SearchAdmissionEventsResult{}, err
		}

		resp, err := deps.Clients.AdmissionHistory.FilterAdmissionEventSlice(ctx, req)
		if err != nil {
			return SearchAdmissionEventsResult{}, fmt.Errorf("can't search admission events: %w", err)
		}

		events := resp.GetAdmissionEvents()
		result := SearchAdmissionEventsResult{
			Events: make([]AdmissionEventSummary, 0, len(events)),
			Window: window,
		}

		for _, event := range events {
			result.Events = append(result.Events, admissionSummary(event))
		}

		result.Count = len(result.Events)

		return result, nil
	}
}

func (a SearchAdmissionEventsArgs) request(now time.Time) (*history_api.FilterAdmissionEventSliceReq, SearchWindow, error) {
	limit := a.Limit

	switch {
	case limit <= 0:
		limit = defaultSearchLimit
	case limit > maxSearchLimit:
		limit = maxSearchLimit
	}

	until, err := parseTime(a.Until, now)
	if err != nil {
		return nil, SearchWindow{}, status.Errorf(codes.InvalidArgument, "can't parse until: %v", err)
	}

	since, err := parseTime(a.Since, until.Add(-defaultLookback))
	if err != nil {
		return nil, SearchWindow{}, status.Errorf(codes.InvalidArgument, "can't parse since: %v", err)
	}

	if !since.Before(until) {
		return nil, SearchWindow{}, status.Error(codes.InvalidArgument, "since must be earlier than until")
	}

	req := &history_api.FilterAdmissionEventSliceReq{
		// The cursor is exclusive and the direction makes History API walk
		// backwards from it, so the newest events of the window come first.
		Cursor:    timestamppb.New(until),
		Direction: directionRight,
		SliceSize: uint32(limit), // #nosec G115 -- limit is bounded by maxSearchLimit above
		Filter: &history_api.AdmissionFilter{
			ResourceNamespace: a.Namespace,
			ResourceKind:      a.ResourceKind,
			ResourceName:      a.ResourceName,
			ImageNames:        a.ImageName,
			ThreatsPolicies:   a.PolicyIDs,
			Blocked:           a.Blocked,
			HasIncident:       a.HasIncident,
			// The period is always set: it bounds the scan, and History API
			// rejects a filter with no criterion at all.
			Period: &history_api.Period{From: timestamppb.New(since), To: timestamppb.New(until)},
		},
	}

	window := SearchWindow{
		Since: since.Format(time.RFC3339),
		Until: until.Format(time.RFC3339),
		Limit: limit,
	}

	return req, window, nil
}

func getAdmissionEvent(deps *Deps) func(context.Context, GetAdmissionEventArgs) (AdmissionEventDetail, error) {
	return func(ctx context.Context, args GetAdmissionEventArgs) (AdmissionEventDetail, error) {
		if strings.TrimSpace(args.ID) == "" {
			return AdmissionEventDetail{}, status.Error(codes.InvalidArgument, "id is empty")
		}

		event, err := deps.Clients.AdmissionHistory.Read(ctx, &history_api.ReadAdmissionEventReq{Id: args.ID})
		if err != nil {
			return AdmissionEventDetail{}, fmt.Errorf("can't read admission event: %w", err)
		}

		return admissionDetail(event), nil
	}
}

func admissionSummary(event *monitor_api.AdmissionEvent) AdmissionEventSummary {
	resource := event.GetResource()

	summary := AdmissionEventSummary{
		ID:               event.GetId(),
		RegisteredAt:     event.GetRegisteredAt().AsTime().Format(time.RFC3339),
		ResourceKind:     resource.GetKind(),
		ResourceName:     resource.GetName(),
		Namespace:        resource.GetNamespace(),
		Blocked:          event.GetBlocked(),
		IsIncident:       event.GetIsIncident(),
		IncidentSeverity: event.GetIncidentSeverity(),
		Policies:         make([]string, 0, len(event.GetThreats())),
	}

	for _, threat := range event.GetThreats() {
		summary.Policies = append(summary.Policies, threat.GetPolicy().GetId())
	}

	return summary
}

func admissionDetail(event *monitor_api.AdmissionEvent) AdmissionEventDetail {
	resource := event.GetResource()

	detail := AdmissionEventDetail{
		ID:               event.GetId(),
		RegisteredAt:     event.GetRegisteredAt().AsTime().Format(time.RFC3339),
		KyvernoVersion:   event.GetKyvernoVersion(),
		ResourceKind:     resource.GetKind(),
		ResourceName:     resource.GetName(),
		APIVersion:       resource.GetApiVersion(),
		Namespace:        resource.GetNamespace(),
		NodeName:         resource.GetNodeName(),
		Containers:       make([]AdmissionContainerInfo, 0, len(resource.GetContainers())),
		Threats:          make([]AdmissionThreatInfo, 0, len(event.GetThreats())),
		Blocked:          event.GetBlocked(),
		IsIncident:       event.GetIsIncident(),
		IncidentSeverity: event.GetIncidentSeverity(),
		BlockBy:          event.GetBlockBy(),
		NotifyBy:         event.GetNotifyBy(),
	}

	for _, container := range resource.GetContainers() {
		detail.Containers = append(detail.Containers, AdmissionContainerInfo{
			Name:     container.GetName(),
			Image:    container.GetImageName(),
			Registry: container.GetRegistry(),
		})
	}

	for _, threat := range event.GetThreats() {
		policy := threat.GetPolicy()

		detail.Threats = append(detail.Threats, AdmissionThreatInfo{
			PolicyID:   policy.GetId(),
			PolicyName: policy.GetName(),
			Rule:       policy.GetRule(),
			Severity:   threat.GetSeverity(),
			Reason:     policy.GetDescription(),
			Category:   policy.GetCategory(),
		})
	}

	return detail
}
