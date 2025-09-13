package transaction

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/virel-project/virel-blockchain/v3/address"
	"github.com/virel-project/virel-blockchain/v3/config"
	"github.com/virel-project/virel-blockchain/v3/util"

	"github.com/virel-project/virel-blockchain/v3/binary"
	"github.com/virel-project/virel-blockchain/v3/bitcrypto"

	"github.com/zeebo/blake3"
)

type Transaction struct {
	Version uint8

	Signer    bitcrypto.Pubkey    // sender's public key
	Signature bitcrypto.Signature // transaction signature

	Data TransactionData

	Nonce uint64
	Fee   uint64
}

type TXID [32]byte

func (t TXID) String() string {
	return hex.EncodeToString(t[:])
}

func (t Transaction) Serialize() []byte {
	s := binary.NewSer(make([]byte, 132))

	if t.Version != 0 {
		s.AddUint8(t.Data.AssociatedTransactionVersion())
	}

	s.AddFixedByteArray(t.Signer[:])
	s.AddFixedByteArray(t.Signature[:])

	t.Data.Serialize(&s)

	s.AddUvarint(t.Nonce)
	s.AddUvarint(t.Fee)

	return s.Output()
}
func (t *Transaction) Deserialize(data []byte, hasVersion bool) error {
	d := binary.NewDes(data)

	if hasVersion {
		t.Version = d.ReadUint8()
		if t.Version > MAX_TX_VERSION || t.Version == 0 {
			return fmt.Errorf("invalid transaction version %d", t.Version)
		}
	}
	t.Signer = [bitcrypto.PUBKEY_SIZE]byte(d.ReadFixedByteArray(bitcrypto.PUBKEY_SIZE))
	t.Signature = [bitcrypto.SIGNATURE_SIZE]byte(d.ReadFixedByteArray(bitcrypto.SIGNATURE_SIZE))

	switch t.Version {
	case 0, TX_VERSION_TRANSFER: // Transfer
		t.Data = &Transfer{}
	case TX_VERSION_REGISTER_DELEGATE: // Register delegate
		t.Data = &RegisterDelegate{}
	case TX_VERSION_SET_DELEGATE: // Set delegate
		t.Data = &SetDelegate{}
	case TX_VERSION_STAKE:
		t.Data = &Stake{}
	case TX_VERSION_UNSTAKE:
		t.Data = &Unstake{}
	default:
		return fmt.Errorf("unknown transaction version %d at deserialize, hasversion: %v", t.Version, hasVersion)
	}
	err := t.Data.Deserialize(&d)
	if err != nil {
		return err
	}
	t.Nonce = d.ReadUvarint()
	t.Fee = d.ReadUvarint()

	if d.Error() != nil {
		return fmt.Errorf("transaction with version %d encountered deserialization error: %w", t.Version, d.Error())
	}
	return nil
}

func (t Transaction) Hash() TXID {
	return blake3.Sum256(t.Serialize())
}

// total amount of the transaction (includes fee)
func (t Transaction) TotalAmount() (uint64, error) {
	var s uint64 = t.Fee

	amt, err := t.Data.TotalAmount(&t)
	if err != nil {
		return 0, err
	}
	if amt+s < amt {
		return 0, errors.New("overflow on output")
	}

	return amt + s, nil
}

// The base overhad of all transactions. A transaction's VSize cannot be smaller than this.
const base_overhead = bitcrypto.PUBKEY_SIZE /*sender*/ + bitcrypto.SIGNATURE_SIZE /*signature*/ + 1 /*timestamp*/ + 1 /*nonce*/ + 1 /*fee*/

const output_overhead = address.SIZE /* address */ + 1 /* subaddress */ + 1 /* amount */

const max_tx_size = 2048

func (t Transaction) GetVirtualSize() uint64 {
	return base_overhead + t.Data.VSize()
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
func (t *Transaction) Prevalidate(height uint64) error {
	// verify VSize
	vsize := t.GetVirtualSize()

	if vsize > max_tx_size {
		return fmt.Errorf("invalid vsize: %d > MAX_TX_SIZE", vsize)
	}

	// verify version
	if height < config.HARDFORK_V2_HEIGHT {
		if t.Version != 0 {
			return fmt.Errorf("invalid version %d, expected 0", t.Version)
		}
	} else if height < config.HARDFORK_V3_HEIGHT {
		if t.Version != 1 {
			return fmt.Errorf("invalid version %d, expected 1", t.Version)
		}
	} else {
		if t.Version != TX_VERSION_TRANSFER &&
			t.Version != TX_VERSION_REGISTER_DELEGATE &&
			t.Version != TX_VERSION_SET_DELEGATE &&
			t.Version != TX_VERSION_STAKE &&
			t.Version != TX_VERSION_UNSTAKE {
			return fmt.Errorf("invalid version %d, expected 1-5", t.Version)
		}
	}

	// verify sender address
	senderAddr := address.FromPubKey(t.Signer)
	if senderAddr == address.INVALID_ADDRESS {
		return errors.New("invalid sender public key")
	}

	// verify that fee is higher than minimum fee level
	var fee_per_byte uint64 = config.FEE_PER_BYTE
	if height >= config.HARDFORK_V3_HEIGHT {
		fee_per_byte = config.FEE_PER_BYTE_V2
	}
	if t.Fee < fee_per_byte*vsize {
		return fmt.Errorf("invalid transaction fee: got %d, expected at least %d", t.Fee,
			fee_per_byte*vsize)
	}

	// verify signature
	sigValid := bitcrypto.VerifySignature(t.Signer, t.SignatureData(), t.Signature)
	if !sigValid {
		return fmt.Errorf("invalid signature")
	}

	// prevalidate transaction data
	err := t.Data.Prevalidate(t)
	if err != nil {
		return err
	}

	// verify that there's no overflow
	totamt, err := t.TotalAmount()
	if err != nil {
		return err
	}

	atsd := t.Data.StateOutputs(t, senderAddr)
	stateSum := t.Fee
	for _, v := range atsd {
		stateSum += v.Amount
	}
	if stateSum != totamt {
		return fmt.Errorf("sum of transaction outputs is %d, total amount %d: this should never happen", stateSum, totamt)
	}

	return nil
}

func (t *Transaction) String() string {
	hash := t.Hash()
	o := "Transaction " + hex.EncodeToString(hash[:]) + "\n"

	signer := address.FromPubKey(t.Signer)

	o += fmt.Sprintf(" Version: %d\n", t.Version)
	o += " VSize: " + util.FormatUint(t.GetVirtualSize()) + "; physical size: " + util.FormatInt(len(t.Serialize())) + "\n"
	o += " Signer: " + signer.String() + "\n"

	o += t.Data.String()

	o += " Inputs:\n"
	for _, v := range t.Data.StateInputs(t, signer) {
		o += " - " + v.String() + "\n"
	}

	o += " Outputs:\n"
	for _, v := range t.Data.StateOutputs(t, signer) {
		o += " - " + v.String() + "\n"
	}

	o += " Signature: " + hex.EncodeToString(t.Signature[:]) + "\n"

	o += " Nonce: " + util.FormatUint(t.Nonce) + "\n"
	amount, _ := t.TotalAmount()
	o += " Total amount: " + util.FormatUint(amount) + "\n"
	o += " Fee: " + util.FormatUint(t.Fee)

	return o
}
