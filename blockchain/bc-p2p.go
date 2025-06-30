package blockchain

import (
	"time"

	"github.com/virel-project/virel-blockchain/adb"
	"github.com/virel-project/virel-blockchain/block"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/p2p"
	"github.com/virel-project/virel-blockchain/p2p/packet"
	"github.com/virel-project/virel-blockchain/transaction"
)

func (bc *Blockchain) pinger() {
	for {
		func() {
			time.Sleep(config.P2P_PING_INTERVAL * time.Second)

			bc.P2P.RLock()

			for _, v := range bc.P2P.Connections {
				go v.SendPacket(&p2p.Packet{
					Type: packet.PING,
					Data: []byte{},
				})
			}
			bc.P2P.RUnlock()
		}()
	}
}

func (bc *Blockchain) newConnections() {
	for {
		conn := <-bc.P2P.NewConnections

		var stats *Stats
		bc.DB.View(func(tx adb.Txn) error {
			stats = bc.GetStats(tx)
			return nil
		})

		conn.SendPacket(&p2p.Packet{
			Type: packet.STATS,
			Data: packet.PacketStats{
				Height:         stats.TopHeight,
				CumulativeDiff: stats.CumulativeDiff,
				Hash:           stats.TopHash,
			}.Serialize(),
		})
	}
}
func (bc *Blockchain) incomingP2P() {
	for {
		pack := <-bc.P2P.PacketsIn

		if pack.Type == packet.BLOCK {
			Log.Debug("Received new block packet")
			bc.packetBlock(pack)
		} else if pack.Type == packet.TX {
			Log.Debug("Received new transaction packet")
			bc.packetTx(pack)
		} else if pack.Type == packet.STATS {
			bc.packetStats(pack)
		} else if pack.Type == packet.BLOCK_REQUEST {
			Log.Debug("Received block request packet")
			go bc.packetBlockRequest(pack)
		}
	}
}

func (bc *Blockchain) packetTx(pack p2p.Packet) {
	tx := &transaction.Transaction{}

	err := tx.Deserialize(pack.Data)
	if err != nil {
		Log.Warn(err)
		return
	}
	err = tx.Prevalidate()
	if err != nil {
		Log.Warn(err)
		return
	}

	err = bc.DB.Update(func(txn adb.Txn) error {
		return bc.AddTransaction(txn, tx, tx.Hash(), true)
	})
	if err != nil {
		Log.Warn(err)
		return
	}
}

func (bc *Blockchain) packetBlock(pack p2p.Packet) {
	bl := &block.Block{}

	txs, err := bl.DeserializeFull(pack.Data)
	if err != nil {
		Log.Warn("invalid block received:", err)
		return
	}

	err = bl.Prevalidate()
	if err != nil {
		Log.Warn("invalid block received:", err)
		return
	}

	var hash [32]byte
	err = bc.DB.Update(func(tx adb.Txn) error {
		for _, v := range txs {
			err := bc.AddTransaction(tx, v, v.Hash(), false)
			if err != nil {
				return err
			}
		}
		hash, err = bc.AddBlock(tx, bl)
		return err
	})
	if err != nil {
		Log.Warn("could not add block to chain:", err)
		bc.BlockQueue.Update(func(qt *QueueTx) {
			qt.RemoveBlock(bl.Height, hash)
		})
		return
	} else {
		// TODO: remove this, it's only for debug purposes
		err := bc.DB.View(func(tx adb.Txn) error {
			bc.CheckSupply(tx)
			return nil
		})
		if err != nil {
			Log.Err(err)
		}
	}
}

func (bc *Blockchain) packetStats(pack p2p.Packet) {
	st := packet.PacketStats{}

	err := st.Deserialize(pack.Data)
	if err != nil {
		Log.Warn(err)
		return
	}

	Log.Dev("peer has stats", st)
	pack.Conn.PeerData(func(d *p2p.PeerData) {
		d.Stats = st
	})

	bc.SyncMut.Lock()
	if st.CumulativeDiff.Cmp(bc.SyncDiff) > 0 {
		Log.Infof("New target: height %d, cumulative diff %s", st.Height, st.CumulativeDiff)
		bc.SyncHeight = st.Height
		bc.SyncDiff = st.CumulativeDiff
	}
	bc.SyncMut.Unlock()
}

func (bc *Blockchain) packetBlockRequest(pack p2p.Packet) {
	st := packet.PacketBlockRequest{}

	err := st.Deserialize(pack.Data)
	if err != nil {
		Log.Warn(err)
		return
	}

	Log.Devf("received block request with height %d hash %x", st.Height, st.Hash)

	var bl *block.Block
	err = bc.DB.View(func(tx adb.Txn) (err error) {
		if st.Height == 0 {
			bl, err = bc.GetBlock(tx, st.Hash)
		} else {
			bl, err = bc.GetBlockByHeight(tx, st.Height)
		}
		return
	})
	if err != nil {
		Log.Debug("received invalid block request:", err)
		return
	}

	var d []byte
	err = bc.DB.View(func(txn adb.Txn) (err error) {
		d, err = bc.SerializeFullBlock(txn, bl)
		return
	})
	if err != nil {
		Log.Err(err)
		return
	}

	pack.Conn.SendPacket(&p2p.Packet{
		Type: packet.BLOCK,
		Data: d,
	})
}

func (bc *Blockchain) SendStats(stats *Stats) {
	for _, v := range bc.P2P.Connections {
		v.SendPacket(&p2p.Packet{
			Type: packet.STATS,
			Data: packet.PacketStats{
				Height:         stats.TopHeight,
				CumulativeDiff: stats.CumulativeDiff,
				Hash:           stats.TopHash,
			}.Serialize(),
		})
	}
}

func (bc *Blockchain) BroadcastBlock(bl *block.Block) {
	Log.Debug("broadcasting block")

	var ser []byte
	err := bc.DB.View(func(txn adb.Txn) (err error) {
		ser, err = bc.SerializeFullBlock(txn, bl)
		return
	})
	if err != nil {
		Log.Err(err)
		return
	}

	// if remote peers have a cumulative difficulty larger than this, then most likely they aren't interested in the block
	maxCumDiff := bl.CumulativeDiff.Add(bl.Difficulty.Mul64(config.MINIDAG_ANCESTORS + 2))

	bc.P2P.RLock()
	for _, v := range bc.P2P.Connections {
		// only send the block if the peer has a smaller cumulative difficulty
		go v.PeerData(func(d *p2p.PeerData) {
			if d.Stats.CumulativeDiff.Cmp(maxCumDiff) <= 0 {
				v.SendPacket(&p2p.Packet{
					Type: packet.BLOCK,
					Data: ser,
				})
			}
		})
	}
	bc.P2P.RUnlock()
}
