package evm_test

import (
	"math"
	"testing"
	"time"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/suite"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/ibc-go/v8/testing/simapp"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/hetu-project/hetu/v1/app"
	ante "github.com/hetu-project/hetu/v1/app/ante"
	"github.com/hetu-project/hetu/v1/encoding"
	"github.com/hetu-project/hetu/v1/ethereum/eip712"
	"github.com/hetu-project/hetu/v1/utils"
	evmtypes "github.com/hetu-project/hetu/v1/x/evm/types"
	feemarkettypes "github.com/hetu-project/hetu/v1/x/feemarket/types"
)

type AnteTestSuite struct {
	suite.Suite

	ctx                      sdk.Context
	app                      *app.Evmos
	clientCtx                client.Context
	anteHandler              sdk.AnteHandler
	ethSigner                types.Signer
	enableFeemarket          bool
	enableLondonHF           bool
	evmParamsOption          func(*evmtypes.Params)
	useLegacyEIP712Extension bool
	useLegacyEIP712TypedData bool
}

const TestGasLimit uint64 = 100000

func (suite *AnteTestSuite) SetupTest() {
	checkTx := false

	suite.app = app.EthSetup(checkTx, func(app *app.Evmos, genesis simapp.GenesisState) simapp.GenesisState {
		if suite.enableFeemarket {
			// setup feemarketGenesis params
			feemarketGenesis := feemarkettypes.DefaultGenesisState()
			feemarketGenesis.Params.EnableHeight = 1
			feemarketGenesis.Params.NoBaseFee = false
			// Verify feeMarket genesis
			err := feemarketGenesis.Validate()
			suite.Require().NoError(err)
			genesis[feemarkettypes.ModuleName] = app.AppCodec().MustMarshalJSON(feemarketGenesis)
		}
		evmGenesis := evmtypes.DefaultGenesisState()
		evmGenesis.Params.AllowUnprotectedTxs = false
		if !suite.enableLondonHF {
			maxInt := sdkmath.NewInt(math.MaxInt64)
			evmGenesis.Params.ChainConfig.LondonBlock = &maxInt
			evmGenesis.Params.ChainConfig.ArrowGlacierBlock = &maxInt
			evmGenesis.Params.ChainConfig.GrayGlacierBlock = &maxInt
			evmGenesis.Params.ChainConfig.MergeNetsplitBlock = &maxInt
			evmGenesis.Params.ChainConfig.ShanghaiBlock = &maxInt
			evmGenesis.Params.ChainConfig.CancunBlock = &maxInt
		}
		if suite.evmParamsOption != nil {
			suite.evmParamsOption(&evmGenesis.Params)
		}
		genesis[evmtypes.ModuleName] = app.AppCodec().MustMarshalJSON(evmGenesis)
		return genesis
	})

	header := tmproto.Header{Height: 2, ChainID: utils.TestnetChainID + "-1", Time: time.Now().UTC()}
	suite.ctx = suite.app.BaseApp.NewUncachedContext(checkTx, header)
	suite.ctx = suite.ctx.WithMinGasPrices(sdk.NewDecCoins(sdk.NewDecCoin(evmtypes.DefaultEVMDenom, sdkmath.OneInt())))
	suite.ctx = suite.ctx.WithBlockGasMeter(storetypes.NewGasMeter(1000000000000000000))
	suite.app.EvmKeeper.WithChainID(suite.ctx)

	// set staking denomination to Evmos denom
	params, err := suite.app.StakingKeeper.GetParams(suite.ctx)
	suite.Require().NoError(err)
	params.BondDenom = utils.BaseDenom
	suite.app.StakingKeeper.SetParams(suite.ctx, params)

	infCtx := suite.ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	suite.app.AccountKeeper.Params.Set(infCtx, authtypes.DefaultParams())

	encodingConfig := encoding.MakeConfig()
	// We're using TestMsg amino encoding in some tests, so register it here.
	encodingConfig.Amino.RegisterConcrete(&testdata.TestMsg{}, "testdata.TestMsg", nil)
	eip712.SetEncodingConfig(encodingConfig)

	suite.clientCtx = client.Context{}.WithTxConfig(encodingConfig.TxConfig)

	suite.Require().NotNil(suite.app.AppCodec())

	anteHandler := ante.NewAnteHandler(ante.HandlerOptions{
		Cdc:                suite.app.AppCodec(),
		AccountKeeper:      suite.app.AccountKeeper,
		BankKeeper:         suite.app.BankKeeper,
		DistributionKeeper: suite.app.DistrKeeper,
		EvmKeeper:          suite.app.EvmKeeper,
		FeegrantKeeper:     suite.app.FeeGrantKeeper,
		IBCKeeper:          suite.app.IBCKeeper,
		StakingKeeper:      suite.app.StakingKeeper,
		FeeMarketKeeper:    suite.app.FeeMarketKeeper,
		SignModeHandler:    encodingConfig.TxConfig.SignModeHandler(),
		SigGasConsumer:     ante.SigVerificationGasConsumer,
	})

	suite.anteHandler = anteHandler
	suite.ethSigner = types.LatestSignerForChainID(suite.app.EvmKeeper.ChainID())
}

func TestAnteTestSuite(t *testing.T) {
	suite.Run(t, &AnteTestSuite{
		enableLondonHF: true,
	})

	// Re-run the tests with EIP-712 Legacy encodings to ensure backwards compatibility.
	// LegacyEIP712Extension should not be run with current TypedData encodings, since they are not compatible.
	suite.Run(t, &AnteTestSuite{
		enableLondonHF:           true,
		useLegacyEIP712Extension: true,
		useLegacyEIP712TypedData: true,
	})

	suite.Run(t, &AnteTestSuite{
		enableLondonHF:           true,
		useLegacyEIP712Extension: false,
		useLegacyEIP712TypedData: true,
	})
}
