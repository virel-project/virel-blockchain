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
			TXID:      hash,
			TxVersion: tx.Version,
			Size:      tx.GetVirtualSize(),
			Fee:       tx.Fee,
			Expires:   time.Now().Add(config.MEMPOOL_EXPIRATION).Unix(),
			Signer:    signerAddr,
			Inputs:    tx.Data.StateInputs(tx, signerAddr),
			Outputs:   out,
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
	_, err := tx.TotalAmount()
	if err != nil {
		return fmt.Errorf("invalid total amount: %w", err)
	}

	// Get all inputs and outputs for the current transaction
	inputs := tx.Data.StateInputs(tx, signer)
	outputs := tx.Data.StateOutputs(tx, signer)

	// Verify input and output sums match
	var totalInputs, totalOutputs uint64
	for _, inp := range inputs {
		totalInputs += inp.Amount
	}
	for _, out := range outputs {
		totalOutputs += out.Amount
	}
	if totalInputs != totalOutputs+tx.Fee {
		return fmt.Errorf("input sum %d doesn't match output sum %d + fee %d",
			totalInputs, totalOutputs, tx.Fee)
	}

	// Collect all affected addresses
	affectedAddrs := make(map[address.Address]bool)
	affectedAddrs[signer] = true
	for _, inp := range inputs {
		affectedAddrs[inp.Sender] = true
	}
	for _, out := range outputs {
		affectedAddrs[out.Recipient] = true
	}

	// Initialize simulated states and delegates
	simulatedStates := make(map[address.Address]*State)
	simulatedDelegates := make(map[uint64]*Delegate)

	// Load initial states from database
	for addr := range affectedAddrs {
		state, err := bc.GetState(txn, addr)
		if err != nil {
			return fmt.Errorf("failed to get state for %s: %w", addr, err)
		}
		simulatedStates[addr] = state
	}

	// Track nonces for all signers from previous entries
	simulatedNonces := make(map[address.Address]uint64)
	simulatedNonces[signer] = simulatedStates[signer].LastNonce

	// Process previous entries to update simulated states and delegates
	for _, entry := range previousEntries {
		// stop if we have reached this transaction (should not happen)
		if entry.TXID == hash {
			return errors.New("previousEntries contains the transaction")
		}

		// Update nonce for entry's signer
		simulatedNonces[entry.Signer]++

		// Apply state changes from inputs
		for _, inp := range entry.Inputs {
			state := simulatedStates[inp.Sender]
			if state.Balance < inp.Amount {
				return fmt.Errorf("insufficient balance in previous entry %x", entry.TXID)
			}
			state.Balance -= inp.Amount
		}

		// Apply state changes from outputs
		for _, out := range entry.Outputs {
			state := simulatedStates[out.Recipient]
			state.Balance += out.Amount
			state.LastIncoming++
		}

		// Handle unstake transactions in previous entries
		if entry.TxVersion == transaction.TX_VERSION_UNSTAKE {
			tx, _, err := bc.GetTx(txn, entry.TXID)
			if err != nil {
				return fmt.Errorf("failed to get unstake transaction: %w", err)
			}

			unstakeData := tx.Data.(*transaction.Unstake)
			delegate, err := bc.getOrLoadDelegate(txn, unstakeData.DelegateId, simulatedDelegates)
			if err != nil {
				return err
			}

			// Find and update the fund for the entry signer
			found := false
			for _, fund := range delegate.Funds {
				if fund.Owner == entry.Signer {
					if fund.Amount < unstakeData.Amount {
						return fmt.Errorf("insufficient stake in previous entry %x", entry.TXID)
					}
					fund.Amount -= unstakeData.Amount
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("no stake found for %s in previous entry %x", entry.Signer, entry.TXID)
			}

			// Update simulated delegate
			simulatedDelegates[unstakeData.DelegateId] = delegate
		}
	}

	// Validate nonce for current transaction
	if tx.Nonce != simulatedNonces[signer]+1 {
		return fmt.Errorf("invalid nonce: expected %d, got %d",
			simulatedNonces[signer]+1, tx.Nonce)
	}

	// Validate sufficient balances for inputs
	for _, inp := range inputs {
		state := simulatedStates[inp.Sender]
		if state.Balance < inp.Amount {
			return fmt.Errorf("insufficient balance for %s: need %d, have %d",
				inp.Sender, inp.Amount, state.Balance)
		}
	}

	// Validate unstake transaction if applicable
	if tx.Version == transaction.TX_VERSION_UNSTAKE {
		unstakeData := tx.Data.(*transaction.Unstake)
		delegate, err := bc.getOrLoadDelegate(txn, unstakeData.DelegateId, simulatedDelegates)
		if err != nil {
			return err
		}

		// Check delegate ID matches signer's current delegate
		signerState := simulatedStates[signer]
		if signerState.DelegateId != unstakeData.DelegateId {
			return fmt.Errorf("signer delegate ID %d doesn't match unstake delegate ID %d",
				signerState.DelegateId, unstakeData.DelegateId)
		}

		// Check sufficient staked amount
		var stakedAmount uint64
		for _, fund := range delegate.Funds {
			if fund.Owner == signer {
				stakedAmount = fund.Amount
				break
			}
		}
		if stakedAmount < unstakeData.Amount {
			return fmt.Errorf("insufficient stake: need %d, have %d",
				unstakeData.Amount, stakedAmount)
		}
	}

	return nil
}

// Helper function to get delegate from simulatedDelegates or database
func (bc *Blockchain) getOrLoadDelegate(txn adb.Txn, delegateId uint64, simulatedDelegates map[uint64]*Delegate) (*Delegate, error) {
	if delegate, exists := simulatedDelegates[delegateId]; exists {
		return delegate, nil
	}
	delegate, err := bc.GetDelegate(txn, delegateId)
	if err != nil {
		return nil, fmt.Errorf("failed to get delegate %d: %w", delegateId, err)
	}
	return delegate, nil
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
