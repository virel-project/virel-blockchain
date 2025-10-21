package blockchain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/virel-project/virel-blockchain/v3/adb"
	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/binary"
	"github.com/virel-project/virel-blockchain/v3/bitcrypto"
	"github.com/virel-project/virel-blockchain/v3/chaintype"
	"github.com/virel-project/virel-blockchain/v3/config"
	"github.com/virel-project/virel-blockchain/v3/p2p"
	"github.com/virel-project/virel-blockchain/v3/p2p/packet"
	"github.com/virel-project/virel-blockchain/v3/transaction"
	"github.com/virel-project/virel-blockchain/v3/util"
)

// Adds a transaction to mempool.
// Transaction must be already prevalidated.
// Blockchain MUST be locked before calling this
func (bc *Blockchain) AddTransaction(txn adb.Txn, tx *transaction.Transaction, hash [32]byte, mempool bool, height uint64) error {
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
		err := bc.validateMempoolTx(txn, tx, hash, mem.Entries, height)
		if err != nil {
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
			return fmt.Errorf("transaction %x already in mempool", hash)
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
func (bc *Blockchain) GetTx(txn adb.Txn, hash [32]byte, topheight uint64) (*transaction.Transaction, uint64, error) {
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

	if includedIn == 0 {
		if topheight > config.HARDFORK_V2_HEIGHT {
			err := tx.Deserialize(des.RemainingData(), true)
			if err == nil {
				return tx, includedIn, nil
			}
			tx = &transaction.Transaction{}
			err = tx.Deserialize(des.RemainingData(), false)
			return tx, includedIn, err
		} else {
			err := tx.Deserialize(des.RemainingData(), false)
			if err == nil {
				return tx, includedIn, nil
			}
			tx = &transaction.Transaction{}
			err = tx.Deserialize(des.RemainingData(), true)
			return tx, includedIn, err
		}
	}

	return tx, includedIn, tx.Deserialize(des.RemainingData(), includedIn >= config.HARDFORK_V2_HEIGHT)
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
func (bc *Blockchain) validateMempoolTx(txn adb.Txn, tx *transaction.Transaction, hash [32]byte, previousEntries []*MempoolEntry, nextheight uint64) error {
	// relay rule: check fee is enough for FEE_PER_BYTE_V2
	minFee := config.FEE_PER_BYTE_V2 * tx.GetVirtualSize()
	if tx.Fee < minFee {
		return fmt.Errorf("invalid tx fee %s, expected at least %s", util.FormatCoin(tx.Fee), util.FormatCoin(minFee))
	}

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
	simulatedStates := make(map[address.Address]*chaintype.State)
	simulatedDelegates := make(map[uint64]*chaintype.Delegate)

	// Load initial states from database
	for addr := range affectedAddrs {
		state, err := bc.GetState(txn, addr)
		if err != nil {
			if strings.Contains(err.Error(), "not in state") {
				Log.Debug("address", addr, "not previously in state")
				state = &chaintype.State{}
			} else {
				return err
			}
		}
		simulatedStates[addr] = state
	}

	// Process previous entries to update simulated states and delegates
	for _, entry := range previousEntries {
		// stop if we have reached this transaction (should not happen)
		if entry.TXID == hash {
			return errors.New("previousEntries contains the transaction")
		}

		// Update nonce for entry's signer
		signerState := simulatedStates[entry.Signer]
		if signerState != nil {
			signerState.LastNonce++
		}

		// Apply state changes from inputs
		for _, inp := range entry.Inputs {
			state := simulatedStates[inp.Sender]
			if state != nil {
				if state.Balance < inp.Amount {
					return fmt.Errorf("insufficient balance in previous entry %x", entry.TXID)
				}
				state.Balance -= inp.Amount
			}
		}

		// Apply state changes from outputs
		for _, out := range entry.Outputs {
			state := simulatedStates[out.Recipient]
			if state != nil {
				state.Balance += out.Amount
				state.LastIncoming++
			}
		}

		stats := bc.GetStats(txn)
		// Handle register delegate transactions in previous entries
		if entry.TxVersion == transaction.TX_VERSION_REGISTER_DELEGATE {
			tx, _, err := bc.GetTx(txn, entry.TXID, stats.TopHeight)
			if err != nil {
				return fmt.Errorf("failed to get register delegate transaction: %w", err)
			}

			registerData := tx.Data.(*transaction.RegisterDelegate)

			simulatedDelegates[registerData.Id] = &chaintype.Delegate{
				Id:    registerData.Id,
				Owner: bitcrypto.Pubkey{}, // TODO: we don't have the owner public key here. But it's ok, it should not be necessary.
				Name:  registerData.Name,
				Funds: []*chaintype.DelegatedFund{},
			}
		}
		// Handle stake transactions in previous entries
		if entry.TxVersion == transaction.TX_VERSION_STAKE {
			tx, _, err := bc.GetTx(txn, entry.TXID, stats.TopHeight)
			if err != nil {
				return fmt.Errorf("failed to get stake transaction: %w", err)
			}

			stakeData := tx.Data.(*transaction.Stake)
			delegate, err := bc.getOrLoadDelegate(txn, stakeData.DelegateId, simulatedDelegates)
			if err != nil {
				return err
			}

			// Find and update the fund for the entry signer
			found := false
			for _, fund := range delegate.Funds {
				if fund.Owner == entry.Signer {
					if fund.Unlock != stakeData.PrevUnlock {
						return fmt.Errorf("stake transaction PrevUnlock %d does not match fund unlock %d", stakeData.PrevUnlock, fund.Unlock)
					}
					fund.Amount, err = util.SafeAdd(fund.Amount, stakeData.Amount)
					fund.Unlock = nextheight + config.STAKE_UNLOCK_TIME
					if err != nil {
						return err
					}
					found = true
					break
				}
			}
			if !found {
				delegate.Funds = append(delegate.Funds, &chaintype.DelegatedFund{
					Owner:  signer,
					Amount: stakeData.Amount,
					Unlock: nextheight + config.STAKE_UNLOCK_TIME,
				})
			}

			// Update simulated delegate
			simulatedDelegates[stakeData.DelegateId] = delegate
		}
		// Handle unstake transactions in previous entries
		if entry.TxVersion == transaction.TX_VERSION_UNSTAKE {
			tx, _, err := bc.GetTx(txn, entry.TXID, stats.TopHeight)
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
		// Handle set delegate transactions in previous entries
		if entry.TxVersion == transaction.TX_VERSION_SET_DELEGATE {
			tx, _, err := bc.GetTx(txn, entry.TXID, stats.TopHeight)
			if err != nil {
				return fmt.Errorf("failed to get unstake transaction: %w", err)
			}

			setDelegateData := tx.Data.(*transaction.SetDelegate)

			if setDelegateData.PreviousDelegate != simulatedStates[signer].DelegateId {
				return fmt.Errorf("set delegate transaction has invalid PreviousDelegate %d, expected %d", setDelegateData.PreviousDelegate, simulatedStates[signer].DelegateId)
			}

			if simulatedStates[signer] == nil {
				simulatedStates[signer], err = bc.GetState(txn, signer)
				if err != nil {
					return err
				}
			}

			simulatedStates[signer].DelegateId = setDelegateData.DelegateId
		}
	}

	// Validate nonce for current transaction
	if tx.Nonce != simulatedStates[signer].LastNonce+1 {
		return fmt.Errorf("invalid nonce: expected %d, got %d",
			simulatedStates[signer].LastNonce+1, tx.Nonce)
	}

	// Validate sufficient balances for inputs
	for _, inp := range inputs {
		state := simulatedStates[inp.Sender]
		if state == nil {
			return fmt.Errorf("input sender state for %s not found in simulated states", inp.Sender)
		}
		if state.Balance < inp.Amount {
			return fmt.Errorf("insufficient balance for %s: need %d, have %d",
				inp.Sender, inp.Amount, state.Balance)
		}
	}

	// Validate stake transaction if applicable
	if tx.Version == transaction.TX_VERSION_STAKE {
		stakeData := tx.Data.(*transaction.Stake)
		delegate, err := bc.getOrLoadDelegate(txn, stakeData.DelegateId, simulatedDelegates)
		if err != nil {
			return err
		}

		// Check stake PrevUnlock
		for _, fund := range delegate.Funds {
			if fund.Owner == signer {
				if fund.Unlock != stakeData.PrevUnlock {
					return fmt.Errorf("stake transaction PrevUnlock %d does not match fund unlock %d", stakeData.PrevUnlock, fund.Unlock)
				}
				break
			}
		}

		// Check delegate ID matches signer's current delegate
		signerState := simulatedStates[signer]
		if signerState.DelegateId != stakeData.DelegateId {
			return fmt.Errorf("signer delegate ID %d doesn't match stake delegate ID %d",
				signerState.DelegateId, stakeData.DelegateId)
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
		var unlockHeight uint64
		for _, fund := range delegate.Funds {
			if fund.Owner == signer {
				stakedAmount = fund.Amount
				unlockHeight = fund.Unlock
				break
			}
		}
		if stakedAmount == 0 && unlockHeight == 0 {
			return fmt.Errorf("unstake failed: there's no such stake in the delegate funds")
		}
		if unlockHeight > nextheight {
			return fmt.Errorf("can't unstake: unlock height is %d, next height %d", unlockHeight, nextheight)
		}

		if stakedAmount < unstakeData.Amount {
			return fmt.Errorf("insufficient stake: need %d, have %d",
				unstakeData.Amount, stakedAmount)
		}
	}
	// Validate register delegate transaction if applicable
	if tx.Version == transaction.TX_VERSION_REGISTER_DELEGATE {
		registerDelegateData := tx.Data.(*transaction.RegisterDelegate)

		_, err := bc.getOrLoadDelegate(txn, registerDelegateData.Id, simulatedDelegates)
		if err == nil {
			return fmt.Errorf("delegate %d is already registered", registerDelegateData.Id)
		}
	}
	// Validate set delegate transaction if applicable
	if tx.Version == transaction.TX_VERSION_SET_DELEGATE {
		setDelegateData := tx.Data.(*transaction.SetDelegate)

		if setDelegateData.PreviousDelegate != simulatedStates[signer].DelegateId {
			return fmt.Errorf("set delegate transaction has invalid PreviousDelegate %d, expected %d", setDelegateData.PreviousDelegate, simulatedStates[signer].DelegateId)
		}

		// check that the user does not have staked funds (in that case, changing delegate is not possible)
		prevDelegate, err := bc.getOrLoadDelegate(txn, setDelegateData.PreviousDelegate, simulatedDelegates)
		if err == nil {
			for _, v := range prevDelegate.Funds {
				if v.Owner == signer {
					return fmt.Errorf("all funds must be unstaked before delegate can be changed")
				}
			}
		}

		// check that the next delegate exists
		_, err = bc.getOrLoadDelegate(txn, setDelegateData.DelegateId, simulatedDelegates)
		if err != nil {
			return fmt.Errorf("transaction tries to set invalid delegate: %w", err)
		}
	}

	return nil
}

// Helper function to get delegate from simulatedDelegates or database
func (bc *Blockchain) getOrLoadDelegate(txn adb.Txn, delegateId uint64, simulatedDelegates map[uint64]*chaintype.Delegate) (*chaintype.Delegate, error) {
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
