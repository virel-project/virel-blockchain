package blockchain

import (
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"runtime"
	"sync"
	"time"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/logger"
	"github.com/virel-project/virel-blockchain/v2/p2p"
	"github.com/virel-project/virel-blockchain/v2/p2p/packet"
	"github.com/virel-project/virel-blockchain/v2/stratum/stratumsrv"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"
)

var Log = logger.New()

type Uint128 = uint128.Uint128

// Blockchain represents a Blockchain structure, for storing transactions
type Blockchain struct {
	DB      adb.DB
	Index   Index
	DataDir string

	P2P     *p2p.P2P
	Stratum *stratumsrv.Server

	shutdownInfo shutdownInfo

	Mining bool // locked by MergesMut

	Merges        []*mergestratum
	MergesMut     util.RWMutex
	mergesUpdated bool
	lastJob       time.Time

	Validator *Validator

	BlockQueue *BlockQueue

	SyncHeight uint64  // top height seen from remote nodes
	SyncDiff   Uint128 // top cumulative diff seen from remote nodes
	SyncMut    util.RWMutex
}
type Index struct {
	Info            adb.Index
	Block           adb.Index
	Topo            adb.Index
	State           adb.Index
	Tx              adb.Index
	InTx            adb.Index
	OutTx           adb.Index
	Delegate        adb.Index // Delegate Id -> Delegate
	StakeSig        adb.Index // Block hash -> Stake signature
	DelegateHistory adb.Index // Block hash -> Delegate (before applying block reward)
}

func (bc *Blockchain) IsShuttingDown() bool {
	bc.shutdownInfo.RLock()
	defer bc.shutdownInfo.RUnlock()

	return bc.shutdownInfo.ShuttingDown
}

func New(dataDir string, db adb.DB) *Blockchain {
	bc := &Blockchain{
		Stratum: &stratumsrv.Server{
			NewConnections: make(chan *stratumsrv.Conn),
		},
	}

	bc.DataDir = dataDir
	bc.DB = db

	bc.Index = Index{
		Info:     bc.DB.Index("info"),
		Block:    bc.DB.Index("block"),
		Topo:     bc.DB.Index("topo"),
		State:    bc.DB.Index("state"),
		Tx:       bc.DB.Index("tx"),
		InTx:     bc.DB.Index("intx"),
		OutTx:    bc.DB.Index("outtx"),
		Delegate: bc.DB.Index("delegate"),
	}

	bc.Validator = bc.NewValidator(runtime.NumCPU())

	// add genesis block if it doesn't exist
	bc.addGenesis()

	var stats *Stats
	var mempool *Mempool
	bc.DB.View(func(tx adb.Txn) error {
		stats = bc.GetStats(tx)
		mempool = bc.GetMempool(tx)
		return nil
	})

	Log.Info("Started blockchain")
	Log.Infof("Height: %d", stats.TopHeight)
	Log.Infof("Cumulative diff: %.3fk\n", stats.CumulativeDiff.Float64()/1000)
	Log.Infof("Top hash: %x", stats.TopHash)
	Log.Debugf("Tips: %x", stats.Tips)
	Log.Debugf("Orphans: %x", stats.Orphans)
	Log.Debugf("Mempool: %d transactions", len(mempool.Entries))

	bc.SyncDiff = stats.CumulativeDiff
	bc.SyncHeight = stats.TopHeight

	bc.BlockQueue = NewBlockQueue(bc)

	return bc
}

func (bc *Blockchain) Synchronize() {
	Log.Debug("Synchronization thread started")
	for {
		if bc.IsShuttingDown() {
			Log.Info("Synchronization thread stopped")
			return
		}

		var stats *Stats
		bc.DB.View(func(tx adb.Txn) error {
			stats = bc.GetStats(tx)
			return nil
		})

		bc.BlockQueue.Update(func(qt *QueueTx) {
			bc.fillQueue(qt, stats.TopHeight)

			reqbls := []packet.PacketBlockRequest{}
			for range config.PARALLEL_BLOCKS_DOWNLOAD {
				reqbl := qt.RequestableBlock()
				if reqbl == nil {
					break
				}
				if reqbl.Height != 0 && reqbl.Height < stats.TopHeight {
					qt.BlockRequested(reqbl.Height)
					continue
				}
				lastIdx := len(reqbls) - 1
				if len(reqbls) > 0 && reqbls[lastIdx].Height != 0 && reqbls[lastIdx].Height+uint64(reqbls[lastIdx].Count) == reqbl.Height-1 {
					reqbls[0].Count++
				} else {
					reqbls = append(reqbls, packet.PacketBlockRequest{
						Height: reqbl.Height,
						Hash:   reqbl.Hash,
						Count:  0,
					})
				}
			}
			for _, v := range reqbls {
				Log.Debugf("requesting block height %d count %d hash %x", v.Height, v.Count, v.Hash)
			}

			if len(reqbls) == 0 {
				return
			}

			go func() {
				// Find a valid peer
				var peer *p2p.Connection
				func() {
					bc.P2P.RLock()
					defer bc.P2P.RUnlock()

					// fix a rare crash
					if len(bc.P2P.Connections) == 0 {
						return
					}

					// take a random connection as the first peer to try
					peernum := mrand.IntN(len(bc.P2P.Connections))

					keys := make([]string, 0, len(bc.P2P.Connections))
					for k := range bc.P2P.Connections {
						keys = append(keys, k)
					}

					for i := 0; i < len(keys); i++ {
						n := (i + peernum) % len(keys)
						conn := bc.P2P.Connections[keys[n]]

						found := false
						conn.PeerData(func(d *p2p.PeerData) {
							if len(reqbls) == 0 {
								peer = conn
								found = true
								return
							}

							last := reqbls[len(reqbls)-1]

							if last.Height == 0 || d.Stats.Height >= last.Height {
								peer = conn
								found = true
							}
						})
						if found {
							break
						}
					}
				}()

				if peer == nil {
					Log.Debug("no peer to query blocks")
					return
				}
				// Request the blocks to the selected peer
				for _, reqbl := range reqbls {
					peer.SendPacket(&p2p.Packet{
						Type: packet.BLOCK_REQUEST,
						Data: reqbl.Serialize(),
					})
				}
			}()
		})

		time.Sleep(50 * time.Millisecond)
	}
}

