package block

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/transaction"
	"github.com/virel-project/virel-blockchain/v2/util"
	"github.com/virel-project/virel-blockchain/v2/util/uint128"

	"github.com/zeebo/blake3"
)

type Uint128 = uint128.Uint128

type BlockHeader struct {
	Version     uint8           `json:"version"` // starts at 0
	Height      uint64          `json:"height"`
	Timestamp   uint64          `json:"timestamp"`
	Nonce       uint32          `json:"nonce"`
	NonceExtra  [16]byte        `json:"nonce_extra"`
	OtherChains []HashingID     `json:"other_chains"`
	Recipient   address.Address `json:"recipient"`   // recipient of block's coinbase reward
	Ancestors   Ancestors       `json:"prev_hash"`   // previous block hash
	SideBlocks  []Commitment    `json:"side_blocks"` // list of block previous side blocks (most recent block first)

	// Proof-of-Stake data

	// Id of the delegate staker
	DelegateId uint64
	// Signature of the stake
	StakeSignature bitcrypto.Signature
}

func (b BlockHeader) BlockStakedHash() util.Hash {
	return b.Ancestors[len(b.Ancestors)-1]
}

func (b BlockHeader) PrevHash() util.Hash {
	return b.Ancestors[0]
}

type Block struct {
	BlockHeader `json:"header"`

	Difficulty     Uint128            `json:"diff"`            // block difficulty
	CumulativeDiff Uint128            `json:"cumulative_diff"` // block cumulative difficulty
	Transactions   []transaction.TXID `json:"transactions"`    // list of transaction hashes
}

func (b Block) String() string {
	hash := b.Hash()

	var x string

	commitment := b.Commitment()

	x += "Block " + hex.EncodeToString(hash[:]) + "\n"
	x += "Version: " + strconv.FormatUint(uint64(b.Version), 10) + "\n"
	x += "Height: " + strconv.FormatUint(uint64(b.Height), 10) + "\n"
	x += "Miner: " + b.Recipient.String() + "\n"
	x += "Reward: " + util.FormatCoin(b.Reward()) + "\n"
	x += "Timestamp: " + strconv.FormatUint(uint64(b.Timestamp), 10) + "\n"
	x += "Difficulty: " + b.Difficulty.String() + "\n"
	x += fmt.Sprintf("Cumulative diff: %.3fk\n", b.CumulativeDiff.Float64()/1000)
	x += "Nonce: " + strconv.FormatUint(uint64(b.Nonce), 10) + "\n"
	x += "Base hash: " + commitment.BaseHash.String() + "\n"
	hid := commitment.HashingID()
	x += "This chain hashing id: " + strconv.FormatUint(hid.NetworkID, 16) + " " +
		hex.EncodeToString(hid.Hash[:]) + "\n"
	x += "MiningBlob: " + commitment.MiningBlob().String() + "\n"
	x += "Seed hash: " + fmt.Sprintf("%x (%d)", commitment.MiningBlob().GetSeed(), GetSeedhashId(commitment.Timestamp)) + "\n"
	x += "Other chains: " + strconv.FormatUint(uint64(len(b.OtherChains)), 10) + "\n"
	for _, v := range b.OtherChains {
		x += fmt.Sprintf(" - %x\n", v)
	}
	x += "Transactions: " + strconv.FormatUint(uint64(len(b.Transactions)), 10) + "\n"
	for _, v := range b.Transactions {
		x += fmt.Sprintf(" - %s\n", v)
	}
	x += "Side blocks: " + strconv.FormatUint(uint64(len(b.SideBlocks)), 10) + "\n"
	for _, v := range b.SideBlocks {
		x += fmt.Sprintf(" - %v\n", v)
	}

	return x
}

func (b *Block) SortOtherChains() {
	slices.SortFunc(b.OtherChains, func(a, b HashingID) int {
		if a.NetworkID < b.NetworkID {
			return -1
		} else if a.NetworkID > b.NetworkID {
			return 1
		}
		panic("possible duplicate value in OtherChains")
	})
}

