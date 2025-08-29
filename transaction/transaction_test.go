package transaction_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"slices"
	"testing"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/bitcrypto"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/transaction"

	"github.com/zeebo/blake3"
)

func TestTransaction(t *testing.T) {
	sd := blake3.Sum256([]byte("test"))
	privk := ed25519.NewKeyFromSeed(sd[:])
	pubk := bitcrypto.Pubkey(privk.Public().(ed25519.PublicKey))

	if hex.EncodeToString(pubk[:]) != "87560320f9cd73a12ef35c886bcde72049d8e4d83ea3b32586270bc7d8e8e422" {
		t.Errorf("invalid public key %x", privk.Public())
	}

	recipient := address.Address{}
	rand.Read(recipient[:])

	tx := transaction.Transaction{
		Version: 1,
		Sender:  pubk,
		Data: &transaction.Transfer{
			Outputs: []transaction.Output{
				{
					Recipient: recipient,
					PaymentId: 1337,
					Amount:    config.COIN,
				},
			},
		},
		Signature: bitcrypto.Signature{},
		Nonce:     1,
		Fee:       0,
	}
	tx.Fee = tx.GetVirtualSize() * config.FEE_PER_BYTE

	err := tx.Sign(bitcrypto.Privkey(privk))
	if err != nil {
		t.Error(err)
	}

	ser := tx.Serialize()

	t.Logf("transaction size: %d, data: %x", len(ser), ser)

	tx2 := transaction.Transaction{}
	err = tx2.Deserialize(ser, true)
	if err != nil {
		t.Error(err)
	}

	ser2 := tx2.Serialize()

	t.Logf("transaction size: %d, data: %x", len(ser2), ser2)

	if !slices.Equal(ser, ser2) {
		t.Error("second serialized transaction differs from original")
		t.Log("tx 1")
		t.Log(tx.String())
		t.Log("tx 2")
		t.Log(tx2.String())
	}

	err = tx.Prevalidate(config.HARDFORK_V3_HEIGHT)

	if err != nil {
		t.Error("transaction verification failed:", err)
	}

	t.Log(tx.String())
}
