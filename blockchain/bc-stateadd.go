package blockchain

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/rpc/daemonrpc"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"
)

// Note: It is up to the caller to save the stats afterwards.
// Note: In case of error, the database transaction `txn` should be reversed.
func (bc *Blockchain) ApplyTxToState(
	txn adb.Txn, tx *transaction.Transaction, signerAddr address.Address, bl *block.Block, blockhash util.Hash, stats *Stats, txid transaction.TXID,
) error {
	// check signer state
	signerState, err := bc.GetState(txn, signerAddr)
	if err != nil {
		return fmt.Errorf("failed to get signer state for transaction %s: %w", txid, err)
	}
	Log.Dev("signer state before:", signerState)

	if tx.Nonce != signerState.LastNonce+1 {
		return fmt.Errorf("transaction %s has unexpected nonce: %d, previous nonce: %d", txid,
			tx.Nonce, signerState.LastNonce)
	}

	// stake if the tx is an stake transaction
	if tx.Version == transaction.TX_VERSION_STAKE {
		// This stake data already has some prevalidation, such as amount > fee spent
		stakeData := tx.Data.(*transaction.Stake)
		if stakeData.DelegateId == 0 {
			return errors.New("stake transaction has invalid delegate id")
		}
		if signerState.DelegateId != stakeData.DelegateId {
			return fmt.Errorf("stake transaction delegate id %d does not match with state %d",
				stakeData.DelegateId, signerState.DelegateId)
		}
		err = bc.ApplyStake(txn, stakeData, signerAddr, txid, stats)
		if err != nil {
			return fmt.Errorf("could not apply stake: %w", err)
		}
		signerState.TotalStaked += stakeData.Amount
	}
	// unstake if the tx is an unstake transaction
	if tx.Version == transaction.TX_VERSION_UNSTAKE {
		// This unstake data already has some prevalidation, such as amount > fee spent
		unstakeData := tx.Data.(*transaction.Unstake)
		if unstakeData.DelegateId == 0 {
			return errors.New("unstake transaction has invalid delegate id")
		}
		if signerState.DelegateId != unstakeData.DelegateId {
			return fmt.Errorf("unstake transaction delegate id %d does not match with state %d",
				unstakeData.DelegateId, signerState.DelegateId)
		}
		err = bc.ApplyUnstake(txn, unstakeData, signerAddr, txid, stats)
		if err != nil {
			return fmt.Errorf("could not apply unstake: %w", err)
		}
		signerState.TotalUnstaked += unstakeData.Amount
	}
	// register delegate if the tx is a register_delegate transaction
	if tx.Version == transaction.TX_VERSION_REGISTER_DELEGATE {
		registerData := tx.Data.(*transaction.RegisterDelegate)

		_, err = bc.GetDelegate(txn, registerData.Id)
		if err == nil {
			return fmt.Errorf("delegate %d is already registered", registerData.Id)
		}

		Log.Debug("registering delegate", registerData.Id, "with name", strconv.Quote(string(registerData.Name)))

		bc.SetDelegate(txn, &Delegate{
			Id:    registerData.Id,
			Owner: tx.Signer,
			Name:  registerData.Name,
			Funds: make([]*daemonrpc.DelegatedFund, 0),
		})
	}
	// set delegate if the tx is a set_delegate transaction
	if tx.Version == transaction.TX_VERSION_SET_DELEGATE {
		setData := tx.Data.(*transaction.SetDelegate)

		if setData.PreviousDelegate != signerState.DelegateId {
			return fmt.Errorf("invalid previous delegate %d, expected %d", setData.PreviousDelegate, signerState.DelegateId)
		}

		prevDelegate, err := bc.GetDelegate(txn, setData.PreviousDelegate)
		if err == nil {
			for _, v := range prevDelegate.Funds {
				if v.Owner == signerAddr {
					return fmt.Errorf("all funds must be unstaked before delegate can be changed")
				}
			}
		}

		_, err = bc.GetDelegate(txn, setData.DelegateId)
		if err != nil {
			return fmt.Errorf("delegate not found: %w", err)
		}

		signerState.DelegateId = setData.DelegateId
		signerState.TotalStaked = 0
		signerState.TotalUnstaked = 0
	}

	// increase signer nonce
	signerState.LastNonce++
	err = bc.SetState(txn, signerAddr, signerState)
	if err != nil {
		return err
	}
	Log.Dev("signer state after:", signerState)

	// apply inputs
	for _, inp := range tx.Data.StateInputs(tx, signerAddr) {
		senderState, err := bc.GetState(txn, inp.Sender)
		if err != nil {
			return err
		}

		Log.Dev("sender state before:", senderState)

		if senderState.Balance < inp.Amount {
			return fmt.Errorf("transaction %s spends too much money: sender %s balance: %d, amount: %d",
				txid, inp.Sender, senderState.Balance, inp.Amount)
		}

		senderState.Balance -= inp.Amount

		err = bc.SetState(txn, inp.Sender, senderState)
		if err != nil {
			return err
		}
	}

	// add the funds to the outputs
	stateoutputs := tx.Data.StateOutputs(tx, signerAddr)
	bc.ApplyTxOutputsToState(txn, blockhash, stateoutputs, txid, stats)

	// add tx hash to sender's outgoing list
	err = bc.SetTxTopoOut(txn, txid, signerAddr, signerState.LastNonce)
	if err != nil {
		return err
	}

	// update tx height
	err = bc.SetTxHeight(txn, txid, bl.Height)
	if err != nil {
		return err
	}

	return nil
}

