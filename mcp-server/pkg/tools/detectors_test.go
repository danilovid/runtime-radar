package tools

import (
	"context"
	"testing"

	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
)

func TestListDetectors(t *testing.T) {
	t.Parallel()

	detectors := &mockDetectors{detectors: []*processor_api.Detector{
		{
			Id:          detectorCryptominer,
			Name:        "Cryptominer execution",
			Description: "The detector detects if any known cryptominers were started or stopped. See MITRE T1496.",
			Version:     2,
			Author:      "Runtime Radar Team",
			License:     "Apache License 2.0",
		},
		{
			Id:          detectorSuspShell,
			Name:        "Suspicious shell",
			Description: "The detector detects an interactive shell started inside a container.",
			Version:     1,
		},
	}}

	deps := newTestDeps(t, nil, nil, detectors)

	result, err := listDetectors(deps)(context.Background(), ListDetectorsArgs{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 2 || result.Total != 2 {
		t.Fatalf("count = %d, total = %d, want 2 and 2", result.Count, result.Total)
	}

	if got := result.Detectors[0]; got.ID != detectorCryptominer || got.Version != 2 || got.License != "Apache License 2.0" {
		t.Errorf("first detector = %+v", got)
	}

	if got := result.Detectors[0].MitreIDs; len(got) != 1 || got[0] != "T1496" {
		t.Errorf("mitre ids = %v, want [T1496]", got)
	}
	if got := result.Detectors[1].MitreIDs; got != nil {
		t.Errorf("mitre ids = %v, want none when the description mentions none", got)
	}

	if result.Note == "" {
		t.Error("note is empty: a caller expecting a severity field needs to be told where severity lives")
	}
}

func TestListDetectorsSearch(t *testing.T) {
	t.Parallel()

	detectors := &mockDetectors{detectors: []*processor_api.Detector{
		{Id: detectorCryptominer, Name: "Cryptominer execution", Description: "miners"},
		{Id: detectorSuspShell, Name: "Suspicious shell", Description: "shells"},
	}}

	deps := newTestDeps(t, nil, nil, detectors)

	result, err := listDetectors(deps)(context.Background(), ListDetectorsArgs{Search: "shell"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 || result.Detectors[0].ID != detectorSuspShell {
		t.Fatalf("detectors = %+v", result.Detectors)
	}

	// The total reports the installed set, not the filtered one.
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
}

func TestMitreIDs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "technique", text: "maps to T1059 execution", want: []string{"T1059"}},
		{name: "sub-technique", text: "T1059.004 unix shell", want: []string{"T1059.004"}},
		{name: "tactic", text: "tactic TA0002", want: []string{"TA0002"}},
		{name: "deduplicated and sorted", text: "T1496 and T1059 and T1496", want: []string{"T1059", "T1496"}},
		{name: "no false positives on identifiers", text: "CS_RT_CVE_2022_0492 detects T99", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := mitreIDs(tc.text)

			if len(got) != len(tc.want) {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ids = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
