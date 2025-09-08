package blockchain

import (
	"errors"
	"fmt"
	"time"

	"github.com/virel-project/virel-blockchain/v2/adb"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/virel-project/virel-blockchain/v2/block"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/p2p"
	"github.com/virel-project/virel-blockchain/v2/p2p/packet"
	"github.com/virel-project/virel-blockchain/v2/transaction"
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

		go func() {
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
		}()
	}
}
func (bc *Blockchain) incomingP2P() {
	for {
		pack := <-bc.P2P.PacketsIn

		switch pack.Type {
		case packet.BLOCK:
			bc.Validator.ProcessBlock(pack)
		case packet.TX:
			Log.Debug("Received new transaction packet")
			bc.packetTx(pack)
		case packet.STATS:
			bc.packetStats(pack)
		case packet.BLOCK_REQUEST:
			Log.Debug("Received block request packet")
			go bc.packetBlockRequest(pack)
		case packet.STAKE_SIGNATURE:
			Log.Debug("Received stake signature packet")
			go bc.packetStakeSignature(pack)
		}
	}
}

func (bc *Blockchain) packetTx(pack p2p.Packet) {
	tx := &transaction.Transaction{}

	var stats *Stats
	bc.DB.View(func(txn adb.Txn) error {
		stats = bc.GetStats(txn)
		return nil
	})

	err := tx.Deserialize(pack.Data, stats.TopHeight >= config.HARDFORK_V2_HEIGHT)
	if err != nil {
		Log.Warn(err)
		return
	}
	err = tx.Prevalidate(stats.TopHeight + 1)
	if err != nil {
		Log.Warn(err)
		return
	}

	err = bc.DB.Update(func(txn adb.Txn) error {
		return bc.AddTransaction(txn, tx, tx.Hash(), true, stats.TopHeight+1)
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

	Log.Debugf("Processing block %d %x", bl.Height, bl.Hash())

	hash := bl.Hash()

	bc.queuedBlockDownloaded(hash, bl.Height)

	err = bc.PrevalidateBlock(bl, txs)
	if err != nil {
		Log.Warn("invalid block received:", err)
		return
	}

	bc.SyncMut.RLock()
	insta := bl.Height >= bc.SyncHeight
	bc.SyncMut.RUnlock()

	bc.Validator.PostprocessBlock(bl, hash, txs, insta)
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

	Log.Devf("received block request with height %d count %d hash %x", st.Height, st.Count, st.Hash)

	if st.Count > config.PARALLEL_BLOCKS_DOWNLOAD {
		Log.Warn("invalid block request count", st.Count)
		return
	}

	var bl *block.Block
	var bls = make([]*block.Block, 0, st.Count+1)
	err = bc.DB.View(func(tx adb.Txn) (err error) {
		if st.Height == 0 {
			bl, err = bc.GetBlock(tx, st.Hash)
			if err != nil {
				return
			}
			bls = append(bls, bl)
		} else {
			for height := st.Height; height <= st.Height+uint64(st.Count); height++ {
				bl, err = bc.GetBlockByHeight(tx, height)
				if err != nil {
					return
				}
				bls = append(bls, bl)
			}
		}
		return
	})
	if err != nil {
		Log.Debug("received invalid block request:", err)

		// send all the blocks we have successfully collected
		if len(bls) > 0 {
			for _, v := range bls {
				err := bc.sendBlockToPeer(v, pack.Conn)
				if err != nil {
					Log.Debug(err)
				}
			}
		}
		return
	}

	// send all the blocks we have successfully collected
	for _, v := range bls {
		err := bc.sendBlockToPeer(v, pack.Conn)
		if err != nil {
			Log.Debug(err)
		}
	}
}

func (bc *Blockchain) packetStakeSignature(pack p2p.Packet) {
	st := &packet.PacketStakeSignature{}

	err := st.Deserialize(pack.Data)
	if err != nil {
		Log.Warn(err)
		return
	}

	err = bc.HandleStakeSignature(st)
	if err != nil {
		Log.Debug(err)
	}
}

func (bc *Blockchain) HandleStakeSignature(st *packet.PacketStakeSignature) error {
	err := bc.DB.Update(func(txn adb.Txn) error {
		_, err := bc.GetStakeSig(txn, st.Hash)
		if err == nil {
			return errors.New("stake signature already in database")
		}

		block, err := bc.GetBlock(txn, st.Hash)
		if err != nil {
			return err
		}
		if block.NextDelegateId != st.DelegateId {
			return errors.New("stake signature is not done by the correct delegate")
		}

		delegate, err := bc.GetDelegate(txn, st.DelegateId)
		if err != nil {
			return fmt.Errorf("failed to get delegate %d: %w", st.DelegateId, err)
		}

		sigOk := bitcrypto.VerifySignature(delegate.Owner, append(config.STAKE_SIGN_PREFIX, st.Hash[:]...), st.Signature)
		if !sigOk {
			return fmt.Errorf("invalid StakeSignature packet: invalid signature")
		}

		err = bc.SetStakeSig(txn, st)
		if err != nil {
			Log.Err(err)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	bc.BroadcastStakeSig(st)
	return nil
}

func (bc *Blockchain) sendBlockToPeer(bl *block.Block, c *p2p.Connection) error {
	var d []byte
	err := bc.DB.View(func(txn adb.Txn) (err error) {
		d, err = bc.SerializeFullBlock(txn, bl)
		return
	})
	if err != nil {
		return err
	}

	return c.SendPacket(&p2p.Packet{
		Type: packet.BLOCK,
		Data: d,
	})
}

func (bc *Blockchain) SendStats(stats *Stats) {
	bc.P2P.RLock()
	defer bc.P2P.RUnlock()

	for _, v := range bc.P2P.Connections {
		go v.SendPacket(&p2p.Packet{
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
	defer bc.P2P.RUnlock()
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
}

// Broadcasts a StakeSignature packet
func (bc *Blockchain) BroadcastStakeSig(pack *packet.PacketStakeSignature) {
	Log.Debug("broadcasting stake sig")

	ser := pack.Serialize()

	bc.P2P.RLock()
	defer bc.P2P.RUnlock()
	for _, v := range bc.P2P.Connections {
		go v.PeerData(func(d *p2p.PeerData) {
			v.SendPacket(&p2p.Packet{
				Type: packet.STAKE_SIGNATURE,
				Data: ser,
			})
		})
	}
}
