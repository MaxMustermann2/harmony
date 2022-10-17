package router

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/harmony-one/harmony/core/types"
	"github.com/harmony-one/harmony/core/vm"
	"github.com/harmony-one/harmony/internal/params"
)

// var RouterAddress common.Address = common.BytesToAddress([]byte{248})

// The Router class is the implementation of the
// cross shard smart contract precompile
type Router struct {
	// temporarily stored to avoid re-parsing
	message interface{}
	// also store the type asserted versions to cut down the costs
	messageSend      *routerMessageSend
	messageRetrySend *routerMessageRetrySend
}

func (router *Router) RequiredGas(
	evm *vm.EVM,
	contract *vm.Contract,
	input []byte,
) (uint64, error) {
	var err error
	router.message, err = parseMethod(input)
	if err != nil {
		// for invalid calls to the precompile
		// charge base gas + data cost (again) to avoid spamming
		var payload []byte
		// re-encoding is required to remove cost of calling the
		// precompile and only the data cost to remain
		payload, err = rlp.EncodeToBytes(router.message)
		if err != nil {
			// TODO for testing - check encoding works
			panic(fmt.Sprintf("could not encode message to bytes %s", err))
		}
		if gas, err := vm.IntrinsicGas(
			payload,
			false,
			evm.ChainConfig().IsS3(evm.EpochNumber),
			evm.ChainConfig().IsIstanbul(evm.EpochNumber),
			false,
		); err != nil {
			// only error returned by IntrinsicGas is ErrOutOfGas
			return 0, err
		} else {
			return gas, nil
		}
	} else {
		if messageSend, ok := router.message.(*routerMessageSend); ok {
			router.messageSend = messageSend
			return params.SstoreSetGas * uint64(len(messageSend.payload)), nil
		} else if messageRetrySend, ok := router.message.(*routerMessageRetrySend); ok {
			router.messageRetrySend = messageRetrySend
			return 3 * params.SstoreSetGas, nil
		} else {
			return 0, errors.New("invalid parsed object")
		}
	}
}

func (router *Router) RunWriteCapable(
	evm *vm.EVM,
	contract *vm.Contract, // the precompile, so its caller is the smart contract or EOA
	input []byte,
) ([]byte, error) {
	// if router.message == nil {
	// 	return nil, errors.New("cannot call Run before CalculateGas")
	// }
	// if router.messageSend != nil && router.messageRetrySend != nil {
	// 	return nil, errors.New("cannot send message and retry message together")
	// }
	if router.messageSend != nil {
		cxReceipt := &types.CXReceipt{
			From:      contract.Caller(),
			To:        &router.messageSend.to,
			ShardID:   evm.ShardID,
			ToShardID: router.messageSend.toShard,
			// TODO check Amount calculation (GasPrice, GasLimit and GasBudget)
			Amount:        contract.Value(),
			Nonce:         evm.StateDB.GetCrossShardNonce(contract.Caller()),
			Payload:       router.messageSend.payload,
			GasPrice:      router.messageSend.gasPrice,
			GasBudget:     router.messageSend.gasBudget,
			GasLeftoverTo: router.messageSend.gasLeftoverTo,
			GasLimit:      router.messageSend.gasLimit,
		}
		// store the message
		outgoingMessage := NewOutgoingMessage(cxReceipt)
		outgoingMessage.StoreMessage(evm.StateDB)
		// CXReceipts are stored per block and not per transaction
		if err := evm.EmitCXReceipt(cxReceipt); err != nil {
			return nil, err
		}
		return outgoingMessage.msgAddress[:], nil
	} else {
		// load the message
		outgoingMessage, err := LoadMessage(router.messageRetrySend.msgAddr, evm.StateDB)
		if err != nil {
			return nil, err
		}
		// these parameters do not feature in the address calculation
		outgoingMessage.cxReceipt.GasPrice = router.messageRetrySend.gasPrice
		outgoingMessage.cxReceipt.GasLimit = router.messageRetrySend.gasLimit
		if err := evm.EmitCXReceipt(outgoingMessage.cxReceipt); err != nil {
			return nil, err
		}
		return outgoingMessage.msgAddress[:], nil
	}
}
