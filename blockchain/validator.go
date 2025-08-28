package blockchain

import (
	"time"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/p2p"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
)

const POSTPROCESS_SIZE = config.PARALLEL_BLOCKS_DOWNLOAD

// Validator implements a thread pool used for efficient block validation
type Validator struct {
	bc          *Blockchain
	Parallelism int
	newBlocks   chan p2p.Packet

	postprocessChan chan bool

	postprocess    []PostprocessData
	postprocessMut util.RWMutex
}

func (bc *Blockchain) NewValidator(parallelism int) *Validator {
	v := &Validator{
		bc:              bc,
		Parallelism:     parallelism,
		newBlocks:       make(chan p2p.Packet, parallelism),
		postprocess:     make([]PostprocessData, 0, POSTPROCESS_SIZE),
		postprocessChan: make(chan bool, 10),
	}

	v.startProcessingBlocks()
	go v.startPostprocessor()

	go func() {
		for {
			v.selectAndPostprocess()
			time.Sleep(time.Second)
		}
	}()

	return v
}

func (v *Validator) Close() {
	close(v.newBlocks)
	close(v.postprocessChan)
	v.postprocessMut.Lock()
	v.postprocess = []PostprocessData{}
	v.postprocessMut.Unlock()
}

func (v *Validator) startProcessingBlocks() {
	for range v.Parallelism {
		go func() {
			for {
				blpacket, ok := <-v.newBlocks
				if !ok {
					return
				}
				v.bc.packetBlock(blpacket)
			}
		}()
	}
}

func (v *Validator) ProcessBlock(blpacket p2p.Packet) {
	v.newBlocks <- blpacket
}

type PostprocessData struct {
	Block *block.Block
	Hash  util.Hash
	Txs   []*transaction.Transaction
}

func (v *Validator) PostprocessBlock(bl *block.Block, hash util.Hash, txs []*transaction.Transaction, insta bool) {

	Log.Debug("PostprocessBlock", bl.Height)

	v.postprocessMut.Lock()
	v.postprocess = append(v.postprocess, PostprocessData{
		Block: bl,
		Hash:  hash,
		Txs:   txs,
	})
	v.postprocessMut.Unlock()

	if insta {
		v.postprocessChan <- true
		return
	}

	if len(v.postprocess) >= POSTPROCESS_SIZE {
		v.postprocessChan <- true
	}
}

func (v *Validator) startPostprocessor() {
	for {
		_, ok := <-v.postprocessChan
		if !ok {
			Log.Debug("closing postprocessor")
			return
		}
		v.selectAndPostprocess()
	}
}

func (v *Validator) selectAndPostprocess() {
	for {
		v.postprocessMut.Lock()
		if len(v.postprocess) == 0 {
			v.postprocessMut.Unlock()
			break
		}
		// find the index (smallestIdx) of the block with the smallest height
		var smallestHeight uint64 = v.postprocess[0].Block.Height
		smallestIdx := 0
		for i := 1; i < len(v.postprocess); i++ {
			h := v.postprocess[i].Block.Height

			if h < smallestHeight {
				smallestIdx = i
				smallestHeight = h
			}
		}

		Log.Debug("actually processing block ", smallestHeight, smallestIdx)

		// postprocess the block

		ppd := v.postprocess[smallestIdx]

		// remove the block from the postprocess queue (order is not important)
		lastidx := len(v.postprocess) - 1
		v.postprocess[smallestIdx] = v.postprocess[lastidx]
		v.postprocess = v.postprocess[:lastidx]
		v.postprocessMut.Unlock()

		v.executePostprocess(ppd.Block, ppd.Hash, ppd.Txs)
	}
}

func (v *Validator) executePostprocess(bl *block.Block, hash util.Hash, txs []*transaction.Transaction) {
	err := v.bc.DB.Update(func(txn adb.Txn) error {
		for _, tx := range txs {
			err := v.bc.AddTransaction(txn, tx, tx.Hash(), false)
			if err != nil {
				return err
			}
		}
		return v.bc.AddBlock(txn, bl, hash)
	})
	if err != nil {
		Log.Warn("could not add block to chain:", err)
		v.bc.BlockQueue.Update(func(qt *QueueTx) {
			qt.RemoveBlock(bl.Height, hash)
		})
		return
	} else if Log.GetLogLevel() > 1 {
		// TODO: remove this, it's only for debug purposes
		err := v.bc.DB.View(func(tx adb.Txn) error {
			v.bc.CheckSupply(tx)
			return nil
		})
		if err != nil {
			Log.Err(err)
		}
	}
}