func (b *Block) setMiningBlob(m MiningBlob) error {
	b.Timestamp = m.Timestamp
	b.Nonce = m.Nonce
	b.NonceExtra = m.NonceExtra
	b.OtherChains = make([]HashingID, 0)
	containsNetworkID := false
	var lastNetworkId uint64 = 0
	for _, v := range m.Chains {
		if v.NetworkID != config.NETWORK_ID {
			if v.NetworkID <= lastNetworkId {
				return fmt.Errorf("mining blob is not sorted correctly")
			}
			for _, oc := range b.OtherChains {
				if oc.Hash == v.Hash || oc.NetworkID == v.NetworkID {
					return fmt.Errorf("duplicate hashing id 0x%x %x", v.NetworkID, v.Hash)
				}
			}
			b.OtherChains = append(b.OtherChains, v)
			return nil
		} else {
			if containsNetworkID {
				return fmt.Errorf("mining blob has duplicate network id")
			}
			containsNetworkID = true
		}
	}
	if !containsNetworkID {
		return fmt.Errorf("mining blob does not contain current network id 0x%x", config.NETWORK_ID)
	}
	return nil
}

// Sets the block's OtherChains to sorted chains of MiningBlob.
func (b *Block) SetMiningBlob(m MiningBlob) error {
	if config.IS_MASTERCHAIN {
		panic("SetMiningBlob should never be called in masterchain")
	}

	return b.setMiningBlob(m)
}

func (b BlockHeader) Serialize() []byte {
	s := binary.NewSer(make([]byte, 75))

	s.AddUint8(b.Version)
	s.AddUvarint(b.Height)
	s.AddUvarint(b.Timestamp)
	s.AddUint32(b.Nonce)
	s.AddFixedByteArray(b.NonceExtra[:])
	s.AddFixedByteArray(b.Recipient[:])

	for _, v := range b.Ancestors {
		s.AddFixedByteArray(v[:])
	}

	s.AddUvarint(uint64(len(b.OtherChains)))
	for _, v := range b.OtherChains {
		s.AddUint64(v.NetworkID)
		s.AddFixedByteArray(v.Hash[:])
	}

	s.AddUvarint(uint64(len(b.SideBlocks)))
	for _, v := range b.SideBlocks {
		s.AddFixedByteArray(v.Serialize())
	}

	if b.Version > 0 {
		s.AddUvarint(b.DelegateId)
		s.AddFixedByteArray(b.StakeSignature[:])
	}

	return s.Output()
}
func (b *BlockHeader) Deserialize(data []byte) ([]byte, error) {
	d := binary.NewDes(data)

	b.Version = d.ReadUint8()
	b.Height = d.ReadUvarint()
	b.Timestamp = d.ReadUvarint()
	b.Nonce = d.ReadUint32()
	b.NonceExtra = [16]byte(d.ReadFixedByteArray(16))
	b.Recipient = address.Address(d.ReadFixedByteArray(address.SIZE))

	for i := range b.Ancestors {
		b.Ancestors[i] = [32]byte(d.ReadFixedByteArray(32))
	}

	if d.Error() != nil {
		return nil, d.Error()
	}

	numChains := int(d.ReadUvarint())
	if numChains < 0 || numChains > config.MAX_MERGE_MINED_CHAINS-1 {
		return d.RemainingData(), fmt.Errorf("OtherChains exceed limit: %d", numChains)
	}
	b.OtherChains = make([]HashingID, numChains)
	// check that there are no duplicate chains
	for i := range b.OtherChains {
		if d.Error() != nil {
			return d.RemainingData(), d.Error()
		}
		b.OtherChains[i] = HashingID{
			NetworkID: d.ReadUint64(),
			Hash:      [32]byte(d.ReadFixedByteArray(32)),
		}
	}

	numSideBlocks := int(d.ReadUvarint())
	if numSideBlocks < 0 || numSideBlocks > config.MAX_SIDE_BLOCKS {
		return d.RemainingData(), fmt.Errorf("side blocks exceed limit: %d", numSideBlocks)
	}
	b.SideBlocks = make([]Commitment, numSideBlocks)
	for i := range b.SideBlocks {
		if d.Error() != nil {
			return d.RemainingData(), d.Error()
		}
		err := b.SideBlocks[i].Deserialize(&d)
		if err != nil {
			return d.RemainingData(), err
		}
	}

	if b.Version > 0 {
		b.DelegateId = d.ReadUvarint()
		b.StakeSignature = bitcrypto.Signature(d.ReadFixedByteArray(bitcrypto.SIGNATURE_SIZE))
	}

	return d.RemainingData(), d.Error()
}

