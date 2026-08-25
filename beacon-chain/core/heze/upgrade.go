// Package heze implements the Heze fork upgrade.
package heze

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// UpgradeToHeze performs the Heze fork upgrade in place.
//
// Heze is a consensus-only fork: it rotates signing domains and gossip
// digests but changes no state or block containers. The upgrade therefore
// only bumps the state's fork field; the state keeps its pre-Heze shape and
// version enum (see params.BeaconChainConfig.HezeShape). Without this bump,
// validators sign with the schedule's Heze domain while nodes verify against
// the state's pre-Heze fork, and every post-fork signature fails.
func UpgradeToHeze(beaconState state.BeaconState) (state.BeaconState, error) {
	if err := beaconState.SetFork(&ethpb.Fork{
		PreviousVersion: beaconState.Fork().CurrentVersion,
		CurrentVersion:  params.BeaconConfig().HezeForkVersion,
		Epoch:           time.CurrentEpoch(beaconState),
	}); err != nil {
		return nil, errors.Wrap(err, "could not set heze fork")
	}
	return beaconState, nil
}