// TODO: clean up expired queue

// Blockchain MUST be locked before calling this
func (bc *Blockchain) fillQueue(qt *QueueTx, topHeight uint64) {
	bc.SyncMut.RLock()
	syncHeight := bc.SyncHeight
	bc.SyncMut.RUnlock()

	if qt.Length() < QUEUE_SIZE {
		if syncHeight > topHeight {
			n := qt.Length()
			for i := topHeight + 1; i <= syncHeight; i++ {
				if n > QUEUE_SIZE {
					break
				}
				n++
				qt.SetBlock(NewQueuedBlock(i, [32]byte{}), false)
			}
		}
	}
}

type shutdownInfo struct {
	ShuttingDown bool
	sync.RWMutex
}

func (bc *Blockchain) Close() {
	bc.shutdownInfo.Lock()
	bc.shutdownInfo.ShuttingDown = true
	bc.shutdownInfo.Unlock()
	Log.Info("Stopping integrated miner if started")
	bc.MergesMut.Lock()
	bc.Mining = false
	bc.MergesMut.Unlock()
	Log.Info("Stopping validator")
	bc.Validator.Close()
	Log.Info("Shutting down P2P server")
	bc.P2P.Close()
	Log.Info("Saving block download queue")
	bc.BlockQueue.Lock()
	bc.BlockQueue.Save()
	bc.BlockQueue.Unlock()
	Log.Info("Closing database")
	bc.DB.Close()
	Log.Info("Virel daemon shutdown complete. Bye!")
}

func (bc *Blockchain) addGenesis() {
	if util.Time() < config.GENESIS_TIMESTAMP {
		Log.Fatal("genesis block in future of", (config.GENESIS_TIMESTAMP-int64(util.Time()))/1000, "seconds")
	}

	genesis := &block.Block{
		BlockHeader: block.BlockHeader{
			Height:     0,
			Version:    0,
			Timestamp:  config.GENESIS_TIMESTAMP,
			Nonce:      0x1337,
			NonceExtra: [16]byte{},
			Recipient:  address.GenesisAddress,
			Ancestors:  block.Ancestors{},
		},
		Difficulty:     uint128.From64(1),
		CumulativeDiff: uint128.From64(1),
		Transactions:   []transaction.TXID{},
	}

	hash := genesis.Hash()

	Log.Debugf("genesis block hash is %x", hash)

	err := bc.DB.Update(func(tx adb.Txn) error {
		bl, err := bc.GetBlock(tx, hash)
		if err != nil {
			Log.Debug("genesis block is not in chain:", err)
			err := bc.insertBlockMain(tx, genesis)
			if err != nil {
				return err
			}
			err = bc.SetStats(tx, &Stats{
				TopHash:        hash,
				TopHeight:      0,
				CumulativeDiff: genesis.Difficulty,
			})
			if err != nil {
				return err
			}
			err = bc.SetMempool(tx, &Mempool{
				Entries: make([]*MempoolEntry, 0),
			})
			if err != nil {
				return err
			}
			err = bc.ApplyBlockToState(tx, genesis, hash)
			if err != nil {
				return err
			}
		} else {
			if bl == nil {
				return errors.New("bl is nil")
			}
			Log.Debug("genesis is already in chain:", bl.String())
		}
		return nil
	})
	if err != nil {
		Log.Fatal(err)
	}
}

