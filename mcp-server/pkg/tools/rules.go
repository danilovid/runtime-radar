package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	enf_api "github.com/runtime-radar/runtime-radar/policy-enforcer/api"
)

const (
	// rulePageSize is how many rules are read per Policy Enforcer page, and
	// maxRulePages bounds the paging loop.
	rulePageSize = 100
	maxRulePages = 20
	// ruleOrder lists rules by name, the way the UI shows them.
	ruleOrder = "name asc"

	// ruleVersion and scopeVersion are the schema versions Policy Enforcer
	// accepts. They are filled in by this server rather than asked of the
	// model: they describe the product's own format, not the user's intent.
	ruleVersion  = "1"
	scopeVersion = "1"

	// severities a rule may react to, in the spelling Policy Enforcer expects.
	ruleSeverityNone     = "none"
	ruleSeverityLow      = "low"
	ruleSeverityMedium   = "medium"
	ruleSeverityHigh     = "high"
	ruleSeverityCritical = "critical"

	severityValues = ruleSeverityNone + ", " + ruleSeverityLow + ", " + ruleSeverityMedium + ", " + ruleSeverityHigh +
		" or " + ruleSeverityCritical
)

// RuleScope is the set of workloads a rule applies to. An empty scope means
// every workload of every cluster.
type RuleScope struct {
	Clusters   []string `json:"clusters,omitempty" jsonschema:"cluster names the rule applies to. Empty means every cluster"`
	Namespaces []string `json:"namespaces,omitempty" jsonschema:"Kubernetes namespaces the rule applies to. Empty means every namespace"`
	Pods       []string `json:"pods,omitempty" jsonschema:"pod names the rule applies to. Empty means every pod"`
	Containers []string `json:"containers,omitempty" jsonschema:"container names the rule applies to. Empty means every container"`
	Nodes      []string `json:"nodes,omitempty" jsonschema:"node names the rule applies to. Empty means every node"`
	ImageNames []string `json:"image_names,omitempty" jsonschema:"container image names the rule applies to. Empty means every image"`
	Registries []string `json:"registries,omitempty" jsonschema:"image registries the rule applies to. Empty means every registry"`
}

// RuleInfo is one policy rule as the tools report it.
type RuleInfo struct {
	ID                string    `json:"id" jsonschema:"identifier of the rule. Pass it to delete_rule"`
	Name              string    `json:"name" jsonschema:"human readable name of the rule"`
	BlockSeverity     string    `json:"block_severity,omitempty" jsonschema:"the rule kills the pod when a threat of at least this severity is detected. Empty when the rule does not block"`
	NotifySeverity    string    `json:"notify_severity,omitempty" jsonschema:"the rule sends a notification when a threat of at least this severity is detected. Empty when the rule does not notify"`
	NotifyTargets     []string  `json:"notify_targets,omitempty" jsonschema:"identifiers of the notification integrations the rule notifies"`
	WhitelistThreats  []string  `json:"whitelist_threats,omitempty" jsonschema:"detector identifiers the rule ignores"`
	WhitelistBinaries []string  `json:"whitelist_binaries,omitempty" jsonschema:"binary path patterns the rule ignores"`
	Scope             RuleScope `json:"scope" jsonschema:"the workloads the rule applies to"`
}

// ListRulesArgs are the arguments of list_rules.
type ListRulesArgs struct {
	Search string `json:"search,omitempty" jsonschema:"case-insensitive substring to filter rules by name"`
}

// ListRulesResult is the answer of list_rules.
type ListRulesResult struct {
	Rules []RuleInfo `json:"rules" jsonschema:"policy rules configured in the product, by name"`
	Count int        `json:"count" jsonschema:"number of rules returned"`
	Total int        `json:"total" jsonschema:"number of rules configured, before the search filter was applied"`
}

