package params

// DecoupledConfig retrieves the decoupled config: mainnet with 8-slot epochs.
//
// It exists so tests built with -tags=decoupled have a runtime config matching
// the compiled-in field parameters. It is deliberately NOT registered in
// init.go's defaults: it reuses mainnet's fork-version schedule, which
// configset rejects as a duplicate. A decoupled node gets its runtime config
// from a devnet yaml instead.
func DecoupledConfig() *BeaconChainConfig {
	decoupledConfig := mainnetBeaconConfig.Copy()
	decoupledConfig.SlotsPerEpoch = 8
	decoupledConfig.SqrRootSlotsPerEpoch = 2
	decoupledConfig.ConfigName = DecoupledName
	decoupledConfig.PresetBase = DecoupledName

	decoupledConfig.InitializeForkSchedule()

	return decoupledConfig
}