// checkBlock validates things like height, diff, etc. for a block. It doesn't validate PoW (that's done by
// bl.Prevalidate()) or transactions.
func (bc *Blockchain) checkBlock(tx adb.Txn, bl, prevBl *block.Block) error {
	// validate difficulty
	expectDiff, err := bc.GetNextDifficulty(tx, prevBl)
	if err != nil {
		err = fmt.Errorf("failed to get difficulty: %w", err)
		return err
	}
	if !bl.Difficulty.Equals(expectDiff) {
		return fmt.Errorf("block has invalid diff: %s, expected: %s", bl.Difficulty.String(),
			expectDiff.String())
	}

	// check that height is correct
	if bl.Height != prevBl.Height+1 {
		return fmt.Errorf("block has invalid height: %d, previous: %d", bl.Height, prevBl.Height)
	}

	// check that timestamp is strictly greater than previous block timestamp
	if prevBl.Timestamp > bl.Timestamp {
		return fmt.Errorf("block has timestamp that's older than previous block: %d<=%d", bl.Timestamp,
			prevBl.Timestamp)
	}

	// validate block's SideBlocks
	// since SideBlocks's Ancestors are derived from height, we don't have to check them here
	for _, side := range bl.SideBlocks {

		// check ancestors
		// TODO PRIORITY: audit this! It's of critical importance!
		var heightDiff int = -1 //
		for ancid, anc := range side.Ancestors {
			if heightDiff == -1 { // common not found
				// scan if we can find the ancestor
				for vid, v := range bl.Ancestors {
					if vid >= ancid && v == anc {
						heightDiff = vid - ancid
						Log.Debug("found ancestor at height difference:", heightDiff)
					}
				}
			} else { // common found, verify that subsequent blocks match
				if ancid+heightDiff >= len(bl.Ancestors) {
					break
				}
				if anc != bl.Ancestors[ancid+heightDiff] {
					return errors.New("subsequent block isn't valid")
				}
			}
		}
		if heightDiff == -1 {
			return fmt.Errorf("common block not found")
		}

		// check that the side block hasn't been already included
		if side.Equals(prevBl.Commitment()) {
			return fmt.Errorf("side block was already included (1)")
		}
		for _, v := range prevBl.SideBlocks { // first check in the prevBl, since we already obtained it
			if side.Equals(v) {
				return fmt.Errorf("side block was already included (2)")
			}
		}
		if prevBl.Height > 0 {
			for _, anc := range bl.Ancestors[1:] { // then check in previous ancestors
				ancBl, err := bc.GetBlock(tx, anc)
				if err != nil {
					Log.Err(err)
					return err
				}
				if side.Equals(ancBl.Commitment()) {
					return fmt.Errorf("side block was already included (3)")
				}
				for _, v := range ancBl.SideBlocks {
					if side.Equals(v) {
						return fmt.Errorf("side block was already included (4)")
					}
				}
				if ancBl.Height == 0 {
					break
				}
			}
		}
	}

	newCumDiff := prevBl.CumulativeDiff.Add(bl.ContributionToCumulativeDiff())
	if !bl.CumulativeDiff.Equals(newCumDiff) {
		return fmt.Errorf("block has invalid cumulative diff: %s, expected: %s", bl.CumulativeDiff,
			newCumDiff)
	}

	// Verify if the signature is valid. If it is blank, this block is not considered staked.
	if bl.Version > 0 && bl.StakeSignature != bitcrypto.BlankSignature {
		stakedhash := bl.BlockStakedHash()
		stats := bc.GetStats(tx)

		// signature is always invalid if the network has nothing at stake
		if stats.StakedAmount == 0 {
			return fmt.Errorf("invalid stake signature: the network has not staked any coins")
		}

		delegate, err := bc.GetStaker(tx, stakedhash, stats)
		if err != nil {
			return fmt.Errorf("failed to get delegate: %w", err)
		}

		if delegate.Id != bl.DelegateId {
			return fmt.Errorf("block DelegateId is invalid: expected %d, got %d", delegate.Id, bl.DelegateId)
		}

		if !bitcrypto.VerifySignature(delegate.Owner, append(config.STAKE_SIGN_PREFIX, stakedhash[:]...), bl.StakeSignature) {
			return errors.New("invalid stake signature")
		}
	}

	return nil
}

// AddBlock attempts adding a block to the blockchain.
// Block should be already prevalidated.
// If the block doesn't fit in the mainchain, it is either added to an altchain or orphaned.
// Blockchain MUST be locked before calling this
func (bc *Blockchain) AddBlock(tx adb.Txn, bl *block.Block, hash util.Hash) error {
	// check if block is duplicate
	_, err := bc.GetBlock(tx, hash)
	if err == nil {
		return fmt.Errorf("duplicate block %x height %d", hash, bl.Height)
	}

	prevHash := bl.PrevHash()

	// check if block is orphaned
	prevBl, err := bc.GetBlock(tx, prevHash)
	if err != nil {
		err := bc.addOrphanBlock(tx, bl, hash, false)
		if err != nil {
			Log.Err(err)
			return err
		}
		return nil
	}

	// check if parent block is orphaned
	stats := bc.GetStats(tx)
	if stats.Orphans[prevHash] != nil {
		// this block's parent is orphaned; add this block as an orphan
		err := bc.addOrphanBlock(tx, bl, hash, true)
		if err != nil {
			Log.Err(err)
			return err
		}
		return nil
	}

	err = bc.checkBlock(tx, bl, prevBl)
	if err != nil {
		Log.Warn("block is invalid:", err)
		return err
	}

	// add block to chain
	var isMainchain = prevHash == stats.TopHash
	if isMainchain {
		// remove block from queue
		bc.removeFromQueue(hash, bl.Height)

		err = bc.addMainchainBlock(tx, bl, hash)
	} else {
		err = bc.addAltchainBlock(tx, bl, hash)
	}
	if err != nil {
		Log.Err(err)
		return err
	}
	err = bc.checkDeorphanage(tx, bl, hash)
	if err != nil {
		Log.Err(err)
		return err
	}

	return nil
}

func (bc *Blockchain) removeFromQueue(hash [32]byte, height uint64) {
	bc.BlockQueue.Update(func(qt *QueueTx) {
		qt.RemoveBlock(height, hash)
	})
}
func (bc *Blockchain) queuedBlockDownloaded(hash [32]byte, height uint64) {
	bc.BlockQueue.Update(func(qt *QueueTx) {
		qt.BlockDownloaded(height, hash)
	})
}

