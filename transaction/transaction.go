package transaction

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/virel-project/virel-blockchain/address"
	"github.com/virel-project/virel-blockchain/config"
	"github.com/virel-project/virel-blockchain/util"

	"github.com/virel-project/virel-blockchain/binary"
	"github.com/virel-project/virel-blockchain/bitcrypto"

	"github.com/zeebo/blake3"
)

type Transaction struct {
	Sender    bitcrypto.Pubkey    // sender's public key
	Signature bitcrypto.Signature // transaction signature

	Outputs []Output // transaction outputs

	Nonce uint64 // count of transactions sent by the sender, starting from 1
	Fee   uint64 // fee of the transaction
}

type Output struct {
	Recipient address.Address // recipient's address
	Subaddr   uint64          // subaddress id
	Amount    uint64          // amount excludes the fee
}

func (o Output) Serialize() []byte {
	b := binary.NewSer(make([]byte, address.SIZE+4+4))

	b.AddFixedByteArray(o.Recipient[:])
	b.AddUvarint(o.Subaddr)
	b.AddUvarint(o.Amount)

	return b.Output()
}
func (o *Output) Deserialize(d *binary.Des) error {
	o.Recipient = address.Address(d.ReadFixedByteArray(address.SIZE))
	o.Subaddr = d.ReadUvarint()
	o.Amount = d.ReadUvarint()

	return d.Error()
}

func (o Output) String() string {
	return fmt.Sprintf("amount: %d recipient: %v subaddr: %d", o.Amount, o.Recipient, o.Subaddr)
}

type TXID [32]byte

func (t TXID) String() string {
	return hex.EncodeToString(t[:])
}

func (t Transaction) Serialize() []byte {
	s := binary.NewSer(make([]byte, 120))

	s.AddFixedByteArray(t.Sender[:])
	s.AddFixedByteArray(t.Signature[:])

	s.AddUvarint(uint64(len(t.Outputs)))
	for _, v := range t.Outputs {
		s.AddFixedByteArray(v.Serialize())
	}

	s.AddUvarint(t.Nonce)
	s.AddUvarint(t.Fee)

	return s.Output()
}
func (t *Transaction) Deserialize(data []byte) error {
	d := binary.NewDes(data)

	t.Sender = [bitcrypto.PUBKEY_SIZE]byte(d.ReadFixedByteArray(bitcrypto.PUBKEY_SIZE))
	t.Signature = [bitcrypto.SIGNATURE_SIZE]byte(d.ReadFixedByteArray(bitcrypto.SIGNATURE_SIZE))

	numOutputs := d.ReadUvarint()
	if numOutputs > config.MAX_OUTPUTS || numOutputs == 0 {
		return errors.New("too many outputs")
	}
	t.Outputs = make([]Output, numOutputs)
	for i := range t.Outputs {
		err := t.Outputs[i].Deserialize(&d)
		if err != nil {
			return err
		}
	}

	t.Nonce = d.ReadUvarint()
	t.Fee = d.ReadUvarint()

	return d.Error()
}

func (t Transaction) Hash() TXID {
	return blake3.Sum256(t.Serialize())
}

// total amount of the transaction (includes fee)
func (t Transaction) TotalAmount() uint64 {
	var s uint64 = t.Fee

	for _, v := range t.Outputs {
		s += v.Amount
	}

	return s
}

// The base overhad of all transactions. A transaction's VSize cannot be smaller than this.
const base_overhead = bitcrypto.PUBKEY_SIZE /*sender*/ + bitcrypto.SIGNATURE_SIZE /*signature*/ + 1 /*timestamp*/ + 1 /*nonce*/ + 1 /*fee*/

const output_overhead = address.SIZE /* address */ + 1 /* subaddress */ + 1 /* amount */

const max_tx_size = base_overhead + config.MAX_OUTPUTS*output_overhead

func (t Transaction) GetVirtualSize() uint64 {
	return base_overhead + uint64(len(t.Outputs))*(output_overhead)
}

func (t Transaction) SignatureData() []byte {
	t.Signature = bitcrypto.Signature{}

	return t.Serialize()
}

func (t *Transaction) Sign(pk bitcrypto.Privkey) error {
	sig, err := bitcrypto.Sign(t.SignatureData(), pk)

	t.Signature = sig

	return err
}

// executes partial verification of transaction data, should be used before blockchain AddTransaction
func (t *Transaction) Prevalidate() error {
	// verify VSize
	vsize := t.GetVirtualSize()

	if vsize > max_tx_size {
		return fmt.Errorf("invalid vsize: %d > MAX_TX_SIZE", vsize)
	}

	// verify the number of outputs
	if len(t.Outputs) == 0 || len(t.Outputs) > config.MAX_OUTPUTS {
		return fmt.Errorf("invalid output count: %d", len(t.Outputs))
	}

	// verify sender address
	senderAddr := address.FromPubKey(t.Sender)
	if senderAddr == address.INVALID_ADDRESS {
		return errors.New("invalid sender public key")
	}

	// verify that fee is higher than minimum fee level
	if t.Fee < config.FEE_PER_BYTE*vsize {
		return fmt.Errorf("invalid transaction fee: got %d, expected at least %d", t.Fee,
			config.FEE_PER_BYTE*vsize)
	}

	// verify signature
	sigValid := bitcrypto.VerifySignature(t.Sender, t.SignatureData(), t.Signature)
	if !sigValid {
		return fmt.Errorf("invalid signature")
	}

	// TODO: check if there is something else to prevalidate here

	return nil
}

func (t *Transaction) String() string {
	hash := t.Hash()
	o := "Transaction " + hex.EncodeToString(hash[:]) + "\n"

	o += " VSize: " + util.FormatUint(t.GetVirtualSize()) + "; physical size: " + util.FormatInt(len(t.Serialize())) + "\n"
	o += " Sender: " + address.FromPubKey(t.Sender).Integrated().String() + "\n"
	o += " Outputs:\n"
	for _, v := range t.Outputs {
		o += " - " + v.String()
	}

	o += " Signature: " + hex.EncodeToString(t.Signature[:]) + "\n"

	o += " Nonce: " + util.FormatUint(t.Nonce) + "\n"
	o += " Total amount: " + util.FormatUint(t.TotalAmount()) + "\n"
	o += " Fee: " + util.FormatUint(t.Fee)

	return o
}
