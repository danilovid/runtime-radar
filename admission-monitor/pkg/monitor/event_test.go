package monitor

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func kubeEvent(reason, controller, action, message string, ts time.Time, related *corev1.ObjectReference) *corev1.Event {
	return &corev1.Event{
		ObjectMeta:          metav1.ObjectMeta{Name: "privileged-containers.1", Namespace: "default"},
		Reason:              reason,
		ReportingController: controller,
		Action:              action,
		Message:             message,
		EventTime:           metav1.MicroTime{Time: ts},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "policies.kyverno.io/v1",
			Kind:       "ValidatingPolicy",
			Name:       "privileged-containers",
		},
		Related: related,
	}
}

func blockedResource() *corev1.ObjectReference {
	return &corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       "nginx",
		Namespace:  "default",
		UID:        "9a2f",
	}
}

func TestEventFromKubernetesEvent(t *testing.T) {
	since := time.Now().Add(-time.Minute)
	now := time.Now()
	old := since.Add(-time.Hour)

	const message = "Pod default/nginx: [privileged-containers] fail (blocked); Privileged containers are not allowed."

	tests := []struct {
		name    string
		event   *corev1.Event
		wantNil bool
	}{
		{
			name:  "blocked request",
			event: kubeEvent(kyvernoEventReason, kyvernoAdmissionSource, kyvernoBlockedAction, message, now, blockedResource()),
		},
		{
			name:    "audited violation is left to policy reports",
			event:   kubeEvent(kyvernoEventReason, kyvernoAdmissionSource, "Resource Passed", message, now, blockedResource()),
			wantNil: true,
		},
		{
			name:    "event of another controller",
			event:   kubeEvent(kyvernoEventReason, "kyverno-scan", kyvernoBlockedAction, message, now, blockedResource()),
			wantNil: true,
		},
		{
			name:    "another reason",
			event:   kubeEvent("PolicyApplied", kyvernoAdmissionSource, kyvernoBlockedAction, message, now, blockedResource()),
			wantNil: true,
		},
		{
			name:    "event produced before the monitor started",
			event:   kubeEvent(kyvernoEventReason, kyvernoAdmissionSource, kyvernoBlockedAction, message, old, blockedResource()),
			wantNil: true,
		},
		{
			name:    "event without a related resource",
			event:   kubeEvent(kyvernoEventReason, kyvernoAdmissionSource, kyvernoBlockedAction, message, now, nil),
			wantNil: true,
		},
	}

	k := testKyverno()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := k.eventFromKubernetesEvent(tt.event, since)

			if tt.wantNil {
				if ev != nil {
					t.Fatalf("expected no event, got %+v", ev)
				}
				return
			}

			if ev == nil {
				t.Fatal("expected an event, got none")
			}

			if !ev.GetBlocked() {
				t.Error("expected the event to be marked as blocked")
			}

			if got := ev.GetResource().GetName(); got != "nginx" {
				t.Errorf("expected resource taken from the related object, got '%s'", got)
			}

			if len(ev.GetThreats()) != 1 {
				t.Fatalf("expected 1 threat, got %d", len(ev.GetThreats()))
			}

			threat := ev.GetThreats()[0]
			if got := threat.GetPolicy().GetId(); got != "privileged-containers/privileged-containers" {
				t.Errorf("expected the rule name parsed out of the message, got '%s'", got)
			}
			if got := threat.GetSeverity(); got != testSeverity {
				t.Errorf("expected severity from the source, got '%s'", got)
			}
			if got := threat.GetPolicy().GetDescription(); got != "Privileged containers are not allowed." {
				t.Errorf("expected the details part of the message, got '%s'", got)
			}
		})
	}
}

func TestEventFromKubernetesEventUnknownPolicy(t *testing.T) {
	k := testKyverno()
	ev := kubeEvent(kyvernoEventReason, kyvernoAdmissionSource, kyvernoBlockedAction,
		"Pod default/nginx: [rule-0] fail (blocked); nope", time.Now(), blockedResource())
	ev.InvolvedObject.Name = "some-other-policy"

	if got := k.eventFromKubernetesEvent(ev, time.Now().Add(-time.Minute)); got != nil {
		t.Fatalf("expected policies managed outside of runtime radar to be skipped, got %+v", got)
	}
}