// addOrphanBlock should only be called by the addBlock method
// use parentKnown = true if this block has a known parent which is orphaned
// Blockchain MUST be locked before calling this
func (bc *Blockchain) addOrphanBlock(txn adb.Txn, bl *block.Block, hash [32]byte, parentKnown bool) error {
	Log.Infof("Adding orphan block %d %x diff: %s sides: %d parent known: %v", bl.Height, hash,
		bl.Difficulty, len(bl.SideBlocks), parentKnown)
	stats := bc.GetStats(txn)

	if stats.Orphans[hash] != nil {
		return errors.New("Orphan already exists! This should NEVER happen")
	}

	orphan := &Orphan{
		Expires:  time.Now().Add(time.Hour).Unix(), // orphan blocks expire after 1 hour
		Hash:     hash,
		PrevHash: bl.PrevHash(),
	}

	// add orphan prevhash to queued blocks, if it is not known already
	if !parentKnown {
		bc.BlockQueue.Update(func(qt *QueueTx) {
			qt.SetBlock(NewQueuedBlock(0, bl.PrevHash()), false)
		})
	}

	// TODO: clean up expired orphans

	// insert orphan
	stats.Orphans[hash] = orphan
	bc.setStatsNoBroadcast(txn, stats)

	return bc.insertBlock(txn, bl, hash)
}

// addAltchainBlock should only be called by the addBlock method
// Blockchain MUST be locked before calling this
func (bc *Blockchain) addAltchainBlock(txn adb.Txn, bl *block.Block, hash [32]byte) error {
	Log.Infof("Adding block as alternative on height: %d hash: %x diff: %s", bl.Height, hash, bl.Difficulty)
	stats := bc.GetStats(txn)

	// check if the block extends one of the tips
	extendTip := stats.Tips[bl.PrevHash()]

	if extendTip != nil {
		// block extends one of the tips, update that tip
		Log.Debugf("block %x extends tip %x", hash, extendTip.Hash)
		extendTip.Hash = hash
		extendTip.Height++
		extendTip.CumulativeDiff = bl.CumulativeDiff
	} else {
		// if the block doesn't extend tips, then it's creating a new tip
		Log.Debugf("new tip: %x", hash)
		stats.Tips[hash] = &AltchainTip{
			Hash:           hash,
			Height:         bl.Height,
			CumulativeDiff: bl.CumulativeDiff,
		}
	}

	// insert block and save stats
	err := bc.insertBlock(txn, bl, hash)
	if err != nil {
		Log.Err(err)
		return err
	}
	// broadcasting stats isn't necessary, altchain blocks don't affect our tophash
	bc.setStatsNoBroadcast(txn, stats)

	// check for reorgs
	bc.CheckReorgs(txn, stats)

	if bl.Height+config.MINIDAG_ANCESTORS >= stats.TopHeight {
		go bc.NewStratumJob(false)
	}

	return nil
}

// returns true if a reorg has happened
func (bc *Blockchain) CheckReorgs(txn adb.Txn, stats *Stats) (bool, error) {
	type hashInfo struct {
		Hash  [32]byte
		Block *block.Block
	}

	// Check if a reorg is needed
	var altDiff = stats.CumulativeDiff
	var altHash = stats.TopHash
	var altHeight = stats.TopHeight
	for _, v := range stats.Tips {
		if v.CumulativeDiff.Cmp(altDiff) > 0 {
			altDiff = v.CumulativeDiff
			altHash = v.Hash
			altHeight = v.Height
		}
	}
	// If the reorg is not needed, then return
	if altHash == stats.TopHash {
		Log.Debug("reorg not needed")
		return false, nil
	}
	Log.Infof("Reorg needed: height %d -> %d, hash %x, cumulative diff %s -> %s",
		stats.TopHeight, altHeight, altHash, stats.CumulativeDiff.String(), altDiff.String())

	// reorganize the chain
	err := func() error {
		// step 1: iterate the altchain blocks in reverse order to find out the common block with mainchain
		commonBlockHash := altHash
		commonBlock, err := bc.GetBlock(txn, commonBlockHash)
		if err != nil {
			Log.Err(err)
			return err
		}

		hashes := []hashInfo{
			{
				Hash:  commonBlockHash,
				Block: commonBlock,
			},
		} // hashes holds the altchain blocks, used in step 3

		// TODO: we can optimize this loop by scanning all of the block's known ancestors
		for {
			commonBlockHash = commonBlock.PrevHash()
			commonBlock, err = bc.GetBlock(txn, commonBlockHash)
			if err != nil {
				err := fmt.Errorf("reorg step 1: failed to get common block %x: %v", commonBlockHash, err)
				return err
			}
			Log.Debugf("reorg step 1: scanning altchain block %d %x", commonBlock.Height, commonBlockHash)

			if commonBlock.Height == 0 {
				err = errors.New("could not find common block")
				Log.Err(err)
				return err
			}

			topohash, err := bc.GetTopo(txn, commonBlock.Height)
			// a block doesn't exist in mainchain at this height, just print the error and go on
			if err != nil {
				Log.Debug("a block doesn't exist in mainchain at this height (probably fine), err:", err)
			}

			if topohash == commonBlockHash {
				Log.Debugf("stopping just before block common: %x", commonBlockHash)
				break
			}

			hashes = append(hashes, hashInfo{
				Hash:  commonBlockHash,
				Block: commonBlock,
			})
		}

		// step 2: iterate the mainchain blocks in reverse order until common block to reverse the state
		// changes and remove the topoheight data (only do this if TopHash is not the common block's hash,
		// which can happen after a deorphanage)
		if stats.TopHash != commonBlockHash {
			nHash := stats.TopHash
			n, err := bc.GetBlock(txn, nHash)
			if err != nil {
				Log.Err(err)
				return err
			}

			if n.Hash() == commonBlockHash {
				Log.Debugf("reorg step 2 not needed")
			} else {
				for {
					if nHash == commonBlockHash {
						Log.Debugf("reorg step 2 done")
						break
					}
					if n.Height == 0 {
						err := fmt.Errorf("reorg: Block has height 0! Could not find common hash %x; nHash %x",
							commonBlockHash, nHash)
						Log.Err(err)
						return err
					}

					n, err = bc.GetBlock(txn, nHash)
					if err != nil {
						err := fmt.Errorf("failed to get block %x: %v", nHash, err)
						Log.Err(err)
						return err
					}

					Log.Debugf("reorg step 2: reversing changes of block %d %x", n.Height, nHash)

					// delete this block's topo
					heightBin := make([]byte, 8)
					binary.LittleEndian.PutUint64(heightBin, n.Height)
					err := txn.Del(bc.Index.Topo, heightBin)
					if err != nil {
						Log.Err(err)
						return err
					}

					// remove block from state
					err = bc.RemoveBlockFromState(txn, n, nHash)
					if err != nil {
						Log.Err(err)
						return err
					}

					nHash = n.PrevHash()
				}
			}
		}

		// step 3: iterate altchain blocks starting from common block to validate and apply them to the state
		// and to the topo; if any of these blocks is invalid, delete it and undo the reorg

		Log.Devf("hashes: %x", hashes)

		for i := len(hashes) - 1; i >= 0; i-- {
			Log.Devf("reorg step 3: setting topo: %d (height: %d) %x", i, hashes[i].Block.Height,
				hashes[i].Hash)

			// set this block's topo
			heightBin := make([]byte, 8)
			binary.LittleEndian.PutUint64(heightBin, hashes[i].Block.Height)
			err := txn.Put(bc.Index.Topo, heightBin, hashes[i].Hash[:])
			if err != nil {
				Log.Err(err)
				return err
			}

			bl := hashes[i].Block

			// set the block's cumulative difficulty
			prevBl, err := bc.GetBlock(txn, bl.PrevHash())
			if err != nil {
				Log.Err(err)
				return err
			}

			err = bc.checkBlock(txn, bl, prevBl)
			if err != nil {
				Log.Warn("reorg invalid block:", err)
				return err
			}

			err = bc.ApplyBlockToState(txn, bl, hashes[i].Hash)
			if err != nil {
				Log.Err(err)
				return err
			}

			bc.BlockQueue.Update(func(qt *QueueTx) {
				qt.RemoveBlockByHeight(bl.Height)
			})
		}

		// step 4: update the stats
		Log.Devf("starting reorg step 4")

		stats = bc.GetStats(txn)

		// add the old mainchain as an altchain tip
		delete(stats.Tips, altHash)
		stats.Tips[stats.TopHash] = &AltchainTip{
			Hash:           stats.TopHash,
			Height:         stats.TopHeight,
			CumulativeDiff: stats.CumulativeDiff,
		}

		// set the new mainchain
		stats.TopHash = altHash
		stats.CumulativeDiff = altDiff
		stats.TopHeight = altHeight

		txn.Put(bc.Index.Info, []byte("stats"), stats.Serialize())

		Log.Infof("Reorganize success, new height: %d hash: %x cumulative diff: %s", stats.TopHeight,
			stats.TopHash, stats.CumulativeDiff)
		return nil
	}()

	if err != nil {
		Log.Err("Reorg failed:", err)
		return false, err
	}
	return true, nil
}

