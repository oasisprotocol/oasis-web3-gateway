package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"

	beacon "github.com/oasisprotocol/oasis-core/go/beacon/api"
	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/oasisprotocol/oasis-core/go/common/node"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	"github.com/oasisprotocol/oasis-core/go/common/version"
	consensusGenesis "github.com/oasisprotocol/oasis-core/go/consensus/genesis"
	cmdCommon "github.com/oasisprotocol/oasis-core/go/oasis-node/cmd/common"
	"github.com/oasisprotocol/oasis-core/go/oasis-test-runner/oasis"
	registry "github.com/oasisprotocol/oasis-core/go/registry/api"
	runtimeRegistry "github.com/oasisprotocol/oasis-core/go/runtime/registry"
	"github.com/oasisprotocol/oasis-core/go/staking/api"
	"github.com/oasisprotocol/oasis-core/go/worker/common/p2p"

	"github.com/oasisprotocol/oasis-web3-gateway/benchmarks/util/keys"
)

const (
	cfgNodeBinary     = "node.binary"
	cfgRuntimeLoader  = "runtime.loader"
	cfgRuntimeBinary  = "runtime.binary"
	cfgRuntimeVersion = "runtime.version"
)

var fixtureFlags = flag.NewFlagSet("", flag.ContinueOnError)

func fixture() *oasis.NetworkFixture {
	var runtimeID common.Namespace
	if err := runtimeID.UnmarshalHex(flagRuntimeID); err != nil {
		cmdCommon.EarlyLogAndExit(err)
	}
	computeExtraArgs := []oasis.Argument{
		{
			Name:   p2p.CfgP2PPeerOutboundQueueSize,
			Values: []string{"100_000"},
		},
		{
			Name:   p2p.CfgP2PValidateQueueSize,
			Values: []string{"100_000"},
		},
		{
			Name:   p2p.CfgP2PValidateConcurrency,
			Values: []string{"100_000"},
		},
		{
			Name:   p2p.CfgP2PValidateThrottle,
			Values: []string{"100_000"},
		},
	}

	version, err := version.FromString(viper.GetString(cfgRuntimeVersion))
	if err != nil {
		cmdCommon.EarlyLogAndExit(err)
	}
	fixture := &oasis.NetworkFixture{
		Network: oasis.NetworkCfg{
			NodeBinary:             viper.GetString(cfgNodeBinary),
			RuntimeSGXLoaderBinary: viper.GetString(cfgRuntimeLoader),
			Consensus: consensusGenesis.Genesis{
				Parameters: consensusGenesis.Parameters{
					TimeoutCommit: 1 * time.Second,
				},
			},
			Beacon: beacon.ConsensusParameters{
				Backend: beacon.BackendInsecure,
				InsecureParameters: &beacon.InsecureParameters{
					Interval: 30,
				},
			},
			HaltEpoch:    10000,
			FundEntities: true,
		},
		Entities: []oasis.EntityCfg{
			{IsDebugTestEntity: true},
			{},
		},
		Validators: []oasis.ValidatorFixture{
			{Entity: 1},
		},
		Clients: []oasis.ClientFixture{
			{
				RuntimeConfig: map[int]map[string]interface{}{
					0: {
						"allow_expensive_queries": true,
					},
				},
				Runtimes:           []int{0},
				RuntimeProvisioner: runtimeRegistry.RuntimeProvisionerUnconfined,
			},
		},
		ComputeWorkers: []oasis.ComputeWorkerFixture{
			{NodeFixture: oasis.NodeFixture{Name: "compute-0", ExtraArgs: computeExtraArgs}, Entity: 1, Runtimes: []int{0}, RuntimeProvisioner: runtimeRegistry.RuntimeProvisionerUnconfined},
			{NodeFixture: oasis.NodeFixture{Name: "compute-1", ExtraArgs: computeExtraArgs}, Entity: 1, Runtimes: []int{0}, RuntimeProvisioner: runtimeRegistry.RuntimeProvisionerUnconfined},
			{NodeFixture: oasis.NodeFixture{Name: "compute-2", ExtraArgs: computeExtraArgs}, Entity: 1, Runtimes: []int{0}, RuntimeProvisioner: runtimeRegistry.RuntimeProvisionerUnconfined},
		},
		Seeds: []oasis.SeedFixture{{}},
		Runtimes: []oasis.RuntimeFixture{
			{
				ID:         runtimeID,
				Kind:       registry.KindCompute,
				Entity:     0,
				Keymanager: -1,
				Executor: registry.ExecutorParameters{
					GroupSize:       2,
					GroupBackupSize: 1,
					RoundTimeout:    5,
					MaxMessages:     10_000, // TODO: no way to update max_runtime_messages via fixture.
				},
				TxnScheduler: registry.TxnSchedulerParameters{
					MaxBatchSize:      10_000,
					MaxBatchSizeBytes: 10 * 16 * 1024 * 1024, // 160 MiB
					BatchFlushTimeout: 1 * time.Second,
					ProposerTimeout:   5,
				},
				AdmissionPolicy: registry.RuntimeAdmissionPolicy{
					AnyNode: &registry.AnyNodeRuntimeAdmissionPolicy{},
				},
				GenesisRound:    0,
				GovernanceModel: registry.GovernanceEntity,
				Deployments: []oasis.DeploymentCfg{
					{
						Version: version,
						Binaries: map[node.TEEHardware]string{
							node.TEEHardwareInvalid: viper.GetString(cfgRuntimeBinary),
						},
					},
				},
			},
		},
	}

	return fixture
}

