package blockchain

import (
	"errors"
	"fmt"
	"time"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/p2p"
	"github.com/virel-project/virel-blockchain/v2/p2p/packet"
	"github.com/virel-project/virel-blockchain/v2/transaction"
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
		signerAddr := address.FromPubKey(tx.Signer)
		sout := tx.Data.StateOutputs(tx, signerAddr)
		out := make([]transaction.Output, len(sout))
		for i, v := range sout {
			out[i] = transaction.Output{
				Recipient: v.Recipient,
				PaymentId: v.PaymentId,
				Amount:    v.Amount,
			}
		}
		mem.Entries = append(mem.Entries, &MempoolEntry{
			TXID:    hash,
			Size:    tx.GetVirtualSize(),
			Fee:     tx.Fee,
			Expires: time.Now().Add(config.MEMPOOL_EXPIRATION).Unix(),
			Signer:  signerAddr,
			Inputs:  tx.Data.StateInputs(tx, signerAddr),
			Outputs: out,
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
	return tx, includedIn, tx.Deserialize(des.RemainingData(), includedIn >= config.HARDFORK_V2_HEIGHT || includedIn == 0)
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

// Use this method to validate that a transaction in mempool is valid.
// previousEntries are expected to be correctly ordered and valid. If they aren't, something is wrong elsewhere.
func (bc *Blockchain) validateMempoolTx(txn adb.Txn, tx *transaction.Transaction, hash [32]byte, previousEntries []*MempoolEntry) error {
	signer := address.FromPubKey(tx.Signer)

	// Calculate total transaction amount (outputs + fee)
	totalAmount, err := tx.TotalAmount()
	if err != nil {
		Log.Err(err)
		return err
	}

	// Group inputs by sender and calculate total input amounts
	inputAmounts := make(map[address.Address]uint64)
	inputs := tx.Data.StateInputs(tx, signer)
	for _, input := range inputs {
		inputAmounts[input.Sender] += input.Amount
	}

	// Verify total inputs match outputs + fee
	var totalInputs uint64
	for _, amount := range inputAmounts {
		totalInputs += amount
	}
	if totalInputs != totalAmount {
		err := fmt.Errorf("transaction %x input sum %d doesn't match output sum %d + fee %d",
			hash, totalInputs, totalAmount-tx.Fee, tx.Fee)
		Log.Warn(err)
		return err
	}

	// Get initial signer state
	signerState, err := bc.GetState(txn, signer)
	if err != nil {
		Log.Err(err)
		return err
	}

	// Track expected nonce for signer
	signerNonce := signerState.LastNonce

	// Process previous entries to update balances and nonces
	simulatedStates := make(map[address.Address]*State)
	for _, entry := range previousEntries {
		if entry.TXID == hash {
			break
		}

		// Update expected nonce if entry is from same signer
		if entry.Signer == signer {
			signerNonce++
		}

		// Apply entry to affected sender states
		for sender := range inputAmounts {
			if _, exists := simulatedStates[sender]; !exists {
				senderState, err := bc.GetState(txn, sender)
				if err != nil {
					Log.Err(err)
					return err
				}
				simulatedStates[sender] = senderState
			}

			// Apply input deductions
			for _, input := range entry.Inputs {
				if input.Sender == sender {
					if input.Amount > simulatedStates[sender].Balance {
						err := fmt.Errorf("insufficient balance in transaction %x: need %d, have %d",
							entry.TXID, input.Amount, simulatedStates[sender].Balance)
						Log.Err(err)
						return err
					}
					simulatedStates[sender].Balance -= input.Amount
				}
			}

			// Apply output additions
			for _, output := range entry.Outputs {
				if output.Recipient == sender {
					simulatedStates[sender].Balance += output.Amount
				}
			}
		}
	}

	// Validate each sender's balance covers their inputs
	for sender, amount := range inputAmounts {
		state := simulatedStates[sender]
		if state.Balance < amount {
			err := fmt.Errorf("insufficient balance for sender %s: need %d, have %d",
				sender, amount, state.Balance)
			Log.Warn(err)
			return err
		}
	}

	// Validate signer's nonce
	if tx.Nonce != signerNonce+1 {
		err := fmt.Errorf("invalid nonce for transaction %x: expected %d, got %d",
			hash, signerNonce+1, tx.Nonce)
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