// addMainchainBlock should only be called by the addBlock method
// Blockchain MUST be locked before calling this
func (bc *Blockchain) addMainchainBlock(tx adb.Txn, bl *block.Block, hash [32]byte) error {
	err := bc.ApplyBlockToState(tx, bl, hash)
	if err != nil {
		Log.Warn("block is invalid, not adding to mainchain:", err)
		return err
	}

	Log.Infof("Adding mainchain block %d %x diff: %s sides: %d", bl.Height, hash, bl.Difficulty, len(bl.SideBlocks))
	stats := bc.GetStats(tx)

	stats.TopHash = hash
	stats.TopHeight = bl.Height
	stats.CumulativeDiff = bl.CumulativeDiff
	err = bc.SetStats(tx, stats)
	if err != nil {
		return err
	}

	// add block to mainchain and update stats
	err = bc.insertBlockMain(tx, bl)
	if err != nil {
		Log.Err(err)
		return err
	}

	Log.Debugf("done adding block %x to mainchain", hash)

	return nil
}

// Validates a block, and then adds it to the state
func (bc *Blockchain) ApplyBlockToState(txn adb.Txn, bl *block.Block, blockhash [32]byte) error {
	stats := bc.GetStats(txn)
	defer bc.SetStats(txn, stats)

	// remove transactions from mempool
	pool := bc.GetMempool(txn)
	for _, t := range bl.Transactions {
		pool.DeleteEntry(t)
	}
	err := bc.SetMempool(txn, pool)
	if err != nil {
		return err
	}

	var totalFee uint64 = 0

	// validate and apply transactions
	for _, v := range bl.Transactions {
		tx, _, err := bc.GetTx(txn, v)
		if err != nil {
			return fmt.Errorf("transaction is not in state: %w", err)
		}
		signerAddr := address.FromPubKey(tx.Signer)

		Log.Debugf("Applying transaction %s to mainchain", v)

		err = bc.ApplyTxToState(txn, tx, signerAddr, bl, blockhash, stats, v)
		if err != nil {
			Log.Err(err)
			return err
		}

		// apply tx to total fee
		prev := totalFee
		totalFee += tx.Fee
		if totalFee < prev {
			return errors.New("reward tx fee overflow in block")
		}
	}

	// add block reward to coinbase transaction
	totalReward := bl.Reward() + totalFee
	if totalReward < bl.Reward() {
		return errors.New("reward overflow in block")
	}

	coinbaseOuts := bl.CoinbaseTransaction(totalReward)
	outs := make([]transaction.StateOutput, len(coinbaseOuts))
	for i, v := range coinbaseOuts {
		outs[i] = transaction.StateOutput{
			Type:      v.Type,
			Amount:    v.Amount,
			Recipient: v.Recipient,
			PaymentId: 0,
			ExtraData: v.DelegateId,
		}
	}

	err = bc.ApplyTxOutputsToState(txn, blockhash, outs, blockhash, stats)
	if err != nil {
		Log.Err(err)
		return err
	}

	// update some stats
	bc.SyncMut.Lock()
	if bc.SyncDiff.Cmp(bl.CumulativeDiff) < 0 {
		bc.SyncHeight = bl.Height
		bc.SyncDiff = bl.CumulativeDiff
	}
	bc.SyncMut.Unlock()

	return nil
}

