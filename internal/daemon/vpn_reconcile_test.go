package daemon

import (
	"reflect"
	"testing"

	"github.com/alebeck/boring/internal/tunnel"
)

var vpnPlanTestTunnels = []tunnel.Desc{
	{
		Name:                 "work-api",
		VPNRequired:          true,
		AutoOpenWhenVPN:      true,
		AutoCloseWhenVPNLost: true,
	},
	{
		Name:                 "manual-close",
		VPNRequired:          true,
		AutoCloseWhenVPNLost: true,
	},
	{
		Name:            "manual-keep",
		VPNRequired:     true,
		AutoOpenWhenVPN: true,
	},
	{
		Name:            "always-on",
		AutoOpenWhenVPN: true,
	},
}

func TestPlanVPNActionsOpen(t *testing.T) {
	plan := planVPNActions(
		vpnPlanTestTunnels,
		map[string]bool{},
		map[string]bool{},
		true,
	)
	assertVPNPlan(t, plan,
		[]string{"work-api", "manual-keep", "always-on"},
		nil,
	)
}

func TestPlanVPNActionsNoDuplicateOpen(t *testing.T) {
	plan := planVPNActions(
		vpnPlanTestTunnels,
		map[string]bool{"work-api": true},
		map[string]bool{"manual-keep": true},
		true,
	)
	assertVPNPlan(t, plan, []string{"always-on"}, nil)
}

func TestPlanVPNActionsClose(t *testing.T) {
	plan := planVPNActions(
		vpnPlanTestTunnels,
		map[string]bool{
			"work-api":     true,
			"manual-close": true,
			"manual-keep":  true,
			"always-on":    true,
		},
		map[string]bool{},
		false,
	)
	assertVPNPlan(t, plan, nil, []string{"work-api", "manual-close"})
}

func TestPlanVPNActionsOpenWithoutVPNRequired(t *testing.T) {
	plan := planVPNActions(
		vpnPlanTestTunnels,
		map[string]bool{},
		map[string]bool{},
		false,
	)
	assertVPNPlan(t, plan, []string{"always-on"}, nil)
}

func assertVPNPlan(t *testing.T, plan vpnActionPlan, wantOpen, wantClose []string) {
	gotOpen := tunnelNames(plan.open)
	if !reflect.DeepEqual(gotOpen, wantOpen) {
		t.Fatalf("open = %v, want %v", gotOpen, wantOpen)
	}
	if !reflect.DeepEqual(plan.close, wantClose) {
		t.Fatalf("close = %v, want %v", plan.close, wantClose)
	}
}

func tunnelNames(tunnels []*tunnel.Desc) []string {
	var names []string
	for _, desc := range tunnels {
		names = append(names, desc.Name)
	}
	return names
}
