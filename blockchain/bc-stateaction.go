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

// Note: It is up to the caller to save the stats afterwards.
func (bc *Blockchain) ApplyTxToState(
	txn adb.Txn, tx *transaction.Transaction, signerAddr address.Address, bl *block.Block, stats *Stats, txid transaction.TXID,
) error {
	// check signer state
	signerState, err := bc.GetState(txn, signerAddr)
	if err != nil {
		Log.Err(err)
		return err
	}
	Log.Dev("signer state before:", signerState)

	if tx.Nonce != signerState.LastNonce+1 {
		return fmt.Errorf("transaction %s has unexpected nonce: %d, previous nonce: %d", txid,
			tx.Nonce, signerState.LastNonce)
	}

	// unstake if the tx is an unstake transaction
	if tx.Version == transaction.TX_VERSION_UNSTAKE {
		// This unstake data already has some prevalidation:
		// - amount > fee spent
		unstakeData := tx.Data.(*transaction.Unstake)
		if unstakeData.DelegateId == 0 {
			return errors.New("unstake transaction has invalid delegate id")
		}

		if signerState.DelegateId != unstakeData.DelegateId {
			return fmt.Errorf("unstake transaction delegate id %d does not match with state %d",
				unstakeData.DelegateId, signerState.DelegateId)
		}

		delegate, err := bc.GetDelegate(txn, unstakeData.DelegateId)
		if err != nil {
			return err
		}

		unstaked := false
		for i, fund := range delegate.Funds {
			if fund.Owner != signerAddr {
				continue
			}

			if fund.Amount < unstakeData.Amount {
				return fmt.Errorf("transaction %s trying to unstake %s, more than available balance %s",
					txid, util.FormatCoin(unstakeData.Amount), util.FormatCoin(unstakeData.Amount))
			}
			fund.Amount -= unstakeData.Amount

			if fund.Amount == 0 {
				delegate.Funds = append(delegate.Funds[:i], delegate.Funds[i+1:]...)
			}
			unstaked = true
			break
		}
		if !unstaked {
			return fmt.Errorf("transaction %s nothing to unstake", txid)
		}

		err = stats.Unstaked(unstakeData.Amount)
		if err != nil {
			Log.Err("this should never happen:", err)
			return err
		}

		err = bc.SetDelegate(txn, delegate)
		if err != nil {
			Log.Err("this should never happen:", err)
			return err
		}
	}

	// apply signer state
	signerState.LastNonce++
	err = bc.SetState(txn, signerAddr, signerState)
	if err != nil {
		Log.Err(err)
		return err
	}
	Log.Dev("signer state after:", signerState)

	// apply inputs state
	for _, inp := range tx.Data.StateInputs(tx, signerAddr) {
		senderState, err := bc.GetState(txn, inp.Sender)
		if err != nil {
			Log.Err(err)
			return err
		}

		Log.Dev("sender state before:", senderState)

		if senderState.Balance < inp.Amount {
			err = fmt.Errorf("transaction %s spends too much money: sender %s balance: %d, amount: %d",
				txid, inp.Sender, senderState.Balance, inp.Amount)
			Log.Warn(err)
			return err
		}

		senderState.Balance -= inp.Amount

		err = bc.SetState(txn, inp.Sender, senderState)
		if err != nil {
			return err
		}
	}

	// add the funds to recipient
	stateoutputs := tx.Data.StateOutputs(tx, signerAddr)

	bc.ApplyTxOutputsToState(txn, stateoutputs, txid, stats)

	// add tx hash to sender's outgoing list
	err = bc.SetTxTopoOut(txn, txid, signerAddr, signerState.LastNonce)
	if err != nil {
		Log.Err(err)
		return err
	}
	// update tx height
	err = bc.SetTxHeight(txn, txid, bl.Height)
	if err != nil {
		Log.Err(err)
		return err
	}

	return nil
}

// Note: It is up to the caller to save the stats afterwards.
func (bc *Blockchain) ApplyTxOutputsToState(txn adb.Txn, outs []transaction.StateOutput, txid transaction.TXID, stats *Stats) error {
	for _, out := range outs {
		recState, err := bc.GetState(txn, out.Recipient)
		if err != nil {
			Log.Debug("recipient state not previously known:", err)
			recState = &State{}
		}
		Log.Devf("recipient %s state before: %txid", out.Recipient, recState)

		recState.Balance += out.Amount
		recState.LastIncoming++ // also increase recipient's LastIncoming

		Log.Devf("recipient %s state after: %txid", out.Recipient, recState)

		if out.PaymentId != 0 {

		}

		// add tx hash to recipient's incoming list
		err = bc.SetTxTopoInc(txn, txid, out.Recipient, recState.LastIncoming)
		if err != nil {
			Log.Err(err)
			return err
		}
		err = bc.SetState(txn, out.Recipient, recState)
		if err != nil {
			Log.Err(err)
			return err
		}

		// special payout for PoS reward
		if out.Type == transaction.OUT_COINBASE_POS {
			if out.ExtraData == 0 {
				err = fmt.Errorf("invalid delegate id %d", out.ExtraData)
				Log.Err(err)
				return err
			}
			delegate, err := bc.GetDelegate(txn, out.ExtraData)
			if err != nil {
				Log.Err(err)
				return err
			}

			if len(delegate.Funds) == 0 {
				err = fmt.Errorf("delegate has no funds")
				return err
			}

			totalStake := delegate.TotalAmount()
			totalAdded := uint64(0)

			if totalStake == 0 {
				return errors.New("delegate has no stake")
			}

			for _, fund := range delegate.Funds {
				addAmount := fund.Amount * out.Amount / totalStake

				Log.Debugf("adding %s to staker %s", util.FormatCoin(addAmount), fund.Owner)

				if fund.Amount+addAmount < fund.Amount {
					return errors.New("fund amount overflow")
				}

				fund.Amount += addAmount
				totalAdded += addAmount
			}

			roundingError := out.Amount - totalAdded
			Log.Debugf("rounding errors left us with %s extra, paying it to delegate owner", util.FormatCoin(roundingError))

			delegate.Funds[0].Amount += roundingError

			if delegate.TotalAmount() != totalStake+out.Amount {
				err = fmt.Errorf("staking mismatch: delegate balance %d, expected %d", delegate.TotalAmount(), totalStake+out.Amount)
				Log.Err(err)
				return err
			}

			err = stats.Staked(out.Amount)
			if err != nil {
				Log.Err("this should never happen:", err)
				return err
			}

			err = bc.SetDelegate(txn, delegate)
			if err != nil {
				Log.Err(err)
				return err
			}
		}
	}

	return nil
}