// Reverses the transaction of a block from the blockchain state
func (bc *Blockchain) RemoveBlockFromState(txn adb.Txn, bl *block.Block, blhash [32]byte) error {
	stats := bc.GetStats(txn)
	defer bc.SetStats(txn, stats)

	type txCache struct {
		Hash [32]byte
		Tx   *transaction.Transaction
	}
	txs := make([]txCache, len(bl.Transactions))

	// iterate transactions to find tx fee sum for coinbase transaction
	var totalFee uint64
	if len(bl.Transactions) > 0 {
		memp := bc.GetMempool(txn)
		for i, v := range bl.Transactions {
			tx, _, err := bc.GetTx(txn, v)
			if err != nil {
				Log.Err(err)
				return err
			}
			totalFee += tx.Fee
			txs[i] = txCache{
				Hash: v,
				Tx:   tx,
			}
			// add removed transactions back to mempool
			if memp.GetEntry(v) == nil {
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
				memp.Entries = append(memp.Entries, &MempoolEntry{
					TXID:      v,
					TxVersion: tx.Version,
					Size:      tx.GetVirtualSize(),
					Fee:       tx.Fee,
					Expires:   time.Now().Add(config.MEMPOOL_EXPIRATION).Unix(),
					Signer:    signerAddr,
					Inputs:    tx.Data.StateInputs(tx, signerAddr),
					Outputs:   out,
				})
			}
		}
		err := bc.SetMempool(txn, memp)
		if err != nil {
			Log.Err(err)
			return err
		}
	}

	// undo coinbase transaction

	totalReward := bl.Reward() + totalFee
	coinbaseOutputs := bl.CoinbaseTransaction(totalReward)

	for _, out := range coinbaseOutputs {
		// undo coinbase transaction
		coinbaseState, err := bc.GetState(txn, out.Recipient)
		if err != nil {
			err := fmt.Errorf("coinbase reward account unknown: %s", err)
			Log.Err(err)
			return err
		}
		if coinbaseState.Balance < out.Amount {
			err := fmt.Errorf("balance of coinbase account is too small: balance %d, block reward %d",
				coinbaseState.Balance, out.Amount)
			Log.Err(err)
			return err
		}
		if coinbaseState.LastIncoming == 0 {
			err = fmt.Errorf("coinbase %s LastIncoming must not be zero in block %x", out.Recipient, blhash)
			Log.Err(err)
			return err
		}
		coinbaseState.Balance -= out.Amount
		coinbaseState.LastIncoming--
		err = bc.SetState(txn, out.Recipient, coinbaseState)
		if err != nil {
			Log.Err(err)
			return err
		}
		// removing coinbase transaction from incoming tx list is not necessary, since it's never read and later overwritten

	}

	// remove transactions in reverse order
	for i := len(txs) - 1; i >= 0; i-- {
		tx := txs[i].Tx
		txhash := txs[i].Hash

		Log.Devf("removing transaction %x (index %d) from state", txhash, i)

		signerAddr := address.FromPubKey(tx.Signer)

		// decrease recipient balance and LastIncoming
		stateoutputs := tx.Data.StateOutputs(tx, signerAddr)
		for _, out := range stateoutputs {
			recState, err := bc.GetState(txn, out.Recipient)
			if err != nil {
				Log.Err(err)
				return err
			}
			if recState.Balance < out.Amount {
				err := fmt.Errorf("recipient balance is smaller than output amount: %d < %d",
					recState.Balance, out.Amount)
				if err != nil {
					Log.Err(err)
					return err
				}
			}
			if recState.LastIncoming == 0 {
				err = fmt.Errorf("recipient %s LastIncoming must not be zero in tx %x", out.Recipient, txhash)
				Log.Err(err)
				return err
			}
			recState.Balance -= out.Amount
			recState.LastIncoming--
			err = bc.SetState(txn, out.Recipient, recState)
			if err != nil {
				Log.Err(err)
				return err
			}
		}

		// increase senders balances
		for _, inp := range tx.Data.StateInputs(tx, signerAddr) {
			senderState, err := bc.GetState(txn, inp.Sender)
			if err != nil {
				Log.Err(err)
				return err
			}

			senderState.Balance += inp.Amount
			err = bc.SetState(txn, inp.Sender, senderState)
			if err != nil {
				Log.Err(err)
				return err
			}
		}

		// decrease signer nonce
		signerState, err := bc.GetState(txn, signerAddr)
		if err != nil {
			Log.Err(err)
			return err
		}
		if signerState.LastNonce == 0 {
			err = fmt.Errorf("sender %s last nonce must not be zero in tx %x", signerAddr, txhash)
			Log.Err(err)
			return err
		}
		signerState.LastNonce--
		err = bc.SetState(txn, signerAddr, signerState)
		if err != nil {
			Log.Err(err)
			return err
		}

		// set tx height to zero
		err = bc.SetTxHeight(txn, txhash, 0)
		if err != nil {
			Log.Err(err)
			return err
		}

	}

	return nil
}