func (b Block) Serialize() []byte {
	s := binary.NewSer(make([]byte, 80))

	s.AddFixedByteArray(b.BlockHeader.Serialize())

	if b.Difficulty.IsZero() {
		return nil
	}

	// difficulty is encoded as a little-endian byte slice, with leading zero bytes removed
	diff := make([]byte, 16)
	b.Difficulty.PutBytes(diff)
	for len(diff) > 0 && diff[len(diff)-1] == 0 {
		diff = diff[:len(diff)-1]
	}
	s.AddByteSlice(diff)

	// cumulative difficulty is encoded the same way as difficulty
	diff = make([]byte, 16)
	b.CumulativeDiff.PutBytes(diff)
	for len(diff) > 0 && diff[len(diff)-1] == 0 {
		diff = diff[:len(diff)-1]
	}
	s.AddByteSlice(diff)

	// add transactions
	s.AddUvarint(uint64(len(b.Transactions)))
	for _, v := range b.Transactions {
		s.AddFixedByteArray(v[:])
	}

	return s.Output()
}
func (b *Block) Deserialize(data []byte) error {
	data, err := b.BlockHeader.Deserialize(data)
	if err != nil {
		return err
	}

	d := binary.NewDes(data)

	// read difficulty
	diff := make([]byte, 16)
	copy(diff, d.ReadByteSlice())
	// Note: This is not a correct Little-Endian decoding. Will keep as-is for compatibility.
	b.Difficulty = uint128.FromBytes(diff)
	// read cumulative difficulty
	diff = make([]byte, 16)
	copy(diff, d.ReadByteSlice())
	// Note: This is not a correct Little-Endian decoding. Will keep as-is for compatibility.
	b.CumulativeDiff = uint128.FromBytes(diff)
	if d.Error() != nil {
		return d.Error()
	}

	// read transactions
	numTx := d.ReadUvarint()
	if d.Error() != nil {
		return d.Error()
	}
	if numTx > config.MAX_TX_PER_BLOCK {
		return fmt.Errorf("block has too many transactions: %d, max: %d", numTx, config.MAX_TX_PER_BLOCK)
	}
	b.Transactions = make([]transaction.TXID, numTx)
	for i := uint64(0); i < numTx; i++ {
		txhash := [32]byte(d.ReadFixedByteArray(32))
		if d.Error() != nil {
			return d.Error()
		}
		b.Transactions[i] = txhash
	}

	return d.Error()
}

// deserializes the full block, which includes tx data. Used in P2P.
func (b *Block) DeserializeFull(data []byte) ([]*transaction.Transaction, error) {
	data, err := b.BlockHeader.Deserialize(data)
	if err != nil {
		return nil, err
	}

	d := binary.NewDes(data)

	// read difficulty
	diff := make([]byte, 16)
	copy(diff, d.ReadByteSlice())
	b.Difficulty = Uint128{
		Hi: binary.LittleEndian.Uint64(diff[8:]),
		Lo: binary.LittleEndian.Uint64(diff[:8]),
	}
	// read cumulative difficulty
	diff = make([]byte, 16)
	copy(diff, d.ReadByteSlice())
	b.CumulativeDiff = Uint128{
		Hi: binary.LittleEndian.Uint64(diff[8:]),
		Lo: binary.LittleEndian.Uint64(diff[:8]),
	}

	numTx := d.ReadUvarint()

	if d.Error() != nil {
		return nil, d.Error()
	}

	if numTx > config.MAX_TX_PER_BLOCK {
		return nil, fmt.Errorf("block has too many transactions: %d, max: %d", numTx, config.MAX_TX_PER_BLOCK)
	}

	txs := make([]*transaction.Transaction, numTx)
	b.Transactions = make([]transaction.TXID, numTx)
	for i := uint64(0); i < numTx; i++ {
		sl := d.ReadByteSlice()

		tx := transaction.Transaction{}
		err := tx.Deserialize(sl, b.Height >= config.HARDFORK_V2_HEIGHT)
		if err != nil {
			return nil, err
		}

		txhash := tx.Hash()

		b.Transactions[i] = txhash
		txs[i] = &tx
	}

	return txs, d.Error()
}

