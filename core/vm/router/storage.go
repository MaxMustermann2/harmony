// This file handles the storage of cross shard messages.
// Note that it handles only types.cxReceipt and not types.CXReceipt
// Which is stored by rawdb.WriteCXReceipts
// TODO: avoid duplication of content stored by both
// (1) A message address is used instead of transaction hash
// since each transaction can involve multiple cross shard messages
// (2) To avoid collision with a message address and an EOA
// we are storing the message at the RouterAddress as key, value
// We also avoid the problem faced in validator storage
// by separating the components instead of using one single blob

package router

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/harmony-one/harmony/core/types"
	"github.com/harmony-one/harmony/core/vm"
	"golang.org/x/crypto/sha3"
)

// Offsets (in words) of various data in the outgoingMessage
const (
	omIdxFromAddrGasLimit = iota
	omIdxToAddressNonceToShard
	omIdxGasLeftoverToPayloadLen
	omIdxAmount
	omIdxGasBudget
	omIdxGasPrice
	omIdxPayloadHash
)

type OutgoingMessage struct {
	cxReceipt   *types.CXReceipt
	msgAddress  common.Address
	payloadHash common.Hash
}

func NewOutgoingMessage(cxReceipt *types.CXReceipt) *OutgoingMessage {
	outgoingMessage := &OutgoingMessage{
		cxReceipt: cxReceipt,
	}
	outgoingMessage.msgAddress, outgoingMessage.payloadHash =
		outgoingMessage.CalculateAddressAndPayloadHash()
	return outgoingMessage
}

// Compute the message address and address of the payload.
func (outgoingMessage *OutgoingMessage) CalculateAddressAndPayloadHash() (common.Address, common.Hash) {
	h := sha3.NewLegacyKeccak256()
	h.Write(outgoingMessage.cxReceipt.Payload)
	var payloadHash common.Hash
	copy(payloadHash[:], h.Sum(nil))
	h.Reset()
	h.Write([]byte{0xff})
	h.Write(outgoingMessage.cxReceipt.From[:])
	h.Write(outgoingMessage.cxReceipt.To[:])
	binary.Write(h, binary.BigEndian, outgoingMessage.cxReceipt.ShardID)
	binary.Write(h, binary.BigEndian, outgoingMessage.cxReceipt.ToShardID)
	h.Write(payloadHash[:])
	var valBytes [32]byte
	outgoingMessage.cxReceipt.Amount.FillBytes(valBytes[:])
	h.Write(valBytes[:])
	binary.Write(h, binary.BigEndian, outgoingMessage.cxReceipt.Nonce)
	var msgAddr common.Address
	copy(msgAddr[:], h.Sum(nil)[12:])
	return msgAddr, payloadHash
}

// compute the storage address of the nth word in the entry for
// this message.
func (outgoingMessage *OutgoingMessage) wordAddr(n uint8) common.Hash {
	var ret common.Hash
	copy(ret[:], outgoingMessage.msgAddress[:])
	// message address ++ 0x01 ++ <11 zero bytes>
	// ...where the 0x01 makes sure it does not collide
	// with the messages received map
	ret[20] = 0x01
	ret[31] = n
	return ret
}

func (outgoingMessage *OutgoingMessage) StoreMessage(db vm.StateDB) {
	// From Address (20) + Gas Limit (8)
	var buf common.Hash
	copy(buf[:20], outgoingMessage.cxReceipt.From[:])
	binary.BigEndian.PutUint64(buf[20:20+8], outgoingMessage.cxReceipt.GasLimit)
	db.SetState(RouterAddress, outgoingMessage.wordAddr(omIdxFromAddrGasLimit), buf)

	// ToAddress (20) + Nonce (8) + ToShardID (4)
	buf = common.Hash{}
	copy(buf[:20], outgoingMessage.cxReceipt.To[:])
	binary.BigEndian.PutUint64(buf[20:20+8], outgoingMessage.cxReceipt.Nonce)
	binary.BigEndian.PutUint32(buf[28:28+4], outgoingMessage.cxReceipt.ToShardID)
	db.SetState(RouterAddress, outgoingMessage.wordAddr(omIdxToAddressNonceToShard), buf)

	// GasLeftOverTo (20) + PayloadLength (8)
	buf = common.Hash{}
	copy(buf[:20], outgoingMessage.cxReceipt.GasLeftoverTo[:])
	binary.BigEndian.PutUint64(buf[20:20+8], uint64(len(outgoingMessage.cxReceipt.Payload)))
	db.SetState(RouterAddress, outgoingMessage.wordAddr(omIdxGasLeftoverToPayloadLen), buf)

	// Amount (32)
	buf = common.Hash{}
	outgoingMessage.cxReceipt.Amount.FillBytes(buf[:])
	db.SetState(RouterAddress, outgoingMessage.wordAddr(omIdxAmount), buf)

	// GasBudget (32)
	buf = common.Hash{}
	outgoingMessage.cxReceipt.GasBudget.FillBytes(buf[:])
	db.SetState(RouterAddress, outgoingMessage.wordAddr(omIdxGasBudget), buf)

	// GasPrice (32)
	buf = common.Hash{}
	outgoingMessage.cxReceipt.GasPrice.FillBytes(buf[:])
	db.SetState(RouterAddress, outgoingMessage.wordAddr(omIdxGasPrice), buf)

	// PayloadHash (32)
	buf = common.Hash{}
	copy(buf[:], outgoingMessage.payloadHash[:])
	db.SetState(RouterAddress, outgoingMessage.wordAddr(omIdxPayloadHash), buf)

	// Payload
	outgoingMessage.storePayload(db)
}

