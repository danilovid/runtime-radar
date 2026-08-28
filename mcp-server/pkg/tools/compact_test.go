package tools

import (
	"strings"
	"testing"
	"time"

	"github.com/cilium/tetragon/api/v1/tetragon"
	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTruncate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		text      string
		limit     int
		truncated bool
		want      string
	}{
		{name: "short text is untouched", text: "curl https://x", limit: 64, want: "curl https://x"},
		{name: "exact length is untouched", text: "abcd", limit: 4, want: "abcd"},
		{name: "long text is cut", text: "abcdefgh", limit: 4, truncated: true, want: "abcd" + truncationSuffix},
		{
			// A cut in the middle of a multibyte rune would produce invalid
			// UTF-8, which JSON encoding then mangles.
			name:      "multibyte text is cut on a rune boundary",
			text:      strings.Repeat("я", 10),
			limit:     5,
			truncated: true,
			want:      strings.Repeat("я", 2) + truncationSuffix,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, truncated := truncate(tc.text, tc.limit)

			if got != tc.want {
				t.Errorf("text = %q, want %q", got, tc.want)
			}
			if truncated != tc.truncated {
				t.Errorf("truncated = %v, want %v", truncated, tc.truncated)
			}
		})
	}
}

func TestSummarizeCompactsTheEvent(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 20, 10, 30, 0, 0, time.UTC)
	event := execEvent("11111111-1111-1111-1111-111111111111", testNamespace, "web-1", "/bin/sh", "-c whoami", at)
	event.Threats = []*processor_api.Threat{{
		Detector: &processor_api.Detector{Id: detectorSuspShell, Name: "Suspicious shell"},
		Severity: severityHigh,
	}}
	event.IsIncident = true
	event.IncidentSeverity = severityHigh

	summary := summarize(event)

	if summary.Time != at.Format(time.RFC3339Nano) {
		t.Errorf("time = %q, want %q", summary.Time, at.Format(time.RFC3339Nano))
	}
	if summary.Node != "node-1" {
		t.Errorf("node = %q", summary.Node)
	}
	if summary.Binary != "/bin/sh" || summary.Arguments != "-c whoami" {
		t.Errorf("binary/arguments = %q / %q", summary.Binary, summary.Arguments)
	}
	if summary.ExecID != "exec-"+event.GetId() || summary.ParentExecID != "parent-"+event.GetId() {
		t.Errorf("exec ids = %q / %q", summary.ExecID, summary.ParentExecID)
	}
	if len(summary.Threats) != 1 || summary.Threats[0].DetectorID != detectorSuspShell || summary.Threats[0].Severity != severityHigh {
		t.Errorf("threats = %+v", summary.Threats)
	}
	if !summary.IsIncident || summary.IncidentSeverity != severityHigh {
		t.Errorf("incident = %v / %q", summary.IsIncident, summary.IncidentSeverity)
	}
}

func TestSummarizeEventTypes(t *testing.T) {
	t.Parallel()

	process := &tetragon.Process{ExecId: "exec-1", Binary: "/bin/sh"}

	cases := []struct {
		name     string
		event    *tetragon.GetEventsResponse
		wantType string
		check    func(t *testing.T, summary EventSummary)
	}{
		{
			name: "kprobe carries the function and the action",
			event: &tetragon.GetEventsResponse{Event: &tetragon.GetEventsResponse_ProcessKprobe{
				ProcessKprobe: &tetragon.ProcessKprobe{
					Process:      process,
					FunctionName: "security_file_permission",
					Action:       tetragon.KprobeAction_KPROBE_ACTION_POST,
				},
			}},
			wantType: eventTypeProcessKprobe,
			check: func(t *testing.T, summary EventSummary) {
				if summary.FunctionName != "security_file_permission" {
					t.Errorf("function_name = %q", summary.FunctionName)
				}
				if summary.Action != tetragon.KprobeAction_KPROBE_ACTION_POST.String() {
					t.Errorf("action = %q", summary.Action)
				}
			},
		},
		{
			name: "exit carries the signal and the status",
			event: &tetragon.GetEventsResponse{Event: &tetragon.GetEventsResponse_ProcessExit{
				ProcessExit: &tetragon.ProcessExit{Process: process, Signal: "SIGKILL", Status: 137},
			}},
			wantType: eventTypeProcessExit,
			check: func(t *testing.T, summary EventSummary) {
				if summary.ExitSignal != "SIGKILL" || summary.ExitStatus != 137 {
					t.Errorf("exit = %q / %d", summary.ExitSignal, summary.ExitStatus)
				}
			},
		},
		{
			name: "tracepoint carries the subsystem and the event",
			event: &tetragon.GetEventsResponse{Event: &tetragon.GetEventsResponse_ProcessTracepoint{
				ProcessTracepoint: &tetragon.ProcessTracepoint{Process: process, Subsys: "raw_syscalls", Event: "sys_enter"},
			}},
			wantType: eventTypeProcessTracepoint,
			check: func(t *testing.T, summary EventSummary) {
				if summary.Subsys != "raw_syscalls" || summary.Event != "sys_enter" {
					t.Errorf("tracepoint = %q / %q", summary.Subsys, summary.Event)
				}
			},
		},
		{
			name:     "an event of an unknown shape is reported as undefined",
			event:    &tetragon.GetEventsResponse{},
			wantType: eventTypeUndef,
			check:    func(*testing.T, EventSummary) {},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.event.Time = timestamppb.New(testNow)
			summary := summarize(&processor_api.RuntimeEvent{Id: "id", Event: tc.event})

			if summary.Type != tc.wantType {
				t.Fatalf("type = %q, want %q", summary.Type, tc.wantType)
			}

			tc.check(t, summary)
		})
	}
}

