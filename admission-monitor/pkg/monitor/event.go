package monitor

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
)

const (
	// Kyverno reports every policy result of the admission webhook as a Kubernetes event on the
	// policy object. A blocked request is the one whose action is "Resource Blocked": an audited
	// violation carries "Resource Passed" instead and is already covered by policy reports.
	// See pkg/event/{reason,source,action}.go of kyverno/kyverno for the exact values.
	kyvernoEventReason     = "PolicyViolation"
	kyvernoAdmissionSource = "kyverno-admission"
	kyvernoBlockedAction   = "Resource Blocked"
)

// kyvernoRuleName pulls the name of the failed rule out of the event message. Kyverno builds the
// message as "<Kind> <namespace>/<name>: [<rule>] <status> (blocked); <details>" and the rule name
// appears nowhere else in the event, while a policy report has a field for it. Parsing keeps the
// threat identifier the same for a blocked and for an audited finding, so a rule whitelisting
// "<policy>/<rule>" works for both.
var kyvernoRuleName = regexp.MustCompile(`: \[([^\]]+)\] `)

// eventFromKubernetesEvent converts an event about a blocked request into an admission event.
// It returns nil for anything that is not a request Kyverno denied at admission, and for events
// produced before since, which belong to the previous run of the monitor.
func (k *Kyverno) eventFromKubernetesEvent(ev *corev1.Event, since time.Time) *api.AdmissionEvent {
	if ev.Reason != kyvernoEventReason || ev.ReportingController != kyvernoAdmissionSource {
		return nil
	}
	if ev.Action != kyvernoBlockedAction {
		return nil
	}
	if eventTime(ev).Before(since) {
		return nil
	}

	// The event is reported on the policy, the blocked resource is the related object.
	related := ev.Related
	if related == nil {
		log.Debug().Str("policy", ev.InvolvedObject.Name).Msgf("Blocked request event has no related resource")
		return nil
	}
	if related.Namespace == k.namespace {
		return nil
	}

	p, ok := k.policiesByName()[ev.InvolvedObject.Name]
	if !ok {
		// Kyverno may be shared with policies managed outside of Runtime Radar.
		return nil
	}

	rule := ""
	if m := kyvernoRuleName.FindStringSubmatch(ev.Message); len(m) > 1 {
		rule = m[1]
	}

	return &api.AdmissionEvent{
		Id:             uuid.NewString(),
		KyvernoVersion: k.Version,
		RegisteredAt:   timestamppb.New(eventTime(ev)),
		Resource: &api.Resource{
			ApiVersion: related.APIVersion,
			Kind:       related.Kind,
			Namespace:  related.Namespace,
			Name:       related.Name,
			Uid:        string(related.UID),
			// A blocked resource never exists, so there is nothing to read containers from.
		},
		Threats: []*api.Threat{{
			Policy: &api.Policy{
				Id:          ev.InvolvedObject.Name + "/" + rule,
				Name:        p.GetName(),
				Rule:        rule,
				Description: eventDetails(ev.Message),
				Source:      ev.InvolvedObject.Kind,
				Action:      p.GetAction(),
			},
			Severity: p.GetSeverity(),
		}},
		Blocked: true,
	}
}

// eventTime returns the time the event happened. Kyverno writes events through the events.k8s.io
// API, which fills event_time; the legacy timestamps are kept as a fallback.
func eventTime(ev *corev1.Event) time.Time {
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.FirstTimestamp.IsZero() {
		return ev.FirstTimestamp.Time
	}

	return ev.CreationTimestamp.Time
}

// eventDetails returns the part of the message Kyverno appends after "; ", which is the reason the
// rule reported. The prefix repeats the resource and the rule we already put into the event.
func eventDetails(message string) string {
	if _, details, found := strings.Cut(message, "; "); found {
		return strings.TrimSpace(details)
	}

	return strings.TrimSpace(message)
}
