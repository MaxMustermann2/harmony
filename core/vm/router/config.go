package router

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	crossShardNonceStr = "Harmony/CrossShardNonce/v1"
)

var (
	RouterAddress      common.Address = common.BytesToAddress([]byte{248})
	CrossShardNonceKey common.Hash    = crypto.Keccak256Hash([]byte(crossShardNonceStr))
)
