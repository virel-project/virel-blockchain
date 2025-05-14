package blockchain

import (
	"errors"
	"fmt"
	"slices"
	"time"
	"virel-blockchain/address"
	"virel-blockchain/binary"
	"virel-blockchain/config"
	"virel-blockchain/p2p"
	"virel-blockchain/p2p/packet"
	"virel-blockchain/transaction"
	"virel-blockchain/util/buck"

	bolt "go.etcd.io/bbolt"
)

// Adds a transaction to mempool.
// Transaction must be already prevalidated.
// Blockchain MUST be locked before calling this
func (bc *Blockchain) AddTransaction(txn *bolt.Tx, tx *transaction.Transaction, hash [32]byte, mempool bool) error {
	b := txn.Bucket([]byte{buck.TX})

	// check that transaction is not duplicate
	if b.Get(hash[:]) != nil {
		Log.Debug("transaction is already in database")
		return nil
	}

	// only validate the transaction if it's added to mempool: transactions added to chain are verified
	// later, when the block is applied to state
	if mempool {
		// validate the transaction
		err := bc.validateMempoolTx(txn, tx, hash)
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

	b = txn.Bucket([]byte{buck.INFO})

	if mempool {
		mem := bc.buckGetMempool(b)
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
		bc.buckSetMempool(b, mem)
		Log.Debugf("Added transaction %x to mempool", hash)
	} else {
		Log.Debugf("Added transaction %x", hash)
	}

	return nil
}

// GetTx returns the transaction given its hash, and the transaction height if available
// Blockchain MUST be RLocked before calling this
func (bc *Blockchain) GetTx(hash [32]byte) (*transaction.Transaction, uint64, error) {
	tx := &transaction.Transaction{}
	var includedIn uint64
	err := bc.DB.View(func(txn *bolt.Tx) (err error) {
		// read TX data
		b := txn.Bucket([]byte{buck.TX})
		tx, includedIn, err = bc.buckGetTx(b, hash)
		return
	})
	return tx, includedIn, err
}

// buckGetBlock returns the block given its hash, using the database bucket
func (bc *Blockchain) buckGetTx(buck *bolt.Bucket, hash transaction.TXID) (*transaction.Transaction, uint64, error) {
	tx := &transaction.Transaction{}
	txbin := buck.Get(hash[:])
	if len(txbin) == 0 {
		return nil, 0, fmt.Errorf("transaction %x not found", hash)
	}

	des := binary.NewDes(txbin)

	height := des.ReadUint64()
	if des.Error() != nil {
		return nil, 0, des.Error()
	}

	return tx, height, tx.Deserialize(des.RemainingData())
}

func (bc *Blockchain) SetTx(txn *bolt.Tx, tx *transaction.Transaction, hash transaction.TXID, height uint64) error {
	b := txn.Bucket([]byte{buck.TX})

	serTx := tx.Serialize()

	ser := binary.NewSer(make([]byte, 0, 8+len(serTx)))
	ser.AddUint64(height)
	ser.AddFixedByteArray(serTx)

	err := b.Put(hash[:], ser.Output())
	if err != nil {
		Log.Err(err)
		return err
	}
	return nil
}

func (bc *Blockchain) SetTxHeight(txn *bolt.Tx, hash transaction.TXID, height uint64) error {
	b := txn.Bucket([]byte{buck.TX})

	txbin := b.Get(hash[:])
	if len(txbin) < 8 {
		return errors.New("cannot SetTxHeight: transaction not in database")
	}

	binary.LittleEndian.PutUint64(txbin[:8], height)

	b.Put(hash[:], txbin)

	return nil
}

// use this method to validate that a transaction in mempool is valid
func (bc *Blockchain) validateMempoolTx(txn *bolt.Tx, tx *transaction.Transaction, hash [32]byte) error {
	bstate := txn.Bucket([]byte{buck.STATE})
	btx := txn.Bucket([]byte{buck.TX})

	senderAddr := address.FromPubKey(tx.Sender)

	// get sender state
	senderState, err := bc.buckGetState(bstate, senderAddr)
	if err != nil {
		Log.Err(err)
		return err
	}

	// apply all the previous mempool transactions to sender state
	Log.Dev("sender state before applying all the mempool transactions:", senderState)
	mem := bc.GetMempool(txn)
	for _, v := range mem.Entries {
		if v.TXID == hash {
			// avoid applying this tx (or future transactions) - mempool is guaranteed to be ordered correctly
			break
		}

		// apply this transaction if it modifies sender state
		if v.Sender == senderAddr || slices.ContainsFunc(v.Outputs, func(e transaction.Output) bool { return e.Recipient == senderAddr }) {
			vt, _, err := bc.buckGetTx(btx, v.TXID)
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
	Log.Dev("sender state after applying all the mempool transactions:", senderState)

	if senderState.Balance < tx.TotalAmount() {
		err = fmt.Errorf("transaction %x spends too much money: balance: %d, amount: %d, fee: %d", hash,
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