func (bc *Blockchain) GetState(tx adb.Txn, addr address.Address) (*State, error) {
	s := &State{}
	bin := tx.Get(bc.Index.State, addr[:])
	if bin == nil {
		return s, fmt.Errorf("address %s not in state", addr)
	}
	err := s.Deserialize(bin)
	return s, err
}
func (bc *Blockchain) SetState(tx adb.Txn, addr address.Address, state *State) (err error) {
	return tx.Put(bc.Index.State, addr[:], state.Serialize())
}

func (bc *Blockchain) CreateCheckpoints(tx adb.Txn, maxHeight, interval uint64) ([]byte, error) {
	s := binary.NewSer(make([]byte, maxHeight/interval*32))
	s.AddUint32(uint32(interval))
	for height := interval; height <= maxHeight; height += interval {
		bl, err := bc.GetTopo(tx, height)
		if err != nil {
			Log.Err(err)
			return nil, err
		}
		Log.Devf("Adding block %d %x to checkpoints", height, bl)
		s.AddFixedByteArray(bl[:])
	}
	return s.Output(), nil
}

// Blockchain MUST be locked before calling this
func (bc *Blockchain) checkDeorphanage(tx adb.Txn, bl *block.Block, hash [32]byte) error {
	Log.Debugf("checkDeorphanage %x", hash)
	stats := bc.GetStats(tx)

	// no need to remove block from queue, it's removed by parent of this function

	// recursively check for deorphans
	err := bc.deorphanBlock(tx, bl, hash, stats)
	if err != nil {
		Log.Err(err)
		return err
	}

	// finally, save stats
	err = bc.SetStats(tx, stats)
	if err != nil {
		return err
	}

	// now that blocks were deorphaned, there might be a reorg
	reorg, err := bc.CheckReorgs(tx, stats)
	if err != nil {
		Log.Err(err)
		return err
	}
	if reorg {
		stats = bc.GetStats(tx)
		bc.cleanupTips(tx, stats)
		err = bc.SetStats(tx, stats)
		if err != nil {
			return err
		}
	}

	return nil
}

// Blockchain MUST be locked before calling this
func (bc *Blockchain) cleanupTips(tx adb.Txn, stats *Stats) {
	Log.Debug("cleaning up tips")
	for i, tip := range stats.Tips {
		topo, err := bc.GetTopo(tx, tip.Height)
		if err != nil {
			Log.Debugf("cleanupTips error is %v; this is probably fine", err)
			continue
		}
		if topo == tip.Hash {
			Log.Debugf("cleanupTips: tip %x is included in mainchain, discarding it", tip.Hash)
			delete(stats.Tips, i)
		}
	}
}

// recursive function which finds all the orphans that are children of the given hash, and creates altchain
// don't forget to save stats later, as this function doesn't do that
func (bc *Blockchain) deorphanBlock(tx adb.Txn, prev *block.Block, prevHash [32]byte, stats *Stats) error {
	Log.Debugf("deorphanBlock hash %x", prevHash)

	for i, v := range stats.Orphans {
		if v.PrevHash == prevHash {
			Log.Debugf("deorphanBlock: %x is deorphaning %x", prevHash, v.Hash)
			bl, err := bc.GetBlock(tx, v.Hash)
			h2 := v.Hash
			if err != nil {
				Log.Err(err)
				return err
			}

			// Here we don't fully validate the block, as we don't know the current state. Instead we only
			// update the cumulative difficulty, as it's needed for the tips
			cdiff := prev.CumulativeDiff.Add(bl.ContributionToCumulativeDiff())

			if !cdiff.Equals(bl.CumulativeDiff) {
				Log.Devf("deorphanBlock: block cumulative difficulty updated: %s -> %s", bl.CumulativeDiff,
					cdiff)
				bl.CumulativeDiff = cdiff
				bc.insertBlock(tx, bl, h2)
			}

			// remove this block from orphans
			delete(stats.Orphans, i)

			// remove bl's tip
			delete(stats.Tips, prevHash)
			// add bl2 to tips
			stats.Tips[h2] = &AltchainTip{
				Hash:           h2,
				Height:         bl.Height,
				CumulativeDiff: bl.CumulativeDiff,
			}

			// recall this function to find bl2's children
			bc.deorphanBlock(tx, bl, h2, stats)
		}
	}

	return nil
}

// Blockchain MUST be RLocked before calling this
func (bc *Blockchain) GetStats(tx adb.Txn) *Stats {
	d := tx.Get(bc.Index.Info, []byte("stats"))

	if len(d) == 0 {
		Log.Fatal("stats are empty")
	}

	s, err := DeserializeStats(d)
	if err != nil {
		Log.Fatal(err)
	}

	return s
}

// Blockchain MUST be locked before calling this
func (bc *Blockchain) SetStats(tx adb.Txn, s *Stats) error {
	if s.TopHeight != 0 {
		go bc.SendStats(s)
	}
	return bc.setStatsNoBroadcast(tx, s)
}

// Blockchain MUST be locked before calling this
func (bc *Blockchain) setStatsNoBroadcast(tx adb.Txn, s *Stats) error {
	err := tx.Put(bc.Index.Info, []byte("stats"), s.Serialize())
	return err
}

