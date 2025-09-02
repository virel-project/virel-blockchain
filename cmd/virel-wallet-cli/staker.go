package main

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/rpc/daemonrpc"
	"github.com/virel-project/virel-blockchain/v2/wallet"
)

func staker(w *wallet.Wallet, delegateAddress address.Address) {
	var lastHeight uint64
	for {
		time.Sleep(time.Second)

		err := w.Refresh()
		if err != nil {
			continue
		}

		height := w.GetHeight()
		if lastHeight < height {

			Log.Debug("staker new height:", height)

			for h := lastHeight + 1; h <= height; h++ {
				err = checkStakeHeight(w, h, delegateAddress)
				if err != nil {
					Log.Warn(err)
				}
			}

			lastHeight = height
		}
	}
}

func checkStakeHeight(w *wallet.Wallet, height uint64, delegateAddress address.Address) error {
	bl, err := w.Rpc().GetBlockByHeight(daemonrpc.GetBlockByHeightRequest{
		Height: height,
	})
	if err != nil {
		return err
	}
	if !delegateAddress.IsDelegate() {
		return fmt.Errorf("address %s is not delegate", delegateAddress)
	}
	delegateid := delegateAddress.DecodeDelegateId()

	Log.Debug("block nextdelegate:", bl.NextDelegate)

	if bl.NextDelegate == delegateAddress {
		Log.Info("Staking block", bl.Hash)
		blhash, err := hex.DecodeString(bl.Hash)
		if err != nil {
			return err
		}
		if len(blhash) != 32 {
			return fmt.Errorf("block hash has invalid length")
		}
		signature, err := w.SignBlockHash([32]byte(blhash))
		if err != nil {
			return err
		}

		res, err := w.Rpc().SubmitStakeSignature(daemonrpc.SubmitStakeSignatureRequest{
			DelegateId: delegateid,
			Hash:       blhash,
			Signature:  signature[:],
		})
		if err != nil {
			return err
		}
		if !res.Accepted {
			return fmt.Errorf("stake signature not accepted: %s", res.ErrorMessage)
		}
	}

	return nil
}
