package blockchain

import (
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/virel-project/virel-blockchain/adb"
	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/binary"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/p2p"
	"github.com/virel-project/virel-blockchain/p2p/packet"
	"github.com/virel-project/virel-blockchain/transaction"
)

// Adds a transaction to mempool.
// Transaction must be already prevalidated.
// Blockchain MUST be locked before calling this
func (bc *Blockchain) AddTransaction(txn adb.Txn, tx *transaction.Transaction, hash [32]byte, mempool bool) error {
	// check that transaction is not duplicate
	if txn.Get(bc.Index.Tx, hash[:]) != nil {
		Log.Debug("transaction is already in database")
		return nil
	}

	var mem *Mempool

	// only validate the transaction if it's added to mempool: transactions added to chain are verified
	// later, when the block is applied to state
	if mempool {
		mem = bc.GetMempool(txn)

		// validate the transaction
		err := bc.validateMempoolTx(txn, tx, hash, mem.Entries)
		if err != nil {
			Log.Err("transaction is not valid in mempool:", err)
			return err
		}

		go bc.BroadcastTx(hash, tx)
	}

	err := bc.SetTx(txn, tx, hash, 0)
	if err != nil {
		Log.Err(err)
		return err
	}

	if mempool {
		if mem.GetEntry(hash) != nil {
			err := fmt.Errorf("transaction %x already in mempool", hash)
			Log.Warn(err)
			return nil
		}
		mem.Entries = append(mem.Entries, &MempoolEntry{
			TXID:    hash,
			Size:    tx.GetVirtualSize(),
			Fee:     tx.Fee,
			Expires: time.Now().Add(config.MEMPOOL_EXPIRATION).Unix(),
			Sender:  address.FromPubKey(tx.Sender),
			Outputs: tx.Outputs,
		})
		err = bc.SetMempool(txn, mem)
		if err != nil {
			return err
		}
		Log.Debugf("Added transaction %x to mempool", hash)
	} else {
		Log.Debugf("Added transaction %x", hash)
	}

	return nil
}

// GetTx returns the transaction given its hash, and the transaction height if available
// Blockchain MUST be RLocked before calling this
func (bc *Blockchain) GetTx(txn adb.Txn, hash [32]byte) (*transaction.Transaction, uint64, error) {
	tx := &transaction.Transaction{}
	// read TX data
	txbin := txn.Get(bc.Index.Tx, hash[:])
	if len(txbin) == 0 {
		return nil, 0, fmt.Errorf("transaction %x not found", hash)
	}
	des := binary.NewDes(txbin)
	includedIn := des.ReadUint64()
	if des.Error() != nil {
		return nil, 0, fmt.Errorf("failed to deserialize transaction: %w", des.Error())
	}
	return tx, includedIn, tx.Deserialize(des.RemainingData())
}

func (bc *Blockchain) SetTx(txn adb.Txn, tx *transaction.Transaction, hash transaction.TXID, height uint64) error {
	serTx := tx.Serialize()

	ser := binary.NewSer(make([]byte, 0, 8+len(serTx)))
	ser.AddUint64(height)
	ser.AddFixedByteArray(serTx)

	err := txn.Put(bc.Index.Tx, hash[:], ser.Output())
	if err != nil {
		Log.Err(err)
		return err
	}
	return nil
}

func (bc *Blockchain) SetTxHeight(txn adb.Txn, hash transaction.TXID, height uint64) error {
	txbin := txn.Get(bc.Index.Tx, hash[:])
	if len(txbin) < 8 {
		return errors.New("cannot SetTxHeight: transaction not in database")
	}

	binary.LittleEndian.PutUint64(txbin[:8], height)

	txn.Put(bc.Index.Tx, hash[:], txbin)

	return nil
}

// use this method to validate that a transaction in mempool is valid
func (bc *Blockchain) validateMempoolTx(txn adb.Txn, tx *transaction.Transaction, hash [32]byte, previousEntries []*MempoolEntry) error {
	senderAddr := address.FromPubKey(tx.Sender)

	// get sender state
	senderState, err := bc.GetState(txn, senderAddr)
	if err != nil {
		Log.Err(err)
		return err
	}

	// apply all the previous mempool transactions to sender state
	Log.Dev("sender state before applying all the mempool transactions:", senderState)
	for _, v := range previousEntries {
		if v.TXID == hash {
			// avoid applying this tx (or future transactions) - mempool is guaranteed to be ordered correctly
			break
		}

		// apply this transaction if it modifies sender state
		if v.Sender == senderAddr || slices.ContainsFunc(v.Outputs, func(e transaction.Output) bool { return e.Recipient == senderAddr }) {
			vt, _, err := bc.GetTx(txn, v.TXID)
			if err != nil {
				Log.Err(err)
				return err
			}

			if v.Sender == senderAddr {
				senderState.Balance -= vt.TotalAmount()
				senderState.Balance -= vt.Fee
				senderState.LastNonce++
			}

			for _, out := range v.Outputs {
				if out.Recipient == senderAddr {
					senderState.Balance += out.Amount
				}
			}
		}
	}
	Log.Debug("sender state after applying all the mempool transactions:", senderState)

	if senderState.Balance < tx.TotalAmount() {
		err = fmt.Errorf("transaction %s spends too much money: balance: %d, amount: %d, fee: %d", hex.EncodeToString(hash[:]),
			senderState.Balance, tx.TotalAmount(), tx.Fee)
		Log.Warn(err)
		return err
	}
	if tx.Nonce != senderState.LastNonce+1 {
		err = fmt.Errorf("transaction %x has unexpected nonce: %d, previous nonce: %d", hash,
			tx.Nonce, senderState.LastNonce)
		Log.Warn(err)
		return err
	}

	return nil
}

func (bc *Blockchain) BroadcastTx(hash [32]byte, tx *transaction.Transaction) {
	Log.Debugf("broadcasting transaction %x", hash)
	for _, c := range bc.P2P.Connections {
		c.SendPacket(&p2p.Packet{
			Type: packet.TX,
			Data: tx.Serialize(),
		})
	}
}