func (outgoingMessage *OutgoingMessage) storePayload(db vm.StateDB) {
	offset := outgoingMessage.payloadHash.Big()
	key := outgoingMessage.payloadHash
	data := make([]byte, len(outgoingMessage.cxReceipt.Payload))
	copy(data[:], outgoingMessage.cxReceipt.Payload)
	for len(data) > 0 {
		var val common.Hash
		copy(val[:], data[:])
		db.SetState(RouterAddress, key, val)
		if len(data) < len(val[:]) {
			data = nil
		} else {
			data = data[len(val[:]):]
			offset.Add(offset, big.NewInt(1))
			offset.FillBytes(key[:])
		}
	}
}

func LoadMessage(msgAddr common.Address, db vm.StateDB) (*OutgoingMessage, error) {
	outgoingMessage := &OutgoingMessage{
		msgAddress: msgAddr,
		cxReceipt:  &types.CXReceipt{},
	}

	// From Address (20) + Gas Limit (8)
	buf := db.GetState(RouterAddress, outgoingMessage.wordAddr(omIdxFromAddrGasLimit))
	copy(outgoingMessage.cxReceipt.From[:], buf[:20])
	outgoingMessage.cxReceipt.GasLimit = binary.BigEndian.Uint64(buf[20 : 20+8])

	// ToAddress (20) + Nonce (8) + ToShardID (4)
	buf = db.GetState(RouterAddress, outgoingMessage.wordAddr(omIdxToAddressNonceToShard))
	copy(outgoingMessage.cxReceipt.To[:], buf[:20])
	outgoingMessage.cxReceipt.Nonce = binary.BigEndian.Uint64(buf[20 : 20+8])
	outgoingMessage.cxReceipt.ToShardID = binary.BigEndian.Uint32(buf[28 : 28+4])

	// GasLeftOverTo (20) + PayloadLength (8)
	buf = db.GetState(RouterAddress, outgoingMessage.wordAddr(omIdxGasLeftoverToPayloadLen))
	copy(outgoingMessage.cxReceipt.GasLeftoverTo[:], buf[:20])
	payloadLength := binary.BigEndian.Uint64(buf[20 : 20+8])

	// Amount (32)
	buf = db.GetState(RouterAddress, outgoingMessage.wordAddr(omIdxAmount))
	outgoingMessage.cxReceipt.Amount.SetBytes(buf[:])

	// GasBudget (32)
	buf = db.GetState(RouterAddress, outgoingMessage.wordAddr(omIdxGasBudget))
	outgoingMessage.cxReceipt.GasBudget.SetBytes(buf[:])

	// GasPrice (32)
	buf = db.GetState(RouterAddress, outgoingMessage.wordAddr(omIdxGasPrice))
	outgoingMessage.cxReceipt.GasPrice.SetBytes(buf[:])

	// PayloadHash (32)
	outgoingMessage.payloadHash = db.GetState(RouterAddress, outgoingMessage.wordAddr(omIdxPayloadHash))

	// Payload
	outgoingMessage.loadPayload(db, payloadLength)

	calculatedAddress, calculatedHash := outgoingMessage.CalculateAddressAndPayloadHash()
	if !bytes.Equal(outgoingMessage.msgAddress.Bytes(), calculatedAddress.Bytes()) {
		return nil, errors.New(
			fmt.Sprintf(
				"unexpected address %s (should be %s)",
				calculatedAddress.Hex(),
				outgoingMessage.msgAddress.Hex(),
			),
		)
	}

	if !bytes.Equal(outgoingMessage.payloadHash.Bytes(), calculatedHash.Bytes()) {
		return nil, fmt.Errorf(
			"unexpected hash %s (should be %s)",
			calculatedHash.Hex(),
			outgoingMessage.payloadHash.Hex(),
		)
	}

	return outgoingMessage, nil
}

func (outgoingMessage *OutgoingMessage) loadPayload(db vm.StateDB, payloadLength uint64) {
	ret := make([]byte, payloadLength)
	buf := ret
	offset := outgoingMessage.payloadHash.Big()
	key := outgoingMessage.payloadHash
	for len(buf) > 0 {
		word := db.GetState(RouterAddress, key)
		copy(buf, word[:])
		if len(buf) < len(word) {
			buf = nil
		} else {
			buf = buf[len(word):]
			offset.Add(offset, big.NewInt(1))
			offset.FillBytes(key[:])
		}
	}
	outgoingMessage.cxReceipt.Payload = ret
}
