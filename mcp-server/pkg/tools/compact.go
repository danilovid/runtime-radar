package tools

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cilium/tetragon/api/v1/tetragon"
	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// maxSummaryArgsBytes bounds process arguments in a search result. A slice
	// of fifty events must stay readable in a model's context window.
	maxSummaryArgsBytes = 512
	// maxDetailArgsBytes bounds process arguments of a single event, which is
	// asked for precisely to look at them.
	maxDetailArgsBytes = 4 * 1024
	// maxKprobeArgs and maxKprobeArgBytes bound the rendered kprobe arguments.
	maxKprobeArgs     = 16
	maxKprobeArgBytes = 512
	// maxStackFrames bounds a stack trace, which is unbounded in the raw event.
	maxStackFrames = 20
	// maxThreats and maxDetectErrors bound lists that are short in practice but
	// unbounded in the API.
	maxThreats      = 32
	maxDetectErrors = 32

	truncationSuffix = "…[truncated]"
)

// eventTypes maps the runtime event types the product knows to the tetragon
// oneof they are stored in. The values match history-api's RuntimeEventType*
// constants, which is what the filter API expects.
const (
	eventTypeProcessExec       = "PROCESS_EXEC"
	eventTypeProcessExit       = "PROCESS_EXIT"
	eventTypeProcessKprobe     = "PROCESS_KPROBE"
	eventTypeProcessTracepoint = "PROCESS_TRACEPOINT"
	eventTypeProcessLoader     = "PROCESS_LOADER"
	eventTypeProcessUprobe     = "PROCESS_UPROBE"
	eventTypeUndef             = "UNDEF"
)

// protoJSON renders the bits of an event that have no fixed shape, such as
// kprobe arguments. Proto names keep the rendering equal to what the product's
// HTTP API returns.
var protoJSON = protojson.MarshalOptions{UseProtoNames: true}

// ThreatInfo is a detection reported on an event.
type ThreatInfo struct {
	DetectorID   string `json:"detector_id" jsonschema:"identifier of the detector that reported the threat"`
	DetectorName string `json:"detector_name,omitempty" jsonschema:"human readable name of the detector"`
	Severity     string `json:"severity,omitempty" jsonschema:"severity of the detection: NONE, LOW, MEDIUM, HIGH or CRITICAL"`
}

// DetectErrorInfo is a detector that failed to run on an event. It matters
// because a missing detection is not the same thing as no threat.
type DetectErrorInfo struct {
	DetectorID   string `json:"detector_id" jsonschema:"identifier of the detector that failed"`
	DetectorName string `json:"detector_name,omitempty" jsonschema:"human readable name of the detector"`
	Error        string `json:"error,omitempty" jsonschema:"error the detector failed with"`
}

