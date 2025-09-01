package blockchain

import (
	"errors"
	"fmt"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
)

// Note: In case of error, the database transaction `txn` should be reversed.
func (bc *Blockchain) RemoveTxFromState(
	txn adb.Txn, tx *transaction.Transaction, signerAddr address.Address, bl *block.Block, blockhash util.Hash, stats *Stats, txid transaction.TXID,
) error {
	// reset tx height
	err := bc.SetTxHeight(txn, txid, 0)
	if err != nil {
		return err
	}

	// remove the funds from the outputs
	stateoutputs := tx.Data.StateOutputs(tx, signerAddr)
	bc.RemoveTxOutputsFromState(txn, blockhash, stateoutputs, txid, stats)
	// NOTE: We don't have to remove the TxTopoOut/TxTopoInc, since we are reducing LastIncoming/LastNonce, so they are never read and later overwritten.

	// remove inputs
	for _, inp := range tx.Data.StateInputs(tx, signerAddr) {
		senderState, err := bc.GetState(txn, inp.Sender)
		if err != nil {
			return err
		}

		senderState.Balance += inp.Amount
		err = bc.SetState(txn, inp.Sender, senderState)
		if err != nil {
			return err
		}
	}

	// decrease signer nonce
	signerState, err := bc.GetState(txn, signerAddr)
	if err != nil {
		return err
	}
	if signerState.LastNonce == 0 {
		err = fmt.Errorf("sender %s last nonce must not be zero in tx %s", signerAddr, txid)
		return err
	}
	signerState.LastNonce--
	err = bc.SetState(txn, signerAddr, signerState)
	if err != nil {
		return err
	}

	// undo unstake if the tx is an unstake transaction
	if tx.Version == transaction.TX_VERSION_UNSTAKE {
		unstakeData := tx.Data.(*transaction.Unstake)
		if unstakeData.DelegateId == 0 {
			return errors.New("unstake transaction has invalid delegate id")
		}
		if signerState.DelegateId != unstakeData.DelegateId {
			return fmt.Errorf("unstake transaction delegate id %d does not match with state %d",
				unstakeData.DelegateId, signerState.DelegateId)
		}
		err = bc.RemoveUnstake(txn, blockhash, unstakeData, signerAddr, txid, stats)
		if err != nil {
			return fmt.Errorf("could not remove unstake: %w", err)
		}
	}

	return nil
}

func (bc *Blockchain) RemoveUnstake(txn adb.Txn, blockhash util.Hash, unstakeData *transaction.Unstake, signerAddr address.Address, txid transaction.TXID, stats *Stats) error {
	delegate, err := bc.GetDelegate(txn, unstakeData.DelegateId)
	if err != nil {
		return err
	}

	unstaked := false
	for _, fund := range delegate.Funds {
		if fund.Owner != signerAddr {
			continue
		}
		fund.Amount += unstakeData.Amount
		unstaked = true
		break
	}
	// If the fund was not found, we add it back.
	// Order is not important as it's sorted later with SetDelegate.
	if !unstaked {
		delegate.Funds = append(delegate.Funds, &DelegatedFund{
			Owner:  signerAddr,
			Amount: unstakeData.Amount,
		})
	}

	// reverse stats.Unstaked
	err = stats.Staked(unstakeData.Amount)
	if err != nil {
		return fmt.Errorf("failed to remove from stats, this should never happen: %w", err)
	}

	// Update the delegate in the state (note: SetDelegate also sorts the funds)
	err = bc.SetDelegate(txn, delegate)
	if err != nil {
		return fmt.Errorf("failed to set delegate, this should never happen: %w", err)
	}
	return nil
}

func (bc *Blockchain) RemoveTxOutputsFromState(txn adb.Txn, blockhash util.Hash, outs []transaction.StateOutput, txid transaction.TXID, stats *Stats) error {
	for _, out := range outs {
		recState, err := bc.GetState(txn, out.Recipient)
		if err != nil {
			return fmt.Errorf("failed to get output state: %w", err)
		}
		if recState.Balance < out.Amount {
			return fmt.Errorf("recipient balance is smaller than output amount: %d < %d", recState.Balance, out.Amount)
		}
		if recState.LastIncoming == 0 {
			return fmt.Errorf("recipient %s LastIncoming must not be zero in tx %s", out.Recipient, txid)
		}
		recState.Balance -= out.Amount
		recState.LastIncoming--
		err = bc.SetState(txn, out.Recipient, recState)
		if err != nil {
			return fmt.Errorf("failed to set state: %w", err)
		}
		// Reverse coinbase PoS
		if out.Type == transaction.OUT_COINBASE_POS {
			bc.RemovePosReward(txn, blockhash, &out, txid, stats)
		}
	}
	return nil
}

func (bc *Blockchain) RemovePosReward(txn adb.Txn, blockhash util.Hash, out *transaction.StateOutput, txid transaction.TXID, stats *Stats) error {
	if out.ExtraData == 0 {
		return fmt.Errorf("invalid delegate id %d", out.ExtraData)
	}
	// Reverse the PoS reward distribution
	delegate, err := bc.GetDelegate(txn, out.ExtraData)
	if err != nil {
		return err
	}

	if len(delegate.Funds) == 0 {
		return fmt.Errorf("delegate has no funds")
	}

	oldDelegate, err := bc.GetDelegateHistory(txn, blockhash)
	if err != nil {
		return fmt.Errorf("failed to get delegate history: %w", err)
	}
	if oldDelegate.Id != delegate.Id || oldDelegate.Owner != delegate.Owner {
		return fmt.Errorf("delegate historical data does not match with the current data, this should never happen")
	}

	return bc.SetDelegate(txn, delegate)
}
