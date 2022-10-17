package router

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/harmony-one/harmony/accounts/abi"
)

var RouterABI = `
[
  {
    "inputs": [
      {
        "internalType": "address",
        "name": "msgAddr",
        "type": "address"
      },
      {
        "internalType": "uint256",
        "name": "gasLimit",
        "type": "uint256"
      },
      {
        "internalType": "uint256",
        "name": "gasPrice",
        "type": "uint256"
      }
    ],
    "name": "retrySend",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "address",
        "name": "to_",
        "type": "address"
      },
      {
        "internalType": "shardId",
        "name": "toShard",
        "type": "uint32"
      },
      {
        "internalType": "bytes",
        "name": "payload",
        "type": "bytes"
      },
      {
        "internalType": "uint256",
        "name": "gasBudget",
        "type": "uint256"
      },
      {
        "internalType": "uint256",
        "name": "gasPrice",
        "type": "uint256"
      },
      {
        "internalType": "uint256",
        "name": "gasLimit",
        "type": "uint256"
      },
      {
        "internalType": "address",
        "name": "gasLeftoverTo",
        "type": "address"
      }
    ],
    "name": "send",
    "outputs": [
      {
        "internalType": "address",
        "name": "msgAddr",
        "type": "address"
      }
    ],
    "stateMutability": "nonpayable",
    "type": "function"
  }
]
`
var abiRouter abi.ABI

func init() {
	var err error
	abiRouter, err = abi.JSON(strings.NewReader(RouterABI))
	if err != nil {
		panic(fmt.Sprintf("the router ABI is incorrect: %s", err))
	}
}

type routerMessageSend struct {
	to            common.Address
	toShard       uint32
	payload       []byte
	gasPrice      *big.Int
	gasBudget     *big.Int
	gasLimit      uint64
	gasLeftoverTo common.Address
}

type routerMessageRetrySend struct {
	msgAddr  common.Address
	gasLimit uint64
	gasPrice *big.Int
}

// parseMethod converts the byte argument into either
// routerMessageSend or routerMessageRetrySend
// it does not validate the data beyond sizes and types
// which is done via the ABI module
func parseMethod(input []byte) (interface{}, error) {
	method, err := abiRouter.MethodById(input)
	if err != nil {
		return nil, err
	}
	input = input[4:]                // drop the method selector
	args := map[string]interface{}{} // store into map
	if err = method.Inputs.UnpackIntoMap(args, input); err != nil {
		return nil, err
	}
	// UnpackIntoInterface returns a list of interfaces and requires casting anyway
	switch method.Name {
	case "send":
		{
			to, err := abi.ParseAddressFromKey(args, "to_")
			if err != nil {
				return nil, err
			}
			toShard, err := abi.ParseUint32FromKey(args, "toShard")
			if err != nil {
				return nil, err
			}
			payload, err := abi.ParseBytesFromKey(args, "payload")
			if err != nil {
				return nil, err
			}
			gasPrice, err := abi.ParseBigIntFromKey(args, "gasPrice")
			if err != nil {
				return nil, err
			}
			gasLimit, err := abi.ParseUint64FromKey(args, "gasLimit")
			if err != nil {
				return nil, err
			}
			gasBudget, err := abi.ParseBigIntFromKey(args, "gasBudget")
			if err != nil {
				return nil, err
			}
			gasLeftoverTo, err := abi.ParseAddressFromKey(args, "gasLeftoverTo")
			if err != nil {
				return nil, err
			}
			return &routerMessageSend{
				to:            to,
				toShard:       toShard,
				payload:       payload,
				gasPrice:      gasPrice,
				gasLimit:      gasLimit,
				gasBudget:     gasBudget,
				gasLeftoverTo: gasLeftoverTo,
			}, nil
		}
	case "retrySend":
		{
			msgAddr, err := abi.ParseAddressFromKey(args, "msgAddr")
			if err != nil {
				return nil, err
			}
			gasPrice, err := abi.ParseBigIntFromKey(args, "gasPrice")
			if err != nil {
				return nil, err
			}
			gasLimit, err := abi.ParseUint64FromKey(args, "gasLimit")
			if err != nil {
				return nil, err
			}
			return &routerMessageRetrySend{
				msgAddr:  msgAddr,
				gasLimit: gasLimit,
				gasPrice: gasPrice,
			}, nil
		}
	default:
		{
			return nil, errors.New("unknown method name")
		}
	}
}
