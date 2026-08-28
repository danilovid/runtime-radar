package tools

import (
	"context"
	"testing"

	enf_api "github.com/runtime-radar/runtime-radar/policy-enforcer/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// testRuleID is the identifier the mocked Policy Enforcer hands back.
const testRuleID = "rule-1"

// mockRules records what the rule tools send to Policy Enforcer.
type mockRules struct {
	enf_api.RuleControllerClient

	listResp  *enf_api.ListRulePageResp
	createReq *enf_api.Rule
	deleteReq *enf_api.DeleteRuleReq
}

func (m *mockRules) ListPage(_ context.Context, _ *enf_api.ListRulePageReq, _ ...grpc.CallOption) (*enf_api.ListRulePageResp, error) {
	if m.listResp == nil {
		return &enf_api.ListRulePageResp{}, nil
	}

	return m.listResp, nil
}

func (m *mockRules) Create(_ context.Context, req *enf_api.Rule, _ ...grpc.CallOption) (*enf_api.CreateRuleResp, error) {
	m.createReq = req

	return &enf_api.CreateRuleResp{Id: testRuleID}, nil
}

func (m *mockRules) Delete(_ context.Context, req *enf_api.DeleteRuleReq, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	m.deleteReq = req

	return &emptypb.Empty{}, nil
}

func TestListRules(t *testing.T) {
	t.Parallel()

	rules := &mockRules{listResp: &enf_api.ListRulePageResp{
		Total: 2,
		Rules: []*enf_api.Rule{
			{
				Id:   testRuleID,
				Name: "Block cryptominers in prod",
				Rule: &enf_api.Rule_RuleJSON{
					Version: ruleVersion,
					Block:   &enf_api.Rule_RuleJSON_Block{Severity: ruleSeverityHigh},
				},
				Scope: &enf_api.Rule_Scope{Version: scopeVersion, Namespaces: []string{testNamespace}},
			},
			{
				Id:   "rule-2",
				Name: "Notify on shells",
				Rule: &enf_api.Rule_RuleJSON{
					Version: ruleVersion,
					Notify:  &enf_api.Rule_RuleJSON_Notify{Severity: "low", Targets: []string{"integration-1"}},
				},
				Scope: &enf_api.Rule_Scope{Version: scopeVersion},
			},
		},
	}}

	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.Clients.Rules = rules

	result, err := listRules(deps)(context.Background(), ListRulesArgs{Search: "crypto"})
	if err != nil {
		t.Fatalf("can't list rules: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("total is %d, want 2", result.Total)
	}
	if result.Count != 1 || len(result.Rules) != 1 {
		t.Fatalf("the search returned %d rules, want 1", result.Count)
	}
	if result.Rules[0].ID != testRuleID {
		t.Errorf("returned rule %q, want rule-1", result.Rules[0].ID)
	}
	if result.Rules[0].BlockSeverity != ruleSeverityHigh {
		t.Errorf("block severity is %q, want high", result.Rules[0].BlockSeverity)
	}
	if len(result.Rules[0].Scope.Namespaces) != 1 || result.Rules[0].Scope.Namespaces[0] != testNamespace {
		t.Errorf("scope namespaces are %v, want [%s]", result.Rules[0].Scope.Namespaces, testNamespace)
	}
}

func TestCreateRule(t *testing.T) {
	t.Parallel()

	rules := &mockRules{}
	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.Clients.Rules = rules

	result, err := createRule(deps)(context.Background(), CreateRuleArgs{
		Name:          "Block cryptominers in prod",
		BlockSeverity: "High",
		Scope:         RuleScope{Namespaces: []string{testNamespace}},
	})
	if err != nil {
		t.Fatalf("can't create the rule: %v", err)
	}

	if result.ID != testRuleID {
		t.Errorf("created rule %q, want rule-1", result.ID)
	}
	if result.Note == "" {
		t.Error("the answer does not say what the user should check now")
	}

	// The schema versions describe the product's own format, so they are filled
	// in here rather than asked of the model.
	if rules.createReq.GetRule().GetVersion() != ruleVersion {
		t.Errorf("rule version is %q, want %q", rules.createReq.GetRule().GetVersion(), ruleVersion)
	}
	if rules.createReq.GetScope().GetVersion() != scopeVersion {
		t.Errorf("scope version is %q, want %q", rules.createReq.GetScope().GetVersion(), scopeVersion)
	}
	// Policy Enforcer only accepts lower case severities.
	if rules.createReq.GetRule().GetBlock().GetSeverity() != ruleSeverityHigh {
		t.Errorf("block severity is %q, want high", rules.createReq.GetRule().GetBlock().GetSeverity())
	}
	if rules.createReq.GetRule().GetNotify() != nil {
		t.Error("a rule that was not asked to notify has a notify block")
	}
}

func TestCreateRuleRejectsIncompleteArguments(t *testing.T) {
	t.Parallel()

	rules := &mockRules{}
	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.Clients.Rules = rules

	cases := map[string]CreateRuleArgs{
		"no name":          {BlockSeverity: ruleSeverityHigh},
		"no reaction":      {Name: "Rule"},
		"unknown severity": {Name: "Rule", BlockSeverity: "catastrophic"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := createRule(deps)(context.Background(), args); err == nil {
				t.Error("the arguments were accepted")
			}
		})
	}

	if rules.createReq != nil {
		t.Error("an invalid rule reached Policy Enforcer")
	}
}

func TestDeleteRule(t *testing.T) {
	t.Parallel()

	rules := &mockRules{}
	deps := newTestDeps(t, &mockRuntimeHistory{}, &mockRuntimeStats{}, &mockDetectors{})
	deps.Clients.Rules = rules

	if _, err := deleteRule(deps)(context.Background(), DeleteRuleArgs{ID: testRuleID}); err != nil {
		t.Fatalf("can't delete the rule: %v", err)
	}

	if rules.deleteReq.GetId() != testRuleID {
		t.Errorf("deleted %q, want rule-1", rules.deleteReq.GetId())
	}

	if _, err := deleteRule(deps)(context.Background(), DeleteRuleArgs{}); err == nil {
		t.Error("a delete without an identifier was accepted")
	}
}
