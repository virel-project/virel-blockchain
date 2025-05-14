package blockchain

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
	"virel-blockchain/block"
	"virel-blockchain/stratum"
	"virel-blockchain/stratum/stratumclient"
	"virel-blockchain/util"
	"virel-blockchain/util/uint128"
)

type mergestratum struct {
	Destination string
	Permanent   bool
	Client      *stratumclient.Client
	HashingID   block.HashingID
	Difficulty  uint64
	JobID       string

	sync.RWMutex
}

func (m *mergestratum) Start() error {
	cl := m.Client
	Log.Debug("starting merge stratum", m.Destination)
	return cl.Start()
}

// AddStratum blocks until the stratum errors out if permanent, otherwise it blocks forever.
func (bc *Blockchain) AddStratum(ip, stratum_wallet string, permanent bool) {
	bc.MergesMut.Lock()
	for _, v := range bc.Merges {
		if v.Destination == ip { // avoid duplicate stratums
			bc.MergesMut.Unlock()
			return
		}
	}
	cl, err := stratumclient.New(ip, stratum_wallet)
	if err != nil {
		Log.Err(err)
		return
	}
	m := &mergestratum{
		Destination: ip,
		Permanent:   permanent,
		Client:      cl,
	}
	bc.Merges = append(bc.Merges, m)
	bc.MergesMut.Unlock()

	defer func() {
		// remove this merge stratum from bc.Merges
		bc.MergesMut.Lock()
		defer bc.MergesMut.Unlock()

		if permanent {
			defer func() {
				go func() {
					time.Sleep(1 * time.Second)
					bc.AddStratum(ip, stratum_wallet, permanent)
				}()
			}()
		}

		for i, v := range bc.Merges {
			if v.Destination == ip { // remove the stratum
				bc.Merges = append(bc.Merges[:i], bc.Merges[i+1:]...)
				return
			}
		}
	}()

	err = m.Start()
	if err != nil {
		Log.Warn(err)
	}

	for {
		// this is master connecting to slave.
		// S will start sending jobs. Analyze them to obtain the HashingID, which will be used by the daemon
		// for creating Merge Mining jobs, and also the Difficulty.
		// We will submit solutions to S when we find a block that meets the slavechain difficulty.

		job, ok := <-m.Client.JobChan
		if !ok {
			Log.Debug("job chan closed")
			return
		}

		Log.Dev("recv job:", job)

		mb := block.MiningBlob{}
		err := mb.Deserialize(job.Blob)
		if err != nil {
			Log.Debug("failed to deserialize mining blob:", err)
			m.Client.Close()
			return
		}
		if len(mb.Chains) != 1 {
			Log.Warn("mining blob has", len(mb.Chains), "hashing ids, should be 1")
			Log.Warnf("%x", mb.Chains)
			m.Client.Close()
			return
		}
		if len(job.Target) != 16 && len(job.Target) != 8 && len(job.Target) != 4 {
			Log.Warn("invalid job target size", len(job.Target))
			m.Client.Close()
			return
		}
		m.HashingID = mb.Chains[0]
		m.Difficulty = util.ByteTargetToDiff(job.Target).Lo
		m.JobID = job.JobID

		Log.Infof("Received merge-mined job network: %x difficulty: %d job id: %s", m.HashingID.NetworkID,
			m.Difficulty, m.JobID)

		bc.MergesMut.Lock()
		bc.mergesUpdated = true
		bc.MergesMut.Unlock()

		go bc.NewStratumJob(false)
	}
}

// returns true if the action is successful for at least one merge mined chain
func (bc *Blockchain) submitMergeMinedBlock(bl *block.Block, pow [16]byte) ([]stratum.FoundBlockInfo, bool) {
	bc.MergesMut.RLock()
	defer bc.MergesMut.RUnlock()

	founds := []stratum.FoundBlockInfo{}

	numFounds := 0
	foundsChan := make(chan stratum.FoundBlockInfo, 1)

	for _, v := range bc.Merges {
		v.RLock()

		if v.Difficulty != 0 {
			if matchesDiff(pow, v.Difficulty) {
				numFounds++

				go func() {
					Log.Infof("submit merge mined block to chain %x", v.HashingID.NetworkID)
					nonceHex := make([]byte, 4)
					binary.LittleEndian.PutUint32(nonceHex, bl.Nonce)
					res, err := v.Client.SendWork(stratum.SubmitRequest{
						JobID:  v.JobID,
						Nonce:  hex.EncodeToString(nonceHex),
						Blob:   bl.Commitment().MiningBlob().Serialize(),
						Result: hex.EncodeToString(pow[:]),
					})

					if err != nil {
						Log.Err("submit merge mined block to chain", v.HashingID.NetworkID, "failed:", err)
						foundsChan <- stratum.FoundBlockInfo{
							Ok: false,
						}
						return
					}

					var result stratum.SubmitResponse

					err = json.Unmarshal(res.Result, &result)
					if err != nil || len(result.Blocks) != 1 {
						Log.Err("submit merge mined block to chain", v.HashingID.NetworkID, "failed:", err)
						foundsChan <- stratum.FoundBlockInfo{
							Ok: false,
						}
						return
					}

					foundsChan <- result.Blocks[0]
				}()
			}
		}

		v.RUnlock()
	}

	for i := 0; i < numFounds; i++ {
		f := <-foundsChan
		founds = append(founds, f)
	}

	return founds, numFounds > 0
}

func matchesDiff(hash [16]byte, diff uint64) bool {
	val := uint128.FromBytes(hash[:])

	return val.Cmp(uint128.Max.Div64(diff)) <= 0
}
