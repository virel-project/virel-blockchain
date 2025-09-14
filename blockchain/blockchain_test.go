package blockchain_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/virel-project/virel-blockchain/v3/adb"
	"github.com/virel-project/virel-blockchain/v3/adb/lmdb"
	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/block"
	"github.com/virel-project/virel-blockchain/v3/blockchain"
	"github.com/virel-project/virel-blockchain/v3/chaintype"
	"github.com/virel-project/virel-blockchain/v3/config"
	"github.com/virel-project/virel-blockchain/v3/logger"
	"github.com/virel-project/virel-blockchain/v3/transaction"
	"github.com/virel-project/virel-blockchain/v3/util"
	"github.com/virel-project/virel-blockchain/v3/util/uint128"
	"github.com/virel-project/virel-blockchain/v3/wallet"
)

func getAppTempDir() string {
	tempDir := os.TempDir()
	appTempDir := tempDir + "/vireltestdir"

	os.RemoveAll(appTempDir)

	// Create the directory with appropriate permissions (0700 for private)
	err := os.MkdirAll(appTempDir, 0700)
	if err != nil {
		panic(err)
	}

	return appTempDir
}

func SetupBc() *blockchain.Blockchain {
	log := logger.New()
	tmp := getAppTempDir()

	lmdb, err := lmdb.New(tmp+"/lmdb/", 0o700, log)
	if err != nil {
		panic(err)
	}

	bc := blockchain.New(tmp, lmdb)

	go bc.StartP2P([]string{}, config.P2P_BIND_PORT, true, true)

	blockchain.Log.SetLogLevel(100)
	return bc
}
func TestState(t *testing.T) {
	bc := SetupBc()

	wall, _, err := wallet.CreateWalletFromMnemonic(
		fmt.Sprintf("127.0.0.1:%d", config.RPC_BIND_PORT),
		"soda ladder vault wash wrestle child embark spare code plastic camera render between light deliver garment road visit",
		"examplePassword", false)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("mnemonic seed phrase:", wall.GetMnemonic())

	err = bc.DB.Update(func(txn adb.Txn) error {
		genesis, err := bc.GetBlockByHeight(txn, 0)
		if err != nil {
			return err
		}

		// add block 1
		bl := &block.Block{
			BlockHeader: block.BlockHeader{
				Version:     1,
				Height:      1,
				Timestamp:   uint64(100),
				Nonce:       355,
				NonceExtra:  [16]byte{},
				OtherChains: []block.HashingID{},
				Recipient:   wall.GetAddress().Addr,
				Ancestors:   genesis.Ancestors.AddHash(genesis.Hash()),
			},
			Difficulty: uint128.From64(config.MIN_DIFFICULTY),
		}
		bl.CumulativeDiff = uint128.From64(bl.Height * config.MIN_DIFFICULTY)
		err = AddBlock(txn, bc, bl)
		if err != nil {
			return err
		}

		stats := bc.GetStats(txn)
		staketxs := GetStakeTxs(txn, bc, wall, stats.TopHeight)
		staketxids := make([]transaction.TXID, len(staketxs))
		for i, v := range staketxs {
			hash := v.Hash()
			staketxids[i] = hash
			err = bc.AddTransaction(txn, v, hash, true, stats.TopHeight+1)
			if err != nil {
				return err
			}
			t.Logf("tx has fee %s", util.FormatCoin(v.Fee))
		}

		// add block 2
		bl.Ancestors = bl.Ancestors.AddHash(bl.Hash())
		bl.Height++
		bl.Timestamp += config.TARGET_BLOCK_TIME * 1000
		bl.Transactions = staketxids
		bl.Nonce++
		err = AddBlock(txn, bc, bl)
		if err != nil {
			return err
		}

		// add block 3
		bl.Ancestors = bl.Ancestors.AddHash(bl.Hash())
		bl.Height++
		bl.Timestamp += config.TARGET_BLOCK_TIME * 1000
		bl.Nonce++
		bl.NextDelegateId = delegate_id
		bl.Transactions = []transaction.TXID{}
		err = AddBlock(txn, bc, bl)
		if err != nil {
			return err
		}

		stats = bc.GetStats(txn)

		t.Logf("stats: %s", stats)
		bl3 := *bl

		// add block 4
		bl.Ancestors = bl.Ancestors.AddHash(bl.Hash())
		bl.Height++
		bl.Timestamp += config.TARGET_BLOCK_TIME * 1000
		bl.Nonce++
		bl.NextDelegateId = delegate_id
		err = AddBlock(txn, bc, bl)
		if err != nil {
			return err
		}

		stats = bc.GetStats(txn)

		t.Logf("stats: %s", stats)

		// add block 4
		bl.Ancestors = bl3.Ancestors.AddHash(bl3.Hash())
		bl.Height = bl3.Height + 1
		bl.Timestamp = bl3.Timestamp + config.TARGET_BLOCK_TIME*1000
		bl.Nonce++
		bl.NextDelegateId = delegate_id
		err = AddBlock(txn, bc, bl)
		if err != nil {
			return err
		}

		// add block 5
		bl.Ancestors = bl.Ancestors.AddHash(bl.Hash())
		bl.Height++
		bl.Timestamp += config.TARGET_BLOCK_TIME * 1000
		bl.Nonce++
		bl.NextDelegateId = delegate_id
		err = AddBlock(txn, bc, bl)
		if err != nil {
			return err
		}

		// add block 6
		bl.Ancestors = bl.Ancestors.AddHash(bl.Hash())
		bl.Height++
		bl.Timestamp += config.TARGET_BLOCK_TIME * 1000
		bl.Nonce++
		bl.DelegateId = delegate_id
		bl.NextDelegateId = delegate_id
		bl.StakeSignature, err = wall.SignBlockHash(bl.BlockStakedHash())
		if err != nil {
			return err
		}
		bl.CumulativeDiff = bl.CumulativeDiff.Add64(1)
		err = AddBlock(txn, bc, bl)
		if err != nil {
			return err
		}

		stats = bc.GetStats(txn)

		t.Logf("stats: %s", stats)

		const pow_reward = config.BLOCK_REWARD * 0.5
		const pos_reward = config.BLOCK_REWARD * 0.4
		const dev_reward = config.BLOCK_REWARD * 0.1

		const burnrewards = 5 * (pos_reward + pow_reward*0.25) / config.COIN

		err = PrintState(txn, bc, map[string]float64{
			"burnaddress": config.REGISTER_DELEGATE_BURN/config.COIN + burnrewards + 1.6611,
			"delegate0":   1 + pos_reward*2,
			// genesis address: first block + 10% of all blocks
			"vo3yexhnu89af4aai83uou17dupb79c3gxng1q": (config.BLOCK_REWARD+float64(bl.Height)*dev_reward)/config.COIN + 0.3164,
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func AddBlock(txn adb.Txn, bc *blockchain.Blockchain, bl *block.Block) error {
	err := bc.PrevalidateBlock(bl, []*transaction.Transaction{})
	if err != nil {
		return err
	}

	err = bc.AddBlock(txn, bl, bl.Hash())
	if err != nil {
		return err
	}

	stats := bc.GetStats(txn)
	_, err = bc.CheckReorgs(txn, stats)
	if err != nil {
		return err
	}
	err = bc.SetStats(txn, stats)
	if err != nil {
		return err
	}

	return nil
}

const delegate_id = 1

func GetStakeTxs(txn adb.Txn, bc *blockchain.Blockchain, w *wallet.Wallet, height uint64) []*transaction.Transaction {
	txs := make([]*transaction.Transaction, 0)

	state, err := bc.GetState(txn, w.GetAddress().Addr)
	if err != nil {
		panic(err)
	}
	w.ManualRefresh(state, height)
	tx, err := w.RegisterDelegate("test delegate", delegate_id)
	if err != nil {
		panic(err)
	}
	fmt.Println(tx)
	txs = append(txs, tx)

	state.LastNonce++
	w.ManualRefresh(state, height)
	tx, err = w.SetDelegate(delegate_id, state.DelegateId)
	if err != nil {
		panic(err)
	}
	fmt.Println(tx)
	txs = append(txs, tx)

	state.LastNonce++
	w.ManualRefresh(state, height)
	tx, err = w.Stake(delegate_id, config.COIN, 0)
	if err != nil {
		panic(err)
	}
	fmt.Println(tx)
	txs = append(txs, tx)

	return txs
}
func PrintState(txn adb.Txn, bc *blockchain.Blockchain, check map[string]float64) error {
	sum := uint64(0)
	err := txn.ForEach(bc.Index.State, func(k, v []byte) error {
		addr := address.Address(k)
		state := &chaintype.State{}

		err := state.Deserialize(v)
		if err != nil {
			return err
		}

		fmt.Printf("address: %s balance: %s last nonce: %d\n", addr, util.FormatCoin(state.Balance), state.LastNonce)

		c := check[addr.String()]
		if c != 0 {
			bl := float64(state.Balance) / config.COIN
			if bl > c*1.001 || bl < c*0.999 {
				return fmt.Errorf("address %s balance %s does not match expected %f", addr, util.FormatCoin(state.Balance), c)
			}
		}

		sum += state.Balance

		return nil
	})
	return err
}