// CreateRuleArgs are the arguments of create_rule. A rule has to react somehow,
// so at least one of block_severity and notify_severity is required.
type CreateRuleArgs struct {
	Name              string    `json:"name" jsonschema:"name of the rule, shown in the UI. Required"`
	BlockSeverity     string    `json:"block_severity,omitempty" jsonschema:"kill the pod when a threat of at least this severity is detected: none, low, medium, high or critical. Leave empty for a rule that only notifies"`
	NotifySeverity    string    `json:"notify_severity,omitempty" jsonschema:"notify when a threat of at least this severity is detected: none, low, medium, high or critical. Leave empty for a rule that only blocks"`
	NotifyTargets     []string  `json:"notify_targets,omitempty" jsonschema:"identifiers of the notification integrations to notify. Ask the user for them or read them from list_rules; they cannot be guessed from a name"`
	WhitelistThreats  []string  `json:"whitelist_threats,omitempty" jsonschema:"detector identifiers the rule ignores, as returned by list_detectors"`
	WhitelistBinaries []string  `json:"whitelist_binaries,omitempty" jsonschema:"binary path patterns the rule ignores, for example /usr/bin/apt-get"`
	Scope             RuleScope `json:"scope,omitzero" jsonschema:"the workloads the rule applies to. Leave every field empty to apply it everywhere, and say so to the user before creating it"`
}

// CreateRuleResult is the answer of create_rule.
type CreateRuleResult struct {
	ID   string `json:"id" jsonschema:"identifier of the rule that was created"`
	Name string `json:"name" jsonschema:"name of the rule that was created"`
	Note string `json:"note" jsonschema:"what the user should check now that the rule exists"`
}

// SetRuleNotifyTargetsArgs are the arguments of set_rule_notify_targets. Only
// the delivery is changed: what a rule matches and how loudly it reacts are not
// things to adjust while doing something else.
type SetRuleNotifyTargetsArgs struct {
	ID            string   `json:"id" jsonschema:"identifier of the rule, as returned by list_rules. Required"`
	NotifyTargets []string `json:"notify_targets" jsonschema:"identifiers of the notification services the rule sends through, as returned by list_notification_services. An empty list makes the rule fire silently. Required"`
}

// SetRuleNotifyTargetsResult is the answer of set_rule_notify_targets.
type SetRuleNotifyTargetsResult struct {
	ID            string   `json:"id" jsonschema:"identifier of the rule that was changed"`
	Name          string   `json:"name" jsonschema:"name of the rule that was changed"`
	NotifyTargets []string `json:"notify_targets" jsonschema:"the notification services the rule now sends through"`
	Note          string   `json:"note" jsonschema:"what the user should check now"`
}

// DeleteRuleArgs are the arguments of delete_rule.
type DeleteRuleArgs struct {
	ID string `json:"id" jsonschema:"identifier of the rule to delete, as returned by list_rules. Required"`
}

// DeleteRuleResult is the answer of delete_rule.
type DeleteRuleResult struct {
	ID      string `json:"id" jsonschema:"identifier of the rule that was deleted"`
	Deleted bool   `json:"deleted" jsonschema:"always true: a rule that could not be deleted is reported as an error instead"`
}

// createdRuleNote is returned with every new rule. A rule starts working the
// moment it is saved, so the person who asked for it should be told what to
// look at rather than left assuming it was a draft.
const createdRuleNote = "The rule is active from now on. Open Management → Rules in the UI to review its scope, " +
	"and delete it with delete_rule if it turns out to match more workloads than intended."

func registerRuleTools(server *mcp.Server, deps *Deps) {
	addTool(server, deps, &mcp.Tool{
		Name:        "list_rules",
		Annotations: readOnly("List policy rules"),
		Description: "List the policy rules of the product: what each one blocks or notifies about, which " +
			"notification targets it uses, what it whitelists and which workloads it applies to. Use it to see " +
			"whether a rule for some workload already exists before proposing another one, and to learn the " +
			"identifiers create_rule and delete_rule take.",
	}, []auth.Permission{auth.ReadRules()}, listRules(deps))

	addTool(server, deps, &mcp.Tool{
		Name:        "create_rule",
		Annotations: write("Create a policy rule", false),
		Description: "Create a policy rule. A rule reacts to the threats detectors report: block_severity kills " +
			"the pod a threat was detected in, notify_severity sends a notification, and at least one of the two " +
			"is required. A blocking rule stops workloads, so state its scope and severity to the user and get " +
			"their agreement before calling this.",
	}, []auth.Permission{auth.CreateRules()}, createRule(deps))

	addTool(server, deps, &mcp.Tool{
		Name:        "set_rule_notify_targets",
		Annotations: write("Choose where a rule sends its notifications", false),
		Description: "Point an existing rule at the notification services it should send through, or at none. A " +
			"rule that notifies nobody fires silently, which is the usual reason a rule looks configured and " +
			"nothing ever arrives. This changes delivery only: it does not touch what the rule matches, its " +
			"severities or its scope.",
	}, []auth.Permission{auth.UpdateRules()}, setRuleNotifyTargets(deps))

	addTool(server, deps, &mcp.Tool{
		Name:        "delete_rule",
		Annotations: write("Delete a policy rule", true),
		Description: "Delete a policy rule by its identifier. The workloads it covered stop being blocked and " +
			"stop producing notifications, which is a reduction of protection: name the rule to the user and get " +
			"their agreement before calling this.",
	}, []auth.Permission{auth.DeleteRules()}, deleteRule(deps))
}

