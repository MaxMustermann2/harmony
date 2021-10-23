package core

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	bls_core "github.com/harmony-one/bls/ffi/go/bls"
	"github.com/harmony-one/harmony/block"
	blockfactory "github.com/harmony-one/harmony/block/factory"
	"github.com/harmony-one/harmony/common/denominations"
	"github.com/harmony-one/harmony/core/state"
	"github.com/harmony-one/harmony/core/types"
	"github.com/harmony-one/harmony/core/vm"
	"github.com/harmony-one/harmony/crypto/bls"
	"github.com/harmony-one/harmony/crypto/hash"
	chain2 "github.com/harmony-one/harmony/internal/chain"
	"github.com/harmony-one/harmony/internal/params"
	"github.com/harmony-one/harmony/numeric"
	staking "github.com/harmony-one/harmony/staking/types"
)

func getTestEnvironment(testBankKey ecdsa.PrivateKey) (*BlockChain, *state.DB, *block.Header, ethdb.Database) {
	// initialize
	var (
		testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
		testBankFunds   = new(big.Int).Mul(big.NewInt(denominations.One), big.NewInt(40000))
		chainConfig     = params.TestChainConfig
		blockFactory    = blockfactory.ForTest
		database        = rawdb.NewMemoryDatabase()
		gspec           = Genesis{
			Config:  chainConfig,
			Factory: blockFactory,
			Alloc:   GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
			ShardID: 10,
		}
		engine = chain2.NewEngine()
	)
	genesis := gspec.MustCommit(database)
	// _ = genesis

	// fake blockchain
	chain, _ := NewBlockChain(database, nil, gspec.Config, engine, vm.Config{}, nil)
	// fake statedb => not set up correctly
	// db, _ := state.New(common.Hash{}, state.NewDatabase(database))
	db, _ := chain.StateAt(genesis.Root())

	// make a fake block header (use epoch 1 so that locked tokens can be tested)
	header := blockFactory.NewHeader(common.Big0)

	return chain, db, header, database
}

func TestEVMStaking(t *testing.T) {
	key, _ := crypto.GenerateKey()
	chain, db, header, database := getTestEnvironment(*key)
	batch := database.NewBatch()

	// fake transaction
	tx := types.NewTransaction(1, common.BytesToAddress([]byte{0x11}), 0, big.NewInt(111), 1111, big.NewInt(11111), []byte{0x11, 0x11, 0x11})
	// transaction as message (chainId = 2)
	msg, _ := tx.AsMessage(types.NewEIP155Signer(common.Big2))
	// context
	ctx := NewEVMContext(msg, header, chain, nil /* coinbase */)

	// createValidator test
	createValidator := sampleCreateValidator(*key)
	err := ctx.CreateValidator(db, &createValidator)
	if err != nil {
		t.Errorf("Got error %v in CreateValidator", err)
	}
	// write it to snapshot so that we can use it in edit
	wrapper, err := db.ValidatorWrapper(createValidator.ValidatorAddress)
	err = chain.WriteValidatorSnapshot(batch, &staking.ValidatorSnapshot{wrapper, header.Epoch()})
	// also write the delegation so we can use it in CollectRewards
	selfIndex := staking.DelegationIndex{
		createValidator.ValidatorAddress,
		uint64(0),
		common.Big0, // block number at which delegation starts
	}
	err = chain.writeDelegationsByDelegator(batch, createValidator.ValidatorAddress, []staking.DelegationIndex{selfIndex})

	// editValidator test
	editValidator := sampleEditValidator(*key)
	editValidator.SlotKeyToRemove = &createValidator.SlotPubKeys[0]
	err = ctx.EditValidator(db, &editValidator)
	if err != nil {
		t.Errorf("Got error %v in EditValidator", err)
	}

	// delegate test
	delegate := sampleDelegate(*key)
	// add undelegations in epoch0
	wrapper.Delegations[0].Undelegations = []staking.Undelegation{
		staking.Undelegation{
			new(big.Int).Mul(big.NewInt(denominations.One),
				big.NewInt(10000)),
			common.Big0,
		},
	}
	// redelegate using epoch1, so that we can cover the locked tokens use case as well
	ctx2 := NewEVMContext(msg, blockfactory.ForTest.NewHeader(common.Big1), chain, nil)
	err = db.UpdateValidatorWrapper(wrapper.Address, wrapper)
	err = ctx2.Delegate(db, &delegate)
	if err != nil {
		t.Errorf("Got error %v in Delegate", err)
	}

	// undelegate test
	undelegate := sampleUndelegate(*key)
	err = ctx.Undelegate(db, &undelegate)
	if err != nil {
		t.Errorf("Got error %v in Undelegate", err)
	}

	// collectRewards test
	collectRewards := sampleCollectRewards(*key)
	// add block rewards to make sure there are some to collect
	wrapper.Delegations[0].Undelegations = []staking.Undelegation{}
	wrapper.Delegations[0].Reward = common.Big257
	db.UpdateValidatorWrapper(wrapper.Address, wrapper)
	err = ctx.CollectRewards(db, &collectRewards)
	if err != nil {
		t.Errorf("Got error %v in CollectRewards", err)
	}
}

func generateBLSKeyAndSig() (bls.SerializedPublicKey, bls.SerializedSignature) {
	p := &bls_core.PublicKey{}
	p.DeserializeHexStr(testBLSPubKey)
	pub := bls.SerializedPublicKey{}
	pub.FromLibBLSPublicKey(p)
	messageBytes := []byte(staking.BLSVerificationStr)
	privateKey := &bls_core.SecretKey{}
	privateKey.DeserializeHexStr(testBLSPrvKey)
	msgHash := hash.Keccak256(messageBytes)
	signature := privateKey.SignHash(msgHash[:])
	var sig bls.SerializedSignature
	copy(sig[:], signature.Serialize())
	return pub, sig
}