func (bc *Blockchain) ApplyStake(txn adb.Txn, stakeData *transaction.Stake, signerAddr address.Address, txid transaction.TXID, stats *Stats) error {
	delegate, err := bc.GetDelegate(txn, stakeData.DelegateId)
	if err != nil {
		return err
	}

	staked := false
	for _, fund := range delegate.Funds {
		if fund.Owner != signerAddr {
			continue
		}
		fund.Amount, err = util.SafeAdd(fund.Amount, stakeData.Amount)
		if err != nil {
			return err
		}
		staked = true
		break
	}
	if !staked {
		delegate.Funds = append(delegate.Funds, &daemonrpc.DelegatedFund{
			Owner:  signerAddr,
			Amount: stakeData.Amount,
		})
	}

	err = stats.Staked(stakeData.Amount)
	if err != nil {
		return fmt.Errorf("failed to add to stats, this should never happen: %w", err)
	}

	// Update the delegate in the state (note: SetDelegate also sorts the funds and checks there are no duplicate owners)
	err = bc.SetDelegate(txn, delegate)
	if err != nil {
		return fmt.Errorf("failed to set delegate, this should never happen: %w", err)
	}
	return nil
}

func (bc *Blockchain) ApplyUnstake(txn adb.Txn, unstakeData *transaction.Unstake, signerAddr address.Address, txid transaction.TXID, stats *Stats) error {
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
				txid, util.FormatCoin(unstakeData.Amount), util.FormatCoin(fund.Amount))
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
		return fmt.Errorf("failed to add to stats, this should never happen: %w", err)
	}

	// Update the delegate in the state (note: SetDelegate also sorts the funds and checks there are no duplicate owners)
	err = bc.SetDelegate(txn, delegate)
	if err != nil {
		return fmt.Errorf("failed to set delegate, this should never happen: %w", err)
	}
	return nil
}

