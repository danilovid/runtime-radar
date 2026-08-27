package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/runtime-radar/runtime-radar/admission-monitor/api"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/database"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/model"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/monitor"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/monitor/config"
	enforcer_model "github.com/runtime-radar/runtime-radar/policy-enforcer/pkg/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

var allowedSeverities = map[string]bool{
	enforcer_model.LowSeverity.String():      true,
	enforcer_model.MediumSeverity.String():   true,
	enforcer_model.HighSeverity.String():     true,
	enforcer_model.CriticalSeverity.String(): true,
}

type ConfigGeneric struct {
	api.UnimplementedConfigControllerServer

	ConfigRepository database.ConfigRepository
	Monitor          monitor.Monitor
}

func (cg *ConfigGeneric) Add(ctx context.Context, req *api.Config) (*emptypb.Empty, error) {
	if reason, ok := cg.validateConfig(req); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	idStr := req.GetId()
	var id uuid.UUID
	var err error

	if idStr != "" {
		id, err = uuid.Parse(idStr)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "can't parse ID: %v", err)
		}
	}

	cfg := &model.Config{
		model.Base{ID: id},
		(*model.ConfigJSON)(req.GetConfig()),
	}
	if err := cg.ConfigRepository.Add(ctx, cfg); err != nil {
		return nil, status.Errorf(codes.Internal, "can't add config: %v", err)
	}

	// Changes are applied instantly when requested. There is a single admission-monitor instance per
	// cluster, but a background worker still does the same check periodically to survive a restart
	// happening between writing the config and applying it.
	oldCfg := cg.Monitor.Config()

	log.Debug().Interface("old_config", oldCfg).Msgf("Old monitor config")
	log.Debug().Interface("new_config", cfg).Msgf("New monitor config")

	sel, changed := config.Diff(oldCfg, cfg)
	if changed {
		log.Info().
			Interface("config", cfg).
			Interface("selector", sel).
			Msgf("Monitor config changed, re-initializing")

		cg.Monitor.Reinit(sel, cfg)
	} else {
		log.Debug().Msgf("Monitor config didn't change")
	}

	resp := &emptypb.Empty{}

	return resp, nil
}

func (cg *ConfigGeneric) Read(ctx context.Context, _ *emptypb.Empty) (*api.Config, error) {
	cfg, err := cg.ConfigRepository.GetLast(ctx, false)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, status.Errorf(codes.NotFound, "config not found")
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "can't read config: %v", err)
	}

	resp := &api.Config{
		Id:     cfg.ID.String(),
		Config: (*api.Config_ConfigJSON)(cfg.Config),
	}

	return resp, nil
}

func (cg *ConfigGeneric) validateConfig(req *api.Config) (string, bool) {
	if req.Config == nil {
		return "no config", false
	}

	if req.Config.GetVersion() == "" {
		return "empty or missing config version", false
	} else if ver := req.Config.GetVersion(); ver != string(model.ConfigVersion) {
		return fmt.Sprintf("config version mismatch: expected %s, got %s", model.ConfigVersion, ver), false
	}

	if len(req.Config.GetPolicies()) == 0 {
		return "no policies", false
	}

	names := map[string]string{}

	for key, p := range req.Config.GetPolicies() {
		if reason, ok := cg.validatePolicy(key, p, names); !ok {
			return fmt.Sprintf("policy '%s' is invalid: %s", key, reason), false
		}
	}

	return "", true
}

// validatePolicy checks that the manifest can be applied as is. Kyverno validates the policy itself
// when it is created, we only make sure that the parts admission-monitor relies on are in place:
// a supported kind, a name unique within the config and a known severity.
func (cg *ConfigGeneric) validatePolicy(key string, p *api.KyvernoPolicy, names map[string]string) (string, bool) {
	if p.GetName() == "" {
		return "empty or missing name", false
	}

	if !allowedSeverities[p.GetSeverity()] {
		return fmt.Sprintf("unknown severity '%s'", p.GetSeverity()), false
	}

	obj, _, err := monitor.DecodePolicy(key, p)
	if err != nil {
		return err.Error(), false
	}

	// Policy names are cluster-wide, and results in a policy report are attributed to a policy by name,
	// so two sources creating a policy with the same name would be indistinguishable.
	if other, taken := names[obj.GetName()]; taken {
		return fmt.Sprintf("manifest name '%s' is already used by policy '%s'", obj.GetName(), other), false
	}
	names[obj.GetName()] = key

	return "", true
}