func stakingGenesis() *api.Genesis {
	logger := logging.GetLogger("fixture")
	var runtimeID common.Namespace
	if err := runtimeID.UnmarshalHex(flagRuntimeID); err != nil {
		logger.Error("invalid runtime ID",
			"runtime_id", flagRuntimeID,
			"err", err,
		)
		os.Exit(1)
	}

	genesis := api.Genesis{
		Parameters: api.ConsensusParameters{
			MaxAllowances:       100_000_000,
			AllowEscrowMessages: true,
		},
		TokenSymbol: "BENCH",
		Ledger:      make(map[api.Address]*api.Account),
	}

	// Create a genesis with 100_000 accounts.
	for _, acc := range keys.ConsensusBenchmarkKeys(100_000) {
		genesis.Ledger[api.Address(acc.Address)] = &api.Account{
			General: api.GeneralAccount{
				Balance: *quantity.NewFromUint64(100_000_000_000),
				Allowances: map[api.Address]quantity.Quantity{
					api.NewRuntimeAddress(runtimeID): *quantity.NewFromUint64(100_000_000_000),
				},
			},
		}
		genesis.TotalSupply.Add(quantity.NewFromUint64(100_000_000_000))
	}

	return &genesis
}

func genesisMain(cmd *cobra.Command, args []string) {
	f := fixture()
	f.Network.StakingGenesis = stakingGenesis()
	data, err := json.MarshalIndent(f, "", "    ")
	if err != nil {
		cmdCommon.EarlyLogAndExit(err)
	}

	fmt.Printf("%s", data)
}

func fixtureInit(cmd *cobra.Command) {
	fixtureFlags.StringVar(&flagRuntimeID, cfgRuntimeID, "", "Benchmarked runtime ID (HEX)")
	fixtureFlags.String(cfgNodeBinary, "oasis-node", "path to the oasis-node binary")
	fixtureFlags.String(cfgRuntimeLoader, "oasis-core-runtime-loader", "path to the runtime loader")
	fixtureFlags.String(cfgRuntimeBinary, "runtime-binary", "path to the runtime binary")
	fixtureFlags.String(cfgRuntimeVersion, "runtime-version", "runtime version")

	_ = viper.BindPFlags(fixtureFlags)

	fixtureCmd.Flags().AddFlagSet(fixtureFlags)

	cmd.AddCommand(fixtureCmd)
}