// Note: It is up to the caller to save the stats afterwards.
func (bc *Blockchain) ApplyTxOutputsToState(txn adb.Txn, blockhash util.Hash, outs []transaction.StateOutput, txid transaction.TXID, stats *Stats) error {
	for _, out := range outs {
		recState, err := bc.GetState(txn, out.Recipient)
		if err != nil {
			Log.Debug("recipient state not previously known:", err)
			recState = &State{}
		}
		Log.Devf("recipient %s state before: %v", out.Recipient, recState)

		recState.Balance, err = util.SafeAdd(recState.Balance, out.Amount)
		if err != nil {
			return fmt.Errorf("overflow when adding balance: %v", err)
		}
		recState.LastIncoming++ // also increase recipient's LastIncoming

		Log.Devf("recipient %s state after: %v", out.Recipient, recState)

		// add tx hash to recipient's incoming list
		err = bc.SetTxTopoInc(txn, txid, out.Recipient, recState.LastIncoming)
		if err != nil {
			return fmt.Errorf("failed to set TxTopoInc: %w", err)
		}
		err = bc.SetState(txn, out.Recipient, recState)
		if err != nil {
			return fmt.Errorf("failed to set state: %w", err)
		}

		// special payout for PoS reward
		if out.Type == transaction.OUT_COINBASE_POS {
			err = bc.ApplyPosReward(txn, blockhash, &out, txid, stats)
			if err != nil {
				return fmt.Errorf("failed to apply pos reward: %w", err)
			}
		}
	}
	return nil
}
func (bc *Blockchain) ApplyPosReward(txn adb.Txn, blockhash util.Hash, out *transaction.StateOutput, txid transaction.TXID, stats *Stats) error {
	if out.ExtraData == 0 {
		return fmt.Errorf("invalid delegate id %d", out.ExtraData)
	}
	// Get and validate the delegate
	delegate, err := bc.GetDelegate(txn, out.ExtraData)
	if err != nil {
		return err
	}
	if len(delegate.Funds) == 0 {
		return fmt.Errorf("delegate has no funds")
	}
	totalStake := delegate.TotalAmount()
	totalAdded := uint64(0)
	if totalStake == 0 {
		return errors.New("delegate has no stake")
	}

	// Save the delegate history
	err = bc.SetDelegateHistory(txn, blockhash, delegate)
	if err != nil {
		return fmt.Errorf("failed to set delegate: %w", err)
	}

	// Apply the PoS reward distribution
	for _, fund := range delegate.Funds {
		// add the amount with 1% fee (that will be paid to fund owner from the roundingError)
		addAmount := uint128.From64(fund.Amount).Mul64(out.Amount).Div64(100).Mul64(99).Div64(totalStake).Lo

		Log.Debugf("adding %s to staker %s", util.FormatCoin(addAmount), fund.Owner)

		if fund.Amount+addAmount < fund.Amount {
			return errors.New("fund amount overflow")
		}

		fund.Amount += addAmount
		totalAdded += addAmount // No need to check overflow of totalAdded, as we check fund.Amount which is >=
	}
	if totalAdded > out.Amount {
		return fmt.Errorf("totalAdded %d > out.Amount %d, should never happen", totalAdded, out.Amount)
	}

	// Handle rounding error + fee, adding it to the delegate owner
	roundingError := out.Amount - totalAdded
	Log.Debugf("out.Amount: %s", util.FormatCoin(out.Amount))
	Log.Debugf("rounding errors left us with %s extra, paying it to delegate owner", util.FormatCoin(roundingError))
	Log.Debugf("delegate totalStake: %s", util.FormatCoin(totalStake))
	delegateAddr := delegate.OwnerAddress()
	ownerFound := false
	for _, v := range delegate.Funds {
		if v.Owner == delegateAddr {
			ownerFound = true
			v.Amount += roundingError
			break
		}
	}
	if !ownerFound {
		delegate.Funds = append(delegate.Funds, &daemonrpc.DelegatedFund{
			Owner:  delegateAddr,
			Amount: roundingError,
		})
	}

	// Verify the addition was correct
	if delegate.TotalAmount() != totalStake+out.Amount {
		return fmt.Errorf("staking mismatch: delegate balance %d, expected %d", delegate.TotalAmount(), totalStake+out.Amount)
	}

	err = stats.Staked(out.Amount)
	if err != nil {
		return fmt.Errorf("failed to add to stats, this should never happen: %w", err)
	}

	// Update the delegate in the state (note: SetDelegate also sorts the funds)
	err = bc.SetDelegate(txn, delegate)
	if err != nil {
		return fmt.Errorf("failed to set delegate, this should never happen: %w", err)
	}
	return nil
}
