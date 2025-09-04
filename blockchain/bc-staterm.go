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

		senderState.Balance, err = util.SafeAdd(senderState.Balance, inp.Amount)
		if err != nil {
			return err
		}
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
		return fmt.Errorf("sender %s last nonce must not be zero in tx %s", signerAddr, txid)
	}
	signerState.LastNonce--
	if signerState.LastNonce != tx.Nonce {
		return fmt.Errorf("signer nonce %d does not match tx nonce %d", signerState.LastNonce, tx.Nonce)
	}

	// NOTE: The following functions must be careful not to read/write the signer state again,
	// as it is saved later and this could cause conflicts.

	// TODO: undo stake if the tx is an stake transaction
	if tx.Version == transaction.TX_VERSION_STAKE {
		stakeData := tx.Data.(*transaction.Stake)
		if stakeData.DelegateId == 0 {
			return errors.New("stake transaction has invalid delegate id")
		}
		if signerState.DelegateId != stakeData.DelegateId {
			return fmt.Errorf("stake transaction delegate id %d does not match with state %d",
				stakeData.DelegateId, signerState.DelegateId)
		}
		// ApplyUnstake is equivalent to RemoveStake
		err = bc.ApplyUnstake(txn, &transaction.Unstake{
			Amount:     stakeData.Amount,
			DelegateId: stakeData.DelegateId,
		}, signerAddr, txid, stats)
		if err != nil {
			return fmt.Errorf("could not remove stake: %w", err)
		}
		// underflow is not a big deal here, as TotalStaked/TotalUnstaked are not used in consensus (only in display)
		signerState.TotalStaked -= stakeData.Amount
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
		// ApplyStake is equivalent to RemoveUnstake
		err = bc.ApplyStake(txn, &transaction.Stake{
			Amount:     unstakeData.Amount,
			DelegateId: unstakeData.DelegateId,
		}, signerAddr, txid, stats)
		if err != nil {
			return fmt.Errorf("could not remove unstake: %w", err)
		}
		signerState.TotalUnstaked -= unstakeData.Amount
	}
	// unregister delegate if the tx is a register_delegate transaction
	if tx.Version == transaction.TX_VERSION_REGISTER_DELEGATE {
		registerData := tx.Data.(*transaction.RegisterDelegate)

		delegate, err := bc.GetDelegate(txn, registerData.Id)
		if err != nil {
			return fmt.Errorf("delegate %d does not exist: %w", registerData.Id, err)
		}

		if len(delegate.Funds) > 0 {
			return fmt.Errorf("cannot unregister delegate with active funds")
		}

		err = bc.RemoveDelegate(txn, registerData.Id)
		if err != nil {
			return fmt.Errorf("could not remove delegate: %w", err)
		}

		// TODO: restore TotalStaked and TotalUnstaked (not important, as they are only stats not used in consensus)
	}
	// revert delegate if the tx is a set_delegate transaction
	if tx.Version == transaction.TX_VERSION_SET_DELEGATE {
		setData := tx.Data.(*transaction.SetDelegate)

		if setData.DelegateId != signerState.DelegateId {
			return fmt.Errorf("invalid current delegate %d, expected %d", setData.DelegateId, signerState.DelegateId)
		}

		_, err = bc.GetDelegate(txn, setData.DelegateId)
		if err != nil {
			return fmt.Errorf("delegate not found: %w", err)
		}

		signerState.DelegateId = setData.PreviousDelegate
	}

	// save signer state
	err = bc.SetState(txn, signerAddr, signerState)
	if err != nil {
		return err
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
			err = bc.RemovePosReward(txn, blockhash, &out, txid, stats)
			if err != nil {
				return err
			}
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

	err = stats.Unstaked(out.Amount)
	if err != nil {
		return fmt.Errorf("failed to remove from stats, this should never happen: %w", err)
	}

	return bc.SetDelegate(txn, oldDelegate)
}
