package blockchain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/util"
)

const QUEUE_SIZE = config.PARALLEL_BLOCKS_DOWNLOAD * 20

const downloaded_expire = 5
const rerequest_time = 5

func NewQueuedBlock(height uint64, hash [32]byte) *QueuedBlock {
	return &QueuedBlock{
		Height:  height,
		Hash:    hash,
		Expires: time.Now().Add(1 * time.Minute).Unix(),
	}
}

type QueuedBlock struct {
	Height      uint64
	Hash        [32]byte
	Expires     int64 // expiration time (UNIX seconds)
	LastRequest int64 // when was the block last requested (UNIX seconds)
}

type BlockQueue struct {
	bc     *Blockchain
	blocks []*QueuedBlock

	util.RWMutex
}

type QueueTx struct {
	bq *BlockQueue
}

func NewBlockQueue(bc *Blockchain) *BlockQueue {
	bq := &BlockQueue{
		bc:     bc,
		blocks: make([]*QueuedBlock, 0, QUEUE_SIZE),
	}
	err := bq.load()
	if err != nil {
		Log.Warn("blockqueue loading failed:", err)
	}
	return bq
}

func (bq *BlockQueue) Update(fn func(qt *QueueTx)) {
	qt := QueueTx{
		bq: bq,
	}
	bq.Lock()
	fn(&qt)
	bq.Unlock()
}

func (qt *QueueTx) RequestableBlock() *QueuedBlock {
	t := time.Now().Unix()
	for _, v := range qt.bq.blocks {
		if t-v.LastRequest > rerequest_time {
			v.LastRequest = t
			return v
		}
	}
	return nil
}
func (qt *QueueTx) RemoveBlockByHash(hash [32]byte) {
	for _, v := range qt.bq.blocks {
		if v.Hash == hash {
			v.Expires = 0
		}
	}
	qt.bq.cleanup()
}
func (qt *QueueTx) RemoveBlockByHeight(height uint64) {
	for _, v := range qt.bq.blocks {
		if v.Height == height {
			v.Expires = 0
		}
	}
	qt.bq.cleanup()
}
func (qt *QueueTx) RemoveBlock(height uint64, hash [32]byte) {
	for _, v := range qt.bq.blocks {
		if v.Height == height || v.Hash == hash {
			v.Expires = 0
		}
	}
	qt.bq.cleanup()
}
func (qt *QueueTx) PurgeHeightBlocks() {
	for _, v := range qt.bq.blocks {
		if v.Height != 0 {
			v.Expires = 0
		}
	}
	qt.bq.cleanup()
}

// BlockDownloaded is used when a block has been downloaded but it's not in mainchain yet, so we cannot
// remove it immediately, as that would eventually trigger a redownload.
func (qt *QueueTx) BlockDownloaded(height uint64, hash [32]byte) {
	t := time.Now().Unix()
	for _, v := range qt.bq.blocks {
		if (height != 0 && v.Height == height) || v.Hash == hash {
			v.Expires = t + downloaded_expire
			v.LastRequest = t + downloaded_expire
		}
	}
}

// BlockDownloaded is used when a block is in mainchain, so we can remove it immediately.
func (qt *QueueTx) BlockAdded(height uint64) {
	for _, v := range qt.bq.blocks {
		if v.Height <= height {
			v.Expires = 0
		}
	}
}
func (qt *QueueTx) BlockRequested(height uint64) {
	t := time.Now().Unix()
	for _, v := range qt.bq.blocks {
		if v.Height == height {
			v.LastRequest = t
		}
	}
}
func (bq *BlockQueue) cleanup() {
	t := time.Now().Unix()
	b2 := make([]*QueuedBlock, 0, len(bq.blocks))
	for _, bl := range bq.blocks {
		if bl.Expires >= t {
			b2 = append(b2, bl)
		}
	}
	bq.blocks = b2
}
func (qt *QueueTx) SetBlock(qb *QueuedBlock, replace bool) {
	if qb.Hash == [32]byte{} {
		for i, v := range qt.bq.blocks {
			if v.Height == qb.Height {
				if replace {
					qt.bq.blocks[i] = qb
				}
				return
			}
		}
	} else {
		for i, v := range qt.bq.blocks {
			if v.Hash == qb.Hash {
				if replace {
					qt.bq.blocks[i] = qb
				}
				return
			}
		}
	}
	qt.bq.blocks = append(qt.bq.blocks, qb)
}

func (bq *BlockQueue) Save() {
	err := bq.save()
	if err != nil {
		Log.Fatal(err)
	}
}
func (bq *BlockQueue) save() error {
	bq.cleanup()
	blocks := make([]*QueuedBlock, 0, 10)
	for _, bl := range bq.blocks {
		// only blocks with a known hash are saved, as blocks with height are only necessary for syncing
		if bl.Height == 0 {
			blocks = append(blocks, bl)
		}
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		Log.Fatal(err)
	}
	return bq.bc.DB.Update(func(txn adb.Txn) error {
		return txn.Put(bq.bc.Index.Info, []byte("blocksqueue"), data)
	})
}
func (qt *QueueTx) Length() int {
	return len(qt.bq.blocks)
}
func (qt *QueueTx) GetBlocks() []*QueuedBlock {
	return qt.bq.blocks
}

func (bq *BlockQueue) load() error {
	return bq.bc.DB.View(func(tx adb.Txn) error {
		data := tx.Get(bq.bc.Index.Info, []byte("blocksqueue"))
		if data == nil {
			return errors.New("blocksqueue not saved")
		}
		return json.Unmarshal(data, &bq.blocks)
	})
}