func TestSummarizeMasksCredentials(t *testing.T) {
	t.Parallel()

	event := execEvent("id", testNamespace, "web-1", "/usr/bin/curl",
		`curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.body.signature" --password hunter2 https://api.test`, testNow)

	summary := summarize(event)

	for _, secret := range []string{"eyJhbGciOiJIUzI1NiJ9", "hunter2"} {
		if strings.Contains(summary.Arguments, secret) {
			t.Errorf("secret %q survived in %q", secret, summary.Arguments)
		}
	}

	// The shape of the command must survive, or the summary explains nothing.
	for _, kept := range []string{"curl", "Authorization", "https://api.test"} {
		if !strings.Contains(summary.Arguments, kept) {
			t.Errorf("%q was eaten from %q", kept, summary.Arguments)
		}
	}
}

func TestDetailBoundsArgumentsAndStacks(t *testing.T) {
	t.Parallel()

	frames := make([]*tetragon.StackTraceEntry, 0, maxStackFrames*2)
	for i := 0; i < maxStackFrames*2; i++ {
		frames = append(frames, &tetragon.StackTraceEntry{Symbol: "frame", Offset: uint64(i)})
	}

	args := make([]*tetragon.KprobeArgument, 0, maxKprobeArgs*2)
	for i := 0; i < maxKprobeArgs*2; i++ {
		args = append(args, &tetragon.KprobeArgument{
			Label: "path",
			Arg:   &tetragon.KprobeArgument_StringArg{StringArg: strings.Repeat("/very/long/path", 200)},
		})
	}

	event := &processor_api.RuntimeEvent{
		Id: "id",
		Event: &tetragon.GetEventsResponse{
			Time: timestamppb.New(testNow),
			Event: &tetragon.GetEventsResponse_ProcessKprobe{ProcessKprobe: &tetragon.ProcessKprobe{
				Process:          &tetragon.Process{ExecId: "exec-1", Binary: "/bin/cat", Arguments: strings.Repeat("/etc/passwd ", 1000)},
				FunctionName:     "security_file_permission",
				PolicyName:       "file-monitoring",
				Args:             args,
				KernelStackTrace: frames,
				UserStackTrace:   frames,
			}},
		},
		DetectErrors: []*processor_api.DetectError{{
			Detector: &processor_api.Detector{Id: "CS_RT_HACK_TOOLS"},
			Error:    "detector timed out",
		}},
	}

	result := detail(event)

	if len(result.Args) != maxKprobeArgs {
		t.Errorf("args = %d, want %d", len(result.Args), maxKprobeArgs)
	}
	for i, arg := range result.Args {
		if len(arg.Value) > maxKprobeArgBytes+len(truncationSuffix) {
			t.Errorf("arg[%d] length = %d, want at most %d", i, len(arg.Value), maxKprobeArgBytes+len(truncationSuffix))
		}
	}

	if len(result.KernelStackTrace) != maxStackFrames || len(result.UserStackTrace) != maxStackFrames {
		t.Errorf("stack traces = %d / %d, want %d each", len(result.KernelStackTrace), len(result.UserStackTrace), maxStackFrames)
	}

	if !result.ArgumentsTruncated || len(result.Arguments) > maxDetailArgsBytes+len(truncationSuffix) {
		t.Errorf("arguments = %d bytes, truncated = %v", len(result.Arguments), result.ArgumentsTruncated)
	}

	if result.PolicyName != "file-monitoring" {
		t.Errorf("policy_name = %q", result.PolicyName)
	}

	if len(result.DetectErrors) != 1 || result.DetectErrors[0].DetectorID != "CS_RT_HACK_TOOLS" {
		t.Errorf("detect_errors = %+v", result.DetectErrors)
	}
}

func TestDetailKeepsSummaryArgumentsSeparate(t *testing.T) {
	t.Parallel()

	// The single-event view gets the larger budget: an argument list longer
	// than a search summary allows must survive in full here.
	arguments := strings.Repeat("--flag /etc/hosts ", maxSummaryArgsBytes/8)
	event := execEvent("id", testNamespace, "web-1", "/bin/sh", arguments, testNow)

	result := detail(event)

	if result.ArgumentsTruncated {
		t.Errorf("arguments_truncated = true, want false for %d bytes", len(arguments))
	}
	if result.Arguments != arguments {
		t.Errorf("arguments were cut to %d bytes", len(result.Arguments))
	}
}