type CoinbaseOutput struct {
	Recipient  address.Address
	Amount     uint64
	DelegateId uint64 // if zero, this is not a PoS output
}

func (b *Block) CoinbaseOutputs() []CoinbaseOutput {
	totalReward := b.Reward()

	if b.Version == 0 {
		governanceReward := totalReward * config.BLOCK_REWARD_FEE_PERCENT / 100
		powReward := totalReward - governanceReward

		return []CoinbaseOutput{{
			Recipient: address.GenesisAddress,
			Amount:    governanceReward,
		}, {
			Recipient: b.Recipient,
			Amount:    powReward,
		}}
	} else {
		governanceReward := totalReward * config.BLOCK_REWARD_FEE_PERCENT / 100
		communityReward := totalReward - governanceReward

		if b.StakeSignature == bitcrypto.BlankSignature {
			// for blocks without a valid stake signature, apply 10% burn to PoW reward and burn all the PoS reward.
			powReward := communityReward / 2
			powReward = powReward * 9 / 10
			burnReward := communityReward - powReward

			return []CoinbaseOutput{{
				Recipient: address.GenesisAddress,
				Amount:    governanceReward,
			}, {
				Recipient: b.Recipient,
				Amount:    powReward,
			}, {
				Recipient: address.INVALID_ADDRESS,
				Amount:    burnReward,
			}}
		} else {
			posReward := communityReward / 2
			powReward := communityReward - posReward

			return []CoinbaseOutput{{
				Recipient: address.GenesisAddress,
				Amount:    governanceReward,
			}, {
				Recipient: b.Recipient,
				Amount:    powReward,
			}, {
				Recipient:  b.Recipient,
				Amount:     powReward,
				DelegateId: b.DelegateId,
			}}
		}

	}
}

func (bl *Block) ContributionToCumulativeDiff() uint128.Uint128 {
	sideDiff := bl.Difficulty.Mul64(2 * uint64(len(bl.SideBlocks))).Div64(3)

	if bl.Version > 0 && bl.StakeSignature == bitcrypto.BlankSignature {
		// blocks that are not staked only have 50% contribution to cumulative difficulty
		return bl.Difficulty.Add(sideDiff).Div64(2)
	}
	return bl.Difficulty.Add(sideDiff)
}

func (b *Block) Hash() util.Hash {
	return blake3.Sum256(b.Serialize()[:])
}

func (b *Block) ValidPowHash(hash [16]byte) bool {
	val := uint128.FromBytes(hash[:])
	return val.Cmp(uint128.Max.Div(b.Difficulty)) <= 0
}
func ValidPowHash(hash [16]byte, diff Uint128) bool {
	val := uint128.FromBytes(hash[:])
	return val.Cmp(uint128.Max.Div(diff)) <= 0
}
func ValidPowHash32(hash [32]byte, diff Uint128) bool {
	val := HashToVal(hash)
	return val.Cmp(uint128.Max.Div(diff)) <= 0
}
func ValidPowValue(val Uint128, diff Uint128) bool {
	return val.Cmp(uint128.Max.Div(diff)) <= 0
}
func HashToVal(hash [32]byte) Uint128 {
	return uint128.FromBytes(hash[16:])
}