// Blockchain MUST be RLocked before calling this
func (bc *Blockchain) GetMempool(tx adb.Txn) *Mempool {
	s, err := DeserializeMempool(tx.Get(bc.Index.Info, []byte("mempool")))
	if err != nil {
		Log.Fatal(err)
	}
	return s
}

// Blockchain MUST be locked before calling this
func (bc *Blockchain) SetMempool(tx adb.Txn, s *Mempool) error {
	return tx.Put(bc.Index.Info, []byte("mempool"), s.Serialize())
}

// insertBlockMain inserts a block to the blockchain, updating topoheight and removing its transactions from
// mempool (if applicable).
// This should be only called if you are sure that the block extends mainchain.
// Blockchain MUST be locked before calling this
func (bc *Blockchain) insertBlockMain(tx adb.Txn, bl *block.Block) error {
	hash := bl.Hash()

	defer func() {
		go bc.NewStratumJob(true)
	}()

	// add block data
	err := tx.Put(bc.Index.Block, hash[:], bl.Serialize())
	if err != nil {
		return err
	}

	// add block topo
	heightBin := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBin, bl.Height)
	return tx.Put(bc.Index.Topo, heightBin, hash[:])
}

// insertBlock inserts a block to the blockchain, without updating topoheight.
// Blockchain MUST be locked before calling this
func (bc *Blockchain) insertBlock(tx adb.Txn, bl *block.Block, hash [32]byte) error {
	// add block data
	err := tx.Put(bc.Index.Block, hash[:], bl.Serialize())
	if err != nil {
		Log.Err(err)
		return err
	}
	return nil
}

// GetBlock returns the block given its hash
// Blockchain MUST be RLocked before calling this
func (bc *Blockchain) GetBlock(tx adb.Txn, hash [32]byte) (*block.Block, error) {
	bl := &block.Block{}
	// read block data
	blbin := tx.Get(bc.Index.Block, hash[:])
	if len(blbin) == 0 {
		return bl, fmt.Errorf("block %x not found", hash)
	}
	err := bl.Deserialize(blbin)
	return bl, err
}

func (bc *Blockchain) GetTopo(tx adb.Txn, height uint64) ([32]byte, error) {
	var blHash [32]byte
	heightBin := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBin, height)
	topoHash := tx.Get(bc.Index.Topo, heightBin)
	if len(topoHash) != 32 {
		return blHash, fmt.Errorf("unknown block at height %d", height)
	}
	blHash = [32]byte(topoHash)
	return blHash, nil
}

func (bc *Blockchain) GetBlockByHeight(tx adb.Txn, height uint64) (*block.Block, error) {
	hash, err := bc.GetTopo(tx, height)
	if err != nil {
		return nil, fmt.Errorf("failed to get topo: %w", err)
	}
	return bc.GetBlock(tx, hash)
}

func (bc *Blockchain) StartP2P(peers []string, port uint16, private, exclusive bool) {
	p2p.Log = Log
	bc.P2P = p2p.Start(peers, bc.DataDir)
	bc.P2P.Exclusive = exclusive

	go bc.P2P.StartClients(private)
	go bc.pinger()
	go bc.incomingP2P()
	go bc.newConnections()
	go bc.Synchronize()
	go bc.P2P.ListenServer(port, private)
}

func (bc *Blockchain) GetSupply(tx adb.Txn) uint64 {
	var sum uint64 = 0
	err := tx.ForEach(bc.Index.State, func(k, v []byte) error {
		state := &State{}
		err := state.Deserialize(v)
		if err != nil {
			Log.Warn(address.Address(k), err)
		}
		sum += state.Balance
		return nil
	})
	if err != nil {
		Log.Err(err)
	}
	return sum
}
func (bc *Blockchain) CheckSupply(tx adb.Txn) {
	sum := bc.GetSupply(tx)
	supply := block.GetSupplyAtHeight(bc.GetStats(tx).TopHeight)
	if sum != supply {
		err := fmt.Errorf("invalid supply %d, expected %d", sum, supply)
		Log.Fatal(err)
	}
	Log.Debug("CheckSupply: supply is correct:", sum)
}

func (bc *Blockchain) SetTxTopoInc(tx adb.Txn, txid [32]byte, addr address.Address, incid uint64) error {
	incbin := addr[:]
	incbin = binary.AppendUvarint(incbin, incid)
	return tx.Put(bc.Index.InTx, incbin, txid[:])
}
func (bc *Blockchain) SetTxTopoOut(tx adb.Txn, txid [32]byte, addr address.Address, outid uint64) error {
	outbin := addr[:]
	outbin = binary.AppendUvarint(outbin, outid)
	return tx.Put(bc.Index.OutTx, outbin, txid[:])
}

func (bc *Blockchain) GetTxTopoInc(tx adb.Txn, addr address.Address, incid uint64) ([32]byte, error) {
	incbin := addr[:]
	incbin = binary.AppendUvarint(incbin, incid)
	bin := tx.Get(bc.Index.InTx, incbin)
	if len(bin) != 32 {
		return [32]byte{}, errors.New("unknown tx topo inc")
	}
	return [32]byte(bin), nil
}
func (bc *Blockchain) GetTxTopoOut(tx adb.Txn, addr address.Address, outid uint64) ([32]byte, error) {
	outbin := addr[:]
	outbin = binary.AppendUvarint(outbin, outid)
	bin := tx.Get(bc.Index.OutTx, outbin)
	if len(bin) != 32 {
		return [32]byte{}, errors.New("unknown tx topo out")
	}
	return [32]byte(bin), nil
}
