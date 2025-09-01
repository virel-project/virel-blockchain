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

func (bc *Blockchain) RemoveTxFromState(
	txn adb.Txn, tx *transaction.Transaction, signerAddr address.Address, bl *block.Block, stats *Stats, txid transaction.TXID,
) error {
	// reset tx height
	err := bc.SetTxHeight(txn, txid, 0)
	if err != nil {
		return err
	}

	// remove the funds from the outputs
	stateoutputs := tx.Data.StateOutputs(tx, signerAddr)
	bc.RemoveTxOutputsFromState(txn, stateoutputs, txid, stats)

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
		err = bc.RemoveUnstake(txn, unstakeData, signerAddr, txid, stats)
		if err != nil {
			return fmt.Errorf("could not remove unstake: %w", err)
		}
	}

	return nil
}

func (bc *Blockchain) RemoveUnstake(txn adb.Txn, unstakeData *transaction.Unstake, signerAddr address.Address, txid transaction.TXID, stats *Stats) error {
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

func (bc *Blockchain) RemoveTxOutputsFromState(txn adb.Txn, outs []transaction.StateOutput, txid transaction.TXID, stats *Stats) error {
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

			totalStake := delegate.TotalAmount() - out.Amount
			totalSubtracted := uint64(0)

			if totalStake == 0 {
				return fmt.Errorf("delegate has no stake")
			}

			for i, fund := range delegate.Funds {
				// Calculate the amount that was added to each fund
				subtractAmount := fund.Amount * out.Amount * 99 / 100 / (totalStake + out.Amount)

				// Safety check to prevent underflow
				if fund.Amount < subtractAmount {
					return fmt.Errorf("cannot subtract more than available in fund")
				}

				delegate.Funds[i].Amount -= subtractAmount
				totalSubtracted += subtractAmount
			}

			// Handle rounding error (reverse of what was done in ApplyBlockToState)
			roundingError := out.Amount - totalSubtracted
			Log.Debugf("rounding errors left the delegate owner with %s extra", util.FormatCoin(roundingError))
			delegateAddr := delegate.OwnerAddress()
			for _, v := range delegate.Funds {
				if v.Owner == delegateAddr {
					if v.Amount < roundingError {
						return fmt.Errorf("cannot subtract rounding error from first fund")
					}
					v.Amount += roundingError
					break
				}
			}

			// Verify the subtraction was correct
			if delegate.TotalAmount() != totalStake {
				return fmt.Errorf("staking mismatch after reversal: delegate balance %d, expected %d",
					delegate.TotalAmount(), totalStake)
			}

			stats.Unstaked(out.Amount)

			// Update the delegate in the state (note: SetDelegate also sorts the funds)
			err = bc.SetDelegate(txn, delegate)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