func listRules(deps *Deps) func(context.Context, ListRulesArgs) (ListRulesResult, error) {
	return func(ctx context.Context, args ListRulesArgs) (ListRulesResult, error) {
		var (
			rules []*enf_api.Rule
			total int
		)

		for page := uint32(1); page <= maxRulePages; page++ {
			resp, err := deps.Clients.Rules.ListPage(ctx, &enf_api.ListRulePageReq{
				PageNum:  page,
				PageSize: rulePageSize,
				Order:    ruleOrder,
			})
			if err != nil {
				return ListRulesResult{}, fmt.Errorf("can't list rules: %w", err)
			}

			total = int(resp.GetTotal())
			rules = append(rules, resp.GetRules()...)

			if len(resp.GetRules()) < rulePageSize || len(rules) >= total {
				break
			}
		}

		search := strings.ToLower(strings.TrimSpace(args.Search))

		result := ListRulesResult{Rules: make([]RuleInfo, 0, len(rules)), Total: total}

		for _, rule := range rules {
			info := ruleInfo(rule)

			if search != "" && !strings.Contains(strings.ToLower(info.Name), search) {
				continue
			}

			result.Rules = append(result.Rules, info)
		}

		result.Count = len(result.Rules)

		return result, nil
	}
}

func createRule(deps *Deps) func(context.Context, CreateRuleArgs) (CreateRuleResult, error) {
	return func(ctx context.Context, args CreateRuleArgs) (CreateRuleResult, error) {
		rule, err := args.rule()
		if err != nil {
			return CreateRuleResult{}, err
		}

		resp, err := deps.Clients.Rules.Create(ctx, rule)
		if err != nil {
			return CreateRuleResult{}, fmt.Errorf("can't create the rule: %w", err)
		}

		return CreateRuleResult{ID: resp.GetId(), Name: rule.GetName(), Note: createdRuleNote}, nil
	}
}

// setRuleNotifyTargets reads the rule back and writes it again with new
// targets: the enforcer's Update takes a whole rule, and building one from the
// arguments alone would quietly reset everything that was not passed.
func setRuleNotifyTargets(deps *Deps) func(context.Context, SetRuleNotifyTargetsArgs) (SetRuleNotifyTargetsResult, error) {
	return func(ctx context.Context, args SetRuleNotifyTargetsArgs) (SetRuleNotifyTargetsResult, error) {
		id := strings.TrimSpace(args.ID)
		if id == "" {
			return SetRuleNotifyTargetsResult{}, fmt.Errorf("id is required")
		}

		resp, err := deps.Clients.Rules.Read(ctx, &enf_api.ReadRuleReq{Id: id})
		if err != nil {
			return SetRuleNotifyTargetsResult{}, fmt.Errorf("can't read the rule: %w", err)
		}

		rule := resp.GetRule()
		if rule == nil {
			return SetRuleNotifyTargetsResult{}, fmt.Errorf("no rule with id %s", id)
		}

		notify := rule.GetRule().GetNotify()
		if notify == nil {
			return SetRuleNotifyTargetsResult{}, fmt.Errorf(
				"the rule %q does not notify at all: it has no notify severity, so there is nothing to deliver",
				rule.GetName())
		}

		notify.Targets = args.NotifyTargets

		if _, err := deps.Clients.Rules.Update(ctx, rule); err != nil {
			return SetRuleNotifyTargetsResult{}, fmt.Errorf("can't update the rule: %w", err)
		}

		note := "the rule now sends through these services; a notification template for the event type has to exist too"
		if len(args.NotifyTargets) == 0 {
			note = "the rule now notifies nobody and fires silently"
		}

		return SetRuleNotifyTargetsResult{
			ID:            id,
			Name:          rule.GetName(),
			NotifyTargets: args.NotifyTargets,
			Note:          note,
		}, nil
	}
}

