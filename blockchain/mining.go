package blockchain

import (
	"crypto/rand"
	"fmt"
	"math"
	"time"

	"github.com/virel-project/go-randomvirel"
	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/stratum"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"
)

func (bc *Blockchain) StartMining(addr address.Address) {
	bc.MergesMut.Lock()
	if bc.Mining {
		Log.Err("cannot start mining, already mining")
		bc.MergesMut.Unlock()
		return
	}
	bc.Mining = true
	bc.MergesMut.Unlock()

	for {
		bc.MergesMut.Lock()
		if !bc.Mining {
			bc.MergesMut.Unlock()
			return
		}
		bc.MergesMut.Unlock()
		bc.MineBlock(addr)
	}
}

func (bc *Blockchain) GetBlockTemplate(txn adb.Txn, addr address.Address) (*block.Block, uint64, error) {
	stats := bc.GetStats(txn)
	prevBl, err := bc.GetBlock(txn, stats.TopHash)
	if err != nil {
		return nil, 0, err
	}

	height := stats.TopHeight + 1

	var version uint8 = 0
	if height >= config.HARDFORK_V3_HEIGHT {
		version = 1
	}

	bl := &block.Block{
		BlockHeader: block.BlockHeader{
			Version:    version,
			Height:     height,
			Timestamp:  max(util.Time()+1, prevBl.Timestamp), // we ensure that the timestamp is always sequential
			Recipient:  addr,
			Ancestors:  prevBl.Ancestors.AddHash(stats.TopHash),
			SideBlocks: make([]block.Commitment, 0),
		},
		Difficulty:   uint128.From64(config.MIN_DIFFICULTY),
		Transactions: []transaction.TXID{},
	}
	_, err = rand.Read(bl.NonceExtra[:])
	if err != nil {
		return nil, 0, err
	}

	bl.Difficulty, err = bc.GetNextDifficulty(txn, prevBl)
	if err != nil {
		return nil, 0, err
	}

	for _, v := range stats.Tips {
		if len(bl.SideBlocks) == config.MAX_SIDE_BLOCKS {
			Log.Debug("max side blocks reached, breaking")
			break
		}
		if v.Height >= bl.Height || v.Height < bl.Height-config.MINIDAG_ANCESTORS-1 {
			continue
		}
		// check if the tip can actually be used as side block, and it if does, use it!
		err := bc.DB.View(func(txn adb.Txn) error {
			tip, err := bc.GetBlock(txn, v.Hash)
			if err != nil {
				return err
			}
			// As a special rule, to prevent possible slowdowns during seed hash changes, the blocks must
			// have the same seed hash
			if block.GetSeedhashId(tip.Timestamp) != block.GetSeedhashId(bl.Timestamp) {
				Log.Debug("tip is not applicable because the seedhash is different")
				return nil
			}
			if tip.Difficulty.Cmp(bl.Difficulty.Mul64(2).Div64(3)) < 0 {
				Log.Debug("tip is not applicable because its difficulty is too small")
				return nil
			}

			common := bl.Ancestors.FindCommon(tip.Ancestors)
			if common < 0 {
				return nil
			}

			side := tip.Commitment()

			// check that the tip hasn't been already included
			if side.Equals(prevBl.Commitment()) {
				Log.Debug("side block was already included (1)")
				return nil
			}
			for _, v := range prevBl.SideBlocks { // first check in the prevBl, since we already obtained it
				if side.Equals(v) {
					Log.Debug("side block was already included (2)")
					return nil
				}
			}
			if prevBl.Height > 0 {
				for _, anc := range bl.Ancestors[1:] { // then check in previous ancestors
					ancBl, err := bc.GetBlock(txn, anc)
					if err != nil {
						Log.Err(err)
						return err
					}
					if side.Equals(ancBl.Commitment()) {
						Log.Debug("side block was already included (3)")
						return nil
					}
					for _, v := range ancBl.SideBlocks {
						if side.Equals(v) {
							Log.Debug("side block was already included (4)")
							return nil
						}
					}
					if ancBl.Height == 0 {
						break
					}
				}
			}
			Log.Debug("found valid side block")
			bl.SideBlocks = append(bl.SideBlocks, side)

			return nil
		})
		if err != nil {
			Log.Warn(err)
			continue
		}
	}

	// TODO: sort mempool transactions by Fee Per Kilobyte, to prioritize the transactions with higher fee
	// possibly also take in account transaction age in the sorting algorithm
	mem := bc.GetMempool(txn)
	var totsize uint64 = 0
	validEntries := make([]*MempoolEntry, 0, len(mem.Entries))
	for _, v := range mem.Entries {
		if totsize+v.Size > config.MAX_BLOCK_SIZE {
			break
		}

		memtx, _, err := bc.GetTx(txn, v.TXID, bl.Height)
		if err != nil {
			Log.Err(err)
			continue
		}
		err = bc.validateMempoolTx(txn, memtx, v.TXID, validEntries, bl.Height)
		if err != nil {
			Log.Warn("GetBlockTemplate: validateMempoolTx error:", err)
			continue
		}

		totsize += v.Size
		bl.Transactions = append(bl.Transactions, v.TXID)
		validEntries = append(validEntries, v)
	}
	if len(mem.Entries) != len(validEntries) {
		mem.Entries = validEntries
		err = bc.SetMempool(txn, mem)
		if err != nil {
			Log.Warn(err)
		}
	}

	bl.CumulativeDiff = stats.CumulativeDiff.Add(bl.ContributionToCumulativeDiff())

	if bl.Version > 0 {
		stakesig, err := bc.GetStakeSig(txn, bl.BlockStakedHash())
		if err != nil {
			Log.Debug("block template: no stake sig found:", err)
		} else {
			if stakesig.Hash != bl.BlockStakedHash() {
				Log.Warn("stakeSig.Hash and block staked hash do not match:", stakesig.Hash, bl.BlockStakedHash())
			} else {
				bl.StakeSignature = stakesig.Signature
				bl.DelegateId = stakesig.DelegateId
				bl.CumulativeDiff = stats.CumulativeDiff.Add(bl.ContributionToCumulativeDiff())
			}
		}
		if bl.DelegateId == 0 {
			stakedbl, err := bc.GetBlock(txn, bl.BlockStakedHash())
			if err != nil {
				Log.Debug(err)
			} else {
				bl.DelegateId = stakedbl.NextDelegateId
			}
		}
	}

	var min_diff uint64 = bl.Difficulty.Lo

	if config.IS_MASTERCHAIN {
		// add merge mining templates
		bc.MergesMut.Lock()
		for _, v := range bc.Merges {
			v.RLock()
			if v.Difficulty == 0 {
				// skip merges that don't have first job yet
				continue
			}
			bl.OtherChains = append(bl.OtherChains, v.HashingID)
			if v.Difficulty < min_diff {
				Log.Debug("min_diff reduces from", min_diff, "to", v.Difficulty)
				min_diff = v.Difficulty
			}
			v.RUnlock()
		}
		bc.MergesMut.Unlock()

		bl.SortOtherChains()
	}

	// calculate NextDelegateId based on the previous block hash
	if bl.Version > 0 {
		nextdelegate, err := bc.GetStaker(txn, bl.PrevHash(), bc.GetStats(txn))
		if err != nil {
			return nil, 0, err
		}
		bl.NextDelegateId = nextdelegate.Id
	}

	return bl, min_diff, nil
}