func sampleCreateValidator(key ecdsa.PrivateKey) staking.CreateValidator {
	pub, sig := generateBLSKeyAndSig()

	ra, _ := numeric.NewDecFromStr("0.7")
	maxRate, _ := numeric.NewDecFromStr("1")
	maxChangeRate, _ := numeric.NewDecFromStr("0.5")
	return staking.CreateValidator{
		Description: staking.Description{
			Name:            "SuperHero",
			Identity:        "YouWouldNotKnow",
			Website:         "Secret Website",
			SecurityContact: "LicenseToKill",
			Details:         "blah blah blah",
		},
		CommissionRates: staking.CommissionRates{
			Rate:          ra,
			MaxRate:       maxRate,
			MaxChangeRate: maxChangeRate,
		},
		MinSelfDelegation:  new(big.Int).Mul(big.NewInt(denominations.One), big.NewInt(10000)),
		MaxTotalDelegation: new(big.Int).Mul(big.NewInt(denominations.One), big.NewInt(20000)),
		ValidatorAddress:   crypto.PubkeyToAddress(key.PublicKey),
		SlotPubKeys:        []bls.SerializedPublicKey{pub},
		SlotKeySigs:        []bls.SerializedSignature{sig},
		Amount:             new(big.Int).Mul(big.NewInt(denominations.One), big.NewInt(15000)),
	}
}

func sampleEditValidator(key ecdsa.PrivateKey) staking.EditValidator {
	// generate new key and sig
	slotKeyToAdd, slotKeyToAddSig := generateBLSKeyAndSig()

	// rate
	ra, _ := numeric.NewDecFromStr("0.8")

	return staking.EditValidator{
		Description: staking.Description{
			Name:            "Alice",
			Identity:        "alice",
			Website:         "alice.harmony.one",
			SecurityContact: "Bob",
			Details:         "Don't mess with me!!!",
		},
		CommissionRate:     &ra,
		MinSelfDelegation:  new(big.Int).Mul(big.NewInt(denominations.One), big.NewInt(10000)),
		MaxTotalDelegation: new(big.Int).Mul(big.NewInt(denominations.One), big.NewInt(20000)),
		SlotKeyToRemove:    nil,
		SlotKeyToAdd:       &slotKeyToAdd,
		SlotKeyToAddSig:    &slotKeyToAddSig,
		ValidatorAddress:   crypto.PubkeyToAddress(key.PublicKey),
	}
}

func sampleDelegate(key ecdsa.PrivateKey) staking.Delegate {
	address := crypto.PubkeyToAddress(key.PublicKey)
	return staking.Delegate{
		DelegatorAddress: address,
		ValidatorAddress: address,
		// additional delegation of 1000 ONE
		Amount: new(big.Int).Mul(big.NewInt(denominations.One), big.NewInt(1000)),
	}
}

func sampleUndelegate(key ecdsa.PrivateKey) staking.Undelegate {
	address := crypto.PubkeyToAddress(key.PublicKey)
	return staking.Undelegate{
		DelegatorAddress: address,
		ValidatorAddress: address,
		// undelegate the delegation of 1000 ONE
		Amount: new(big.Int).Mul(big.NewInt(denominations.One), big.NewInt(1000)),
	}
}

func sampleCollectRewards(key ecdsa.PrivateKey) staking.CollectRewards {
	address := crypto.PubkeyToAddress(key.PublicKey)
	return staking.CollectRewards{
		DelegatorAddress: address,
	}
}

func TestWriteCapablePrecompilesIntegration(t *testing.T) {
	key, _ := crypto.GenerateKey()
	chain, db, header, _ := getTestEnvironment(*key)
	// gp := new(GasPool).AddGas(math.MaxUint64)
	tx := types.NewTransaction(1, common.BytesToAddress([]byte{0x11}), 0, big.NewInt(111), 1111, big.NewInt(11111), []byte{0x11, 0x11, 0x11})
	msg, _ := tx.AsMessage(types.NewEIP155Signer(common.Big2))
	ctx := NewEVMContext(msg, header, chain, nil /* coinbase */)
	evm := vm.NewEVM(ctx, db, params.TestChainConfig, vm.Config{})
	// interpreter := vm.NewEVMInterpreter(evm, vm.Config{})
	address := common.BytesToAddress([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 252})
	// caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int)
	_, _, err := evm.Call(vm.AccountRef(common.Address{}), address,
		[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 252, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		math.MaxUint64, new(big.Int))
	expectedError := errors.New("abi: cannot marshal in to go type: length insufficient 32 require 64")
	if err != nil {
		if err.Error() != expectedError.Error() {
			t.Errorf(fmt.Sprintf("Got error %v in evm.Call but expected %v", err, expectedError))
		}
	}

	// now add a validator, and send its address as caller
	createValidator := sampleCreateValidator(*key)
	err = ctx.CreateValidator(db, &createValidator)
	_, _, err = evm.Call(vm.AccountRef(common.Address{}),
		createValidator.ValidatorAddress,
		[]byte{},
		math.MaxUint64, new(big.Int))
	if err != nil {
		t.Errorf(fmt.Sprintf("Got error %v in evm.Call", err))
	}

	// now without staking precompile
	cfg := params.TestChainConfig
	cfg.StakingPrecompileEpoch = big.NewInt(10000000)
	evm = vm.NewEVM(ctx, db, cfg, vm.Config{})
	_, _, err = evm.Call(vm.AccountRef(common.Address{}),
		createValidator.ValidatorAddress,
		[]byte{},
		math.MaxUint64, new(big.Int))
	if err != nil {
		t.Errorf(fmt.Sprintf("Got error %v in evm.Call", err))
	}
}
