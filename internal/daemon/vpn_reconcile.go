package daemon

import (
	"errors"
	"time"

	"github.com/alebeck/boring/internal/log"
	"github.com/alebeck/boring/internal/tunnel"
	"github.com/alebeck/boring/internal/vpn"
)

type vpnActionPlan struct {
	open  []*tunnel.Desc
	close []string
}

func (d *daemon) startVPNReconciler() {
	if d.conf == nil || !d.conf.VPN.Enabled() || !hasVPNAutomation(d.conf.Tunnels) {
		return
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.runVPNReconciler()
	}()
}

func hasVPNAutomation(tunnels []tunnel.Desc) bool {
	for _, desc := range tunnels {
		if desc.AutoOpenWhenVPN || desc.AutoCloseWhenVPNLost {
			return true
		}
	}
	return false
}

func (d *daemon) runVPNReconciler() {
	pollInterval := time.Duration(d.conf.VPN.PollInterval) * time.Second
	stableFor := time.Duration(d.conf.VPN.StableFor) * time.Second
	state := vpn.NewStableState(stableFor)

	log.Infof(
		"Starting VPN reconciler with poll interval %s and stable window %s",
		pollInterval,
		stableFor,
	)

	d.observeAndReconcileVPN(state, time.Now())

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case now := <-ticker.C:
			d.observeAndReconcileVPN(state, now)
		}
	}
}

func (d *daemon) observeAndReconcileVPN(state *vpn.StableState, now time.Time) {
	onVPN, err := vpn.OnVPN(d.conf.VPN.ParsedCIDRs)
	if err != nil {
		log.Errorf("Could not check VPN state: %v", err)
		return
	}

	stable, changed, ok := state.Observe(onVPN, now)
	if !ok {
		log.Debugf("Observed VPN state %v; waiting for stable window", onVPN)
		return
	}
	if changed {
		log.Infof("VPN state is now %v", stable)
	}
	d.reconcileVPNState(stable)
}

func (d *daemon) reconcileVPNState(onVPN bool) {
	d.mutex.RLock()
	running := make(map[string]bool, len(d.tunnels))
	for name := range d.tunnels {
		running[name] = true
	}
	opening := make(map[string]bool, len(d.opening))
	for name := range d.opening {
		opening[name] = true
	}
	d.mutex.RUnlock()

	plan := planVPNActions(d.conf.Tunnels, running, opening, onVPN)

	for _, desc := range plan.open {
		if err := d.startTunnel(desc); err != nil && !errors.Is(err, AlreadyRunning) {
			log.Errorf("%v: could not auto-open for VPN state: %v", desc.Name, err)
		}
	}

	for _, name := range plan.close {
		if err := d.stopTunnel(name); err != nil {
			log.Errorf("%v: could not auto-close for VPN state: %v", name, err)
		}
	}
}

func planVPNActions(
	tunnels []tunnel.Desc,
	running map[string]bool,
	opening map[string]bool,
	onVPN bool,
) vpnActionPlan {
	var plan vpnActionPlan

	for i := range tunnels {
		desc := &tunnels[i]
		eligible := !desc.VPNRequired || onVPN
		isRunning := running[desc.Name]
		isOpening := opening[desc.Name]

		if eligible && desc.AutoOpenWhenVPN && !isRunning && !isOpening {
			plan.open = append(plan.open, desc)
		}
		if !eligible && desc.AutoCloseWhenVPNLost && isRunning {
			plan.close = append(plan.close, desc.Name)
		}
	}

	return plan
}