// EventSummary is the compact form of a runtime event: what a triage question
// needs, without the raw Tetragon payload.
type EventSummary struct {
	ID   string `json:"id" jsonschema:"identifier of the event, pass it to get_runtime_event or get_process_context"`
	Time string `json:"time,omitempty" jsonschema:"RFC3339 timestamp of the event"`
	Type string `json:"type" jsonschema:"type of the event: PROCESS_EXEC, PROCESS_EXIT, PROCESS_KPROBE, PROCESS_TRACEPOINT, PROCESS_LOADER or PROCESS_UPROBE"`
	Node string `json:"node,omitempty" jsonschema:"name of the node the event was observed on"`

	Namespace string `json:"namespace,omitempty" jsonschema:"Kubernetes namespace of the pod"`
	Pod       string `json:"pod,omitempty" jsonschema:"name of the pod"`
	Container string `json:"container,omitempty" jsonschema:"name of the container"`
	Image     string `json:"image,omitempty" jsonschema:"image of the container"`
	Workload  string `json:"workload,omitempty" jsonschema:"Kubernetes workload of the pod"`

	Binary             string `json:"binary,omitempty" jsonschema:"absolute path of the executed binary"`
	Arguments          string `json:"arguments,omitempty" jsonschema:"arguments the binary was executed with, truncated to 512 bytes and with credential-shaped values masked"`
	ArgumentsTruncated bool   `json:"arguments_truncated,omitempty" jsonschema:"true when the arguments were truncated"`

	ExecID       string `json:"process_exec_id,omitempty" jsonschema:"execution identifier of the process, unique over time across the cluster"`
	ParentExecID string `json:"process_parent_exec_id,omitempty" jsonschema:"execution identifier of the parent process"`

	FunctionName string `json:"function_name,omitempty" jsonschema:"kprobe function the event was raised on"`
	Action       string `json:"action,omitempty" jsonschema:"action Tetragon performed when the kprobe matched"`
	Subsys       string `json:"tracepoint_subsys,omitempty" jsonschema:"subsystem of the tracepoint"`
	Event        string `json:"tracepoint_event,omitempty" jsonschema:"event of the tracepoint subsystem"`
	Symbol       string `json:"uprobe_symbol,omitempty" jsonschema:"symbol the uprobe was attached to"`
	Path         string `json:"path,omitempty" jsonschema:"path reported by a uprobe or loader event"`
	ExitSignal   string `json:"exit_signal,omitempty" jsonschema:"signal the process exited with"`
	ExitStatus   uint32 `json:"exit_status,omitempty" jsonschema:"status code the process exited with"`

	Threats          []ThreatInfo `json:"threats,omitempty" jsonschema:"threats detectors reported on this event"`
	IsIncident       bool         `json:"is_incident,omitempty" jsonschema:"true when a policy rule turned this event into an incident"`
	IncidentSeverity string       `json:"incident_severity,omitempty" jsonschema:"severity of the incident"`
}

// ProcessInfo is the compact form of a process taking part in an event.
type ProcessInfo struct {
	ExecID             string `json:"exec_id,omitempty" jsonschema:"execution identifier of the process"`
	ParentExecID       string `json:"parent_exec_id,omitempty" jsonschema:"execution identifier of the parent process"`
	PID                uint32 `json:"pid,omitempty" jsonschema:"process identifier in the host PID namespace"`
	UID                uint32 `json:"uid,omitempty" jsonschema:"effective user identifier of the process"`
	Binary             string `json:"binary,omitempty" jsonschema:"absolute path of the executed binary"`
	Arguments          string `json:"arguments,omitempty" jsonschema:"arguments the binary was executed with, truncated and with credential-shaped values masked"`
	ArgumentsTruncated bool   `json:"arguments_truncated,omitempty" jsonschema:"true when the arguments were truncated"`
	CWD                string `json:"cwd,omitempty" jsonschema:"current working directory of the process"`
	Flags              string `json:"flags,omitempty" jsonschema:"Tetragon flags of the process, for debugging purposes only"`
	StartTime          string `json:"start_time,omitempty" jsonschema:"RFC3339 timestamp the process started at"`
	ContainerID        string `json:"container_id,omitempty" jsonschema:"identifier of the container the process runs in"`
	InInitTree         bool   `json:"in_init_tree,omitempty" jsonschema:"true when the process belongs to the container's init process tree"`
}

// KprobeArg is one rendered argument of a kprobe or tracepoint event.
type KprobeArg struct {
	Label string `json:"label,omitempty" jsonschema:"label of the argument as defined by the tracing policy"`
	Value string `json:"value" jsonschema:"JSON rendering of the argument, truncated and with credential-shaped values masked"`
}

// StackFrame is one frame of a kernel or user stack trace.
type StackFrame struct {
	Symbol  string `json:"symbol,omitempty" jsonschema:"symbol name of the function"`
	Offset  uint64 `json:"offset,omitempty" jsonschema:"offset into the native instructions of the function"`
	Address uint64 `json:"address,omitempty" jsonschema:"linear address of the function"`
	Module  string `json:"module,omitempty" jsonschema:"module path for user space addresses"`
}