func deleteRule(deps *Deps) func(context.Context, DeleteRuleArgs) (DeleteRuleResult, error) {
	return func(ctx context.Context, args DeleteRuleArgs) (DeleteRuleResult, error) {
		id := strings.TrimSpace(args.ID)
		if id == "" {
			return DeleteRuleResult{}, fmt.Errorf("id is required")
		}

		if _, err := deps.Clients.Rules.Delete(ctx, &enf_api.DeleteRuleReq{Id: id}); err != nil {
			return DeleteRuleResult{}, fmt.Errorf("can't delete the rule: %w", err)
		}

		return DeleteRuleResult{ID: id, Deleted: true}, nil
	}
}

// rule turns the arguments into the request Policy Enforcer takes, filling in
// the schema versions and rejecting what the enforcer would reject anyway, so
// that the model gets a usable message instead of a gRPC error.
func (a CreateRuleArgs) rule() (*enf_api.Rule, error) {
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	block, err := severity(a.BlockSeverity)
	if err != nil {
		return nil, fmt.Errorf("block_severity: %w", err)
	}

	notify, err := severity(a.NotifySeverity)
	if err != nil {
		return nil, fmt.Errorf("notify_severity: %w", err)
	}

	if block == "" && notify == "" {
		return nil, fmt.Errorf("a rule has to react: set block_severity, notify_severity or both")
	}

	rule := &enf_api.Rule{
		Name: name,
		Type: enf_api.Rule_TYPE_RUNTIME,
		Rule: &enf_api.Rule_RuleJSON{
			Version: ruleVersion,
			Whitelist: &enf_api.Rule_RuleJSON_Whitelist{
				Threats:  a.WhitelistThreats,
				Binaries: a.WhitelistBinaries,
			},
		},
		Scope: &enf_api.Rule_Scope{
			Version:    scopeVersion,
			Clusters:   a.Scope.Clusters,
			Namespaces: a.Scope.Namespaces,
			Pods:       a.Scope.Pods,
			Containers: a.Scope.Containers,
			Nodes:      a.Scope.Nodes,
			ImageNames: a.Scope.ImageNames,
			Registries: a.Scope.Registries,
		},
	}

	if block != "" {
		rule.Rule.Block = &enf_api.Rule_RuleJSON_Block{Severity: block}
	}

	if notify != "" {
		rule.Rule.Notify = &enf_api.Rule_RuleJSON_Notify{Severity: notify, Targets: a.NotifyTargets}
	}

	return rule, nil
}

// severity normalises what the model wrote into the spelling Policy Enforcer
// accepts. An empty value means the rule does not use that reaction at all.
func severity(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))

	switch normalized {
	case "", ruleSeverityNone, ruleSeverityLow, ruleSeverityMedium, ruleSeverityHigh, ruleSeverityCritical:
		return normalized, nil
	default:
		return "", fmt.Errorf("%q is not a severity, expected one of %s", value, severityValues)
	}
}

func ruleInfo(rule *enf_api.Rule) RuleInfo {
	return RuleInfo{
		ID:                rule.GetId(),
		Name:              rule.GetName(),
		BlockSeverity:     rule.GetRule().GetBlock().GetSeverity(),
		NotifySeverity:    rule.GetRule().GetNotify().GetSeverity(),
		NotifyTargets:     rule.GetRule().GetNotify().GetTargets(),
		WhitelistThreats:  rule.GetRule().GetWhitelist().GetThreats(),
		WhitelistBinaries: rule.GetRule().GetWhitelist().GetBinaries(),
		Scope: RuleScope{
			Clusters:   rule.GetScope().GetClusters(),
			Namespaces: rule.GetScope().GetNamespaces(),
			Pods:       rule.GetScope().GetPods(),
			Containers: rule.GetScope().GetContainers(),
			Nodes:      rule.GetScope().GetNodes(),
			ImageNames: rule.GetScope().GetImageNames(),
			Registries: rule.GetScope().GetRegistries(),
		},
	}
}
