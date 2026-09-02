package config

import (
	"github.com/google/go-cmp/cmp"
	"github.com/runtime-radar/runtime-radar/admission-monitor/pkg/model"
	"google.golang.org/protobuf/testing/protocmp"
)

type Selector struct {
	Policies bool
}

type InitKyverno struct {
	Selector Selector
	Config   *model.Config
}

func Diff(oldCfg, newCfg *model.Config) (sel Selector, changed bool) {
	// Kyverno has no notion of a disabled policy: a disabled source simply has no policy in cluster.
	// This is why, unlike runtime-monitor, there is no separate selector for policy states and any
	// change of the policies map leads to the whole set being re-applied.
	if !cmp.Equal(oldCfg.Config.Policies, newCfg.Config.Policies, protocmp.Transform()) {
		sel.Policies, changed = true, true
	}

	return
}