// EventDetail is the full, but bounded, form of a single runtime event.
type EventDetail struct {
	EventSummary

	TetragonVersion string            `json:"tetragon_version,omitempty" jsonschema:"version of Tetragon that produced the event"`
	PodLabels       map[string]string `json:"pod_labels,omitempty" jsonschema:"labels of the pod"`
	WorkloadKind    string            `json:"workload_kind,omitempty" jsonschema:"Kubernetes workload kind of the pod"`

	Process *ProcessInfo `json:"process,omitempty" jsonschema:"process that triggered the event"`
	Parent  *ProcessInfo `json:"parent,omitempty" jsonschema:"immediate parent of the process"`

	PolicyName string      `json:"policy_name,omitempty" jsonschema:"name of the tracing policy that created the probe"`
	Message    string      `json:"message,omitempty" jsonschema:"short message of the tracing policy"`
	Tags       []string    `json:"tags,omitempty" jsonschema:"tags of the tracing policy"`
	Args       []KprobeArg `json:"args,omitempty" jsonschema:"arguments of the observed kprobe or tracepoint, truncated"`
	Return     string      `json:"return,omitempty" jsonschema:"return value of the observed kprobe, truncated"`

	KernelStackTrace []StackFrame `json:"kernel_stack_trace,omitempty" jsonschema:"kernel stack trace of the call, truncated to 20 frames"`
	UserStackTrace   []StackFrame `json:"user_stack_trace,omitempty" jsonschema:"user space stack trace of the call, truncated to 20 frames"`

	DetectErrors []DetectErrorInfo `json:"detect_errors,omitempty" jsonschema:"detectors that failed to run on this event, so their verdict is unknown"`
	BlockBy      []string          `json:"block_by,omitempty" jsonschema:"identifiers of the policy rules that blocked the process"`
	NotifyBy     []string          `json:"notify_by,omitempty" jsonschema:"identifiers of the policy rules that raised a notification"`
}

// summarize turns a runtime event into its compact form.
func summarize(event *processor_api.RuntimeEvent) EventSummary {
	summary := EventSummary{
		ID:               event.GetId(),
		Node:             event.GetEvent().GetNodeName(),
		IsIncident:       event.GetIsIncident(),
		IncidentSeverity: event.GetIncidentSeverity(),
		Threats:          threats(event.GetThreats()),
	}

	if ts := event.GetEvent().GetTime(); ts != nil {
		summary.Time = ts.AsTime().Format(time.RFC3339Nano)
	}

	process, _ := processes(event.GetEvent())
	summary.Type = eventType(event.GetEvent())

	if process != nil {
		summary.Binary = process.GetBinary()
		summary.Arguments, summary.ArgumentsTruncated = truncate(redactSecrets(process.GetArguments()), maxSummaryArgsBytes)
		summary.ExecID = process.GetExecId()
		summary.ParentExecID = process.GetParentExecId()

		if pod := process.GetPod(); pod != nil {
			summary.Namespace = pod.GetNamespace()
			summary.Pod = pod.GetName()
			summary.Workload = pod.GetWorkload()
			summary.Container = pod.GetContainer().GetName()
			summary.Image = pod.GetContainer().GetImage().GetName()
		}
	}

	switch e := event.GetEvent().GetEvent().(type) {
	case *tetragon.GetEventsResponse_ProcessKprobe:
		summary.FunctionName = e.ProcessKprobe.GetFunctionName()
		summary.Action = e.ProcessKprobe.GetAction().String()
	case *tetragon.GetEventsResponse_ProcessTracepoint:
		summary.Subsys = e.ProcessTracepoint.GetSubsys()
		summary.Event = e.ProcessTracepoint.GetEvent()
	case *tetragon.GetEventsResponse_ProcessUprobe:
		summary.Symbol = e.ProcessUprobe.GetSymbol()
		summary.Path = e.ProcessUprobe.GetPath()
	case *tetragon.GetEventsResponse_ProcessLoader:
		summary.Path = e.ProcessLoader.GetPath()
	case *tetragon.GetEventsResponse_ProcessExit:
		summary.ExitSignal = e.ProcessExit.GetSignal()
		summary.ExitStatus = e.ProcessExit.GetStatus()
	}

	return summary
}

