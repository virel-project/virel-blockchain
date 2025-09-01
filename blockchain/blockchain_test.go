package blockchain_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/adb/lmdb"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/blockchain"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/logger"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"
	"github.com/virel-project/virel-blockchain/v2/wallet"
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
		[]byte{}, false)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("mnemonic seed phrase:", wall.GetMnemonic())

	err = bc.DB.Update(func(txn adb.Txn) error {
		genesis, err := bc.GetBlockByHeight(txn, 0)
		if err != nil {
			return err
		}

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

		bl.Ancestors = bl.Ancestors.AddHash(bl.Hash())
		bl.Height++
		bl.Timestamp += config.TARGET_BLOCK_TIME * 1000
		bl.Nonce++

		err = AddBlock(txn, bc, bl)
		if err != nil {
			return err
		}

		stats := bc.GetStats(txn)
		staketxs := GetStakeTxs(txn, bc, wall, stats.TopHeight)
		for _, v := range staketxs {
			hash := v.Hash()
			err = bc.AddTransaction(txn, v, hash, true)
			if err != nil {
				return err
			}
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
func GetStakeTxs(txn adb.Txn, bc *blockchain.Blockchain, w *wallet.Wallet, height uint64) []*transaction.Transaction {
	txs := make([]*transaction.Transaction, 0)

	state, err := bc.GetState(txn, w.GetAddress().Addr)
	if err != nil {
		panic(err)
	}
	w.ManualRefresh(state, height)

	tx, err := w.RegisterDelegate("test delegate")
	if err != nil {
		panic(err)
	}

	txs = append(txs, tx)

	return txs
}