func (bc *Blockchain) MineBlock(addr address.Address) {
	var bl *block.Block
	var min_diff uint64
	err := bc.DB.View(func(tx adb.Txn) (err error) {
		bl, min_diff, err = bc.GetBlockTemplate(tx, addr)
		return
	})
	if err != nil {
		Log.Fatal("failed to get block template:", err)
	}

	bc.findBlockSolution(bl, uint128.From64(min_diff))
}

func (bc *Blockchain) findBlockSolution(bl *block.Block, min_diff Uint128) {

	seed := bl.Commitment().MiningBlob().GetSeed()

	t := time.Now()
	var h uint64 = 0

	for {
		bc.MergesMut.Lock()
		if bc.mergesUpdated {
			bc.mergesUpdated = false
			bc.MergesMut.Unlock()
			return
		}
		if !bc.Mining {
			bc.MergesMut.Unlock()
			return
		}
		bc.MergesMut.Unlock()

		mb := bl.Commitment().MiningBlob()
		if bl.Nonce&0xff == 0 {
			bl.Timestamp = util.Time()
			seed = mb.GetSeed()
		}

		pow := randomvirel.PowHash(seed, mb.Serialize())
		powHash := [16]byte(pow[16:])
		if block.ValidPowHash(powHash, min_diff) {
			_, err := bc.blockFound(bl, powHash)
			if err != nil {
				Log.Err(err)
			}
			return
		}

		bl.Nonce++
		h++
		now := time.Now()
		if now.Sub(t).Seconds() > 2 {
			Log.Info("hashrate:", math.Round(float64(h)/time.Since(t).Seconds()))
			h = 0
			t = now
		}
	}
}

func (bc *Blockchain) blockFound(bl *block.Block, powHash [16]byte) ([]stratum.FoundBlockInfo, error) {
	foundInfo := []stratum.FoundBlockInfo{}

	hash := bl.Hash()

	success := false
	if config.IS_MASTERCHAIN {
		var morefound []stratum.FoundBlockInfo
		morefound, success = bc.submitMergeMinedBlock(bl, powHash)
		if success {
			Log.Infof("found merge block %x with diff %s PoW %x", hash, bl.Difficulty.String(), powHash)
		}
		foundInfo = append(foundInfo, morefound...)
	}
	if bl.ValidPowHash(powHash) {
		foundInfo = append(foundInfo, stratum.FoundBlockInfo{
			Hash:       hash[:],
			Height:     bl.Height,
			NetworkID:  config.NETWORK_ID,
			Difficulty: bl.Difficulty,
			Ok:         true,
		})
		Log.Infof("Found block %x with diff %s sideblocks %v", hash, bl.Difficulty.String(), bl.SideBlocks)
		err := bc.PrevalidateBlock(bl, nil)
		if err != nil {
			Log.Warn(err)
			return nil, err
		}
		go bc.BroadcastBlock(bl)
		err = bc.DB.Update(func(tx adb.Txn) error {
			return bc.AddBlock(tx, bl, hash)
		})
		if err != nil {
			return nil, err
		}
		return foundInfo, nil
	}

	if !success {
		return nil, fmt.Errorf(
			"block does not match minimum difficulty requirements, hash %x diff %s",
			powHash, hashToDiff(powHash),
		)
	}
	return foundInfo, nil
}

func hashToDiff(hash [16]byte) Uint128 {
	return Uint128{Hi: 0xffffffffffffffff, Lo: 0xffffffffffffffff}.Div(uint128.FromBytes(hash[:]))
}