// detail turns a runtime event into its full, bounded form.
func detail(event *processor_api.RuntimeEvent) EventDetail {
	process, parent := processes(event.GetEvent())

	result := EventDetail{
		EventSummary:    summarize(event),
		TetragonVersion: event.GetTetragonVersion(),
		Process:         processInfo(process),
		Parent:          processInfo(parent),
		DetectErrors:    detectErrors(event.GetDetectErrors()),
		BlockBy:         event.GetBlockBy(),
		NotifyBy:        event.GetNotifyBy(),
	}

	if pod := process.GetPod(); pod != nil {
		result.PodLabels = pod.GetPodLabels()
		result.WorkloadKind = pod.GetWorkloadKind()
	}

	switch e := event.GetEvent().GetEvent().(type) {
	case *tetragon.GetEventsResponse_ProcessKprobe:
		result.PolicyName = e.ProcessKprobe.GetPolicyName()
		result.Message = e.ProcessKprobe.GetMessage()
		result.Tags = e.ProcessKprobe.GetTags()
		result.Args = kprobeArgs(e.ProcessKprobe.GetArgs())
		result.Return = kprobeArgValue(e.ProcessKprobe.GetReturn())
		result.KernelStackTrace = stackTrace(e.ProcessKprobe.GetKernelStackTrace())
		result.UserStackTrace = stackTrace(e.ProcessKprobe.GetUserStackTrace())
	case *tetragon.GetEventsResponse_ProcessTracepoint:
		result.PolicyName = e.ProcessTracepoint.GetPolicyName()
		result.Args = kprobeArgs(e.ProcessTracepoint.GetArgs())
	case *tetragon.GetEventsResponse_ProcessUprobe:
		result.PolicyName = e.ProcessUprobe.GetPolicyName()
		result.Args = kprobeArgs(e.ProcessUprobe.GetArgs())
	}

	// The single-event view is asked for precisely to read the command line, so
	// it gets a larger, but still bounded, budget than a search result does.
	if result.Process != nil {
		result.Arguments, result.ArgumentsTruncated = result.Process.Arguments, result.Process.ArgumentsTruncated
	}

	return result
}

// eventType names the runtime event type the way the filter API expects it.
func eventType(event *tetragon.GetEventsResponse) string {
	switch event.GetEvent().(type) {
	case *tetragon.GetEventsResponse_ProcessExec:
		return eventTypeProcessExec
	case *tetragon.GetEventsResponse_ProcessExit:
		return eventTypeProcessExit
	case *tetragon.GetEventsResponse_ProcessKprobe:
		return eventTypeProcessKprobe
	case *tetragon.GetEventsResponse_ProcessTracepoint:
		return eventTypeProcessTracepoint
	case *tetragon.GetEventsResponse_ProcessLoader:
		return eventTypeProcessLoader
	case *tetragon.GetEventsResponse_ProcessUprobe:
		return eventTypeProcessUprobe
	default:
		return eventTypeUndef
	}
}

// processes returns the process that triggered the event and its parent, for
// whichever event type this is.
func processes(event *tetragon.GetEventsResponse) (process, parent *tetragon.Process) {
	switch e := event.GetEvent().(type) {
	case *tetragon.GetEventsResponse_ProcessExec:
		return e.ProcessExec.GetProcess(), e.ProcessExec.GetParent()
	case *tetragon.GetEventsResponse_ProcessExit:
		return e.ProcessExit.GetProcess(), e.ProcessExit.GetParent()
	case *tetragon.GetEventsResponse_ProcessKprobe:
		return e.ProcessKprobe.GetProcess(), e.ProcessKprobe.GetParent()
	case *tetragon.GetEventsResponse_ProcessTracepoint:
		return e.ProcessTracepoint.GetProcess(), e.ProcessTracepoint.GetParent()
	case *tetragon.GetEventsResponse_ProcessLoader:
		return e.ProcessLoader.GetProcess(), nil
	case *tetragon.GetEventsResponse_ProcessUprobe:
		return e.ProcessUprobe.GetProcess(), e.ProcessUprobe.GetParent()
	default:
		return nil, nil
	}
}

