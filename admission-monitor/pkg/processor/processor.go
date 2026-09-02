package processor

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/monitor"
	"github.com/runtime-radar/runtime-radar/lib/rabbit"
	notifier_api "github.com/runtime-radar/runtime-radar/notifier/api"
	enforcer_api "github.com/runtime-radar/runtime-radar/policy-enforcer/api"
	enforcer_model "github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/model"
)

const (
	actionType = "admission-review"
)

// Processor consumes admission events produced by the monitor, evaluates policy-enforcer rules
// against them and records the outcome in history-api and notifier.
//
// Unlike event-processor there is no detection step and no workers pool: threats are already
// decided by Kyverno, the only work left is a couple of gRPC calls per event.
type Processor struct {
	Monitor  monitor.Monitor
	History  rabbit.PublishConsumer
	Enforcer enforcer_api.EnforcerClient
	Notifier notifier_api.NotifierClient
}

func (p *Processor) Run(stop <-chan struct{}) {
	log.Info().Msgf("Events processor started")
	defer log.Info().Msgf("Events processor stopped")

	events := p.Monitor.Events()
	ctx := context.Background()

	for {
		select {
		case ev := <-events:
			t0 := time.Now()
			err := p.process(ctx, ev)
			delta := time.Since(t0)

			if err != nil {
				log.Error().Err(err).Str("delay", delta.String()).Interface("event", ev).Msgf("Can't process admission event")
			} else {
				log.Debug().Str("delay", delta.String()).Interface("event", ev).Msgf("Admission event processed")
			}
		case <-stop:
			return
		}
	}
}

func (p *Processor) process(ctx context.Context, ev *api.AdmissionEvent) error {
	log.Info().Interface("resource", ev.GetResource()).Int("threats", len(ev.GetThreats())).Msg("Threats detected")

	enforcerResp, err := p.evaluatePolicy(ctx, ev)
	if err != nil {
		return fmt.Errorf("can't evaluate policy: %w", err)
	}

	blockRules, notifyRules := map[string]*enforcer_api.Rule{}, map[string]*enforcer_api.Rule{}
	incidentSeverity := enforcer_model.NoneSeverity // incident's severity to be passed to history API. Depends on policy enforcer's response

	for _, e := range enforcerResp.GetResult().GetEvents() {
		policy := e.GetPolicy()

		// take event's severity into account only if at least one rule matched
		if len(policy.GetBlockBy()) > 0 || len(policy.GetNotifyBy()) > 0 {
			s := enforcer_model.NoneSeverity
			s.Set(e.GetSeverity())

			if s > incidentSeverity {
				incidentSeverity = s
			}
		}

		for _, r := range policy.GetBlockBy() {
			blockRules[r.GetId()] = r
		}

		for _, r := range policy.GetNotifyBy() {
			notifyRules[r.GetId()] = r
		}
	}

	ev.IsIncident = len(blockRules) > 0 || len(notifyRules) > 0
	ev.IncidentSeverity = incidentSeverity.String()
	ev.BlockBy = slices.Collect(maps.Keys(blockRules))
	ev.NotifyBy = slices.Collect(maps.Keys(notifyRules))

	// Kyverno is the only component in the admission path, so a request is blocked when the source
	// which produced the threat is in the enforce mode. Rules of policy-enforcer only decide whether
	// the finding is an incident and who is notified about it.
	block := ev.GetBlocked()

	cfg := p.Monitor.Config()
	if shouldSaveEvent(cfg.Config.HistoryControl, ev) {
		if err := p.History.Publish(ctx, ev); err != nil {
			return fmt.Errorf("can't publish admission event '%s': %w", ev.GetId(), err)
		}
	}

	if len(notifyRules) > 0 {
		if err := p.notify(ctx, ev, slices.Collect(maps.Values(notifyRules)), block); err != nil {
			return fmt.Errorf("can't notify: %w", err)
		}
	}

	return nil
}

func shouldSaveEvent(hc api.Config_ConfigJSON_HistoryControl, ev *api.AdmissionEvent) bool {
	switch hc {
	case api.Config_ConfigJSON_NONE:
		return false
	case api.Config_ConfigJSON_ALL:
		return true
	case api.Config_ConfigJSON_WITH_THREATS:
		return len(ev.GetThreats()) > 0
	default: // normally should not happen
		panic(fmt.Sprintf("invalid historyControl value: %v", hc))
	}
}