func processInfo(process *tetragon.Process) *ProcessInfo {
	if process == nil {
		return nil
	}

	info := &ProcessInfo{
		ExecID:       process.GetExecId(),
		ParentExecID: process.GetParentExecId(),
		PID:          process.GetPid().GetValue(),
		UID:          process.GetUid().GetValue(),
		Binary:       process.GetBinary(),
		CWD:          process.GetCwd(),
		Flags:        process.GetFlags(),
		ContainerID:  process.GetDocker(),
		InInitTree:   process.GetInInitTree().GetValue(),
	}

	info.Arguments, info.ArgumentsTruncated = truncate(redactSecrets(process.GetArguments()), maxDetailArgsBytes)

	if ts := process.GetStartTime(); ts != nil {
		info.StartTime = ts.AsTime().Format(time.RFC3339Nano)
	}

	return info
}

func threats(list []*processor_api.Threat) []ThreatInfo {
	if len(list) == 0 {
		return nil
	}

	if len(list) > maxThreats {
		list = list[:maxThreats]
	}

	result := make([]ThreatInfo, 0, len(list))
	for _, threat := range list {
		result = append(result, ThreatInfo{
			DetectorID:   threat.GetDetector().GetId(),
			DetectorName: threat.GetDetector().GetName(),
			Severity:     threat.GetSeverity(),
		})
	}

	return result
}

func detectErrors(list []*processor_api.DetectError) []DetectErrorInfo {
	if len(list) == 0 {
		return nil
	}

	if len(list) > maxDetectErrors {
		list = list[:maxDetectErrors]
	}

	result := make([]DetectErrorInfo, 0, len(list))
	for _, detectError := range list {
		message, _ := truncate(detectError.GetError(), maxKprobeArgBytes)
		result = append(result, DetectErrorInfo{
			DetectorID:   detectError.GetDetector().GetId(),
			DetectorName: detectError.GetDetector().GetName(),
			Error:        message,
		})
	}

	return result
}

func kprobeArgs(args []*tetragon.KprobeArgument) []KprobeArg {
	if len(args) == 0 {
		return nil
	}

	if len(args) > maxKprobeArgs {
		args = args[:maxKprobeArgs]
	}

	result := make([]KprobeArg, 0, len(args))
	for _, arg := range args {
		result = append(result, KprobeArg{Label: arg.GetLabel(), Value: kprobeArgValue(arg)})
	}

	return result
}

// kprobeArgValue renders an argument whose shape depends on the traced call.
// Rendering the proto as JSON keeps every argument kind readable without this
// service having to know all thirty of them.
func kprobeArgValue(arg *tetragon.KprobeArgument) string {
	if arg == nil {
		return ""
	}

	rendered, err := protoJSON.Marshal(arg)
	if err != nil {
		return ""
	}

	value, _ := truncate(redactSecrets(string(rendered)), maxKprobeArgBytes)

	return value
}

func stackTrace(entries []*tetragon.StackTraceEntry) []StackFrame {
	if len(entries) == 0 {
		return nil
	}

	if len(entries) > maxStackFrames {
		entries = entries[:maxStackFrames]
	}

	frames := make([]StackFrame, 0, len(entries))
	for _, entry := range entries {
		frames = append(frames, StackFrame{
			Symbol:  entry.GetSymbol(),
			Offset:  entry.GetOffset(),
			Address: entry.GetAddress(),
			Module:  entry.GetModule(),
		})
	}

	return frames
}

// truncate cuts text to at most limit bytes, on a rune boundary, and reports
// whether anything was cut.
func truncate(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}

	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}

	return strings.TrimRight(text[:cut], " ") + truncationSuffix, true
}
