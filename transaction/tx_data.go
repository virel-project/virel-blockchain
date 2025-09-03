package transaction

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/virel-project/virel-blockchain/v2/address"
	"github.com/virel-project/virel-blockchain/v2/binary"
	"github.com/virel-project/virel-blockchain/v2/config"
	"github.com/virel-project/virel-blockchain/v2/util"
)

const (
	TX_VERSION_INVALID = iota
	TX_VERSION_TRANSFER
	TX_VERSION_REGISTER_DELEGATE
	TX_VERSION_SET_DELEGATE
	TX_VERSION_STAKE
	TX_VERSION_UNSTAKE
)
const MAX_TX_VERSION = TX_VERSION_UNSTAKE

type TransactionData interface {
	AssociatedTransactionVersion() uint8
	Serialize(s *binary.Ser)
	Deserialize(d *binary.Des) error
	String() string
	VSize() uint64
	TotalAmount(tx *Transaction) (uint64, error)
	Prevalidate(tx *Transaction) error
	// The inputs of this transaction.
	// The sum of the amounts of this method must be equal to TransactionData's TotalAmount() + the transaction fee.
	StateInputs(tx *Transaction, signer address.Address) []StateInput
	// Adds the balances of this transaction data to the blockchain state.
	// The sum of the amounts of this method must be equal to TransactionData's TotalAmount().
	StateOutputs(tx *Transaction, signer address.Address) []StateOutput
}

// TransactionData: Transfer

type Transfer struct {
	Outputs []Output // transaction outputs
}

func (t *Transfer) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_TRANSFER
}
func (t *Transfer) Serialize(s *binary.Ser) {
	s.AddUvarint(uint64(len(t.Outputs)))
	for _, v := range t.Outputs {
		v.Serialize(s)
	}
}
func (t *Transfer) Deserialize(d *binary.Des) error {
	numOutputs := d.ReadUvarint()
	if numOutputs > config.MAX_OUTPUTS || numOutputs == 0 {
		return errors.New("too many outputs")
	}
	t.Outputs = make([]Output, numOutputs)
	for i, v := range t.Outputs {
		err := v.Deserialize(d)
		if err != nil {
			return err
		}
		t.Outputs[i] = v
	}
	return d.Error()
}
func (t *Transfer) String() string {
	o := "Transfer with outputs:\n"
	for _, v := range t.Outputs {
		o += " - " + v.String() + "\n"
	}
	return o
}
func (t *Transfer) VSize() uint64 {
	return uint64(len(t.Outputs)) * output_overhead
}
func (t *Transfer) TotalAmount(_ *Transaction) (uint64, error) {
	var s uint64 = 0

	for _, v := range t.Outputs {
		prev := s
		s += v.Amount

		// prevent overflow on outputs
		if prev > s {
			return 0, errors.New("overflow on output")
		}
	}

	return s, nil
}
func (t *Transfer) Prevalidate(_ *Transaction) error {
	// verify the number of outputs
	if len(t.Outputs) == 0 || len(t.Outputs) > config.MAX_OUTPUTS {
		return fmt.Errorf("invalid output count: %d", len(t.Outputs))
	}

	return nil
}
func (t *Transfer) StateInputs(tx *Transaction, sender address.Address) []StateInput {
	totamt, _ := t.TotalAmount(tx)
	return []StateInput{{
		Amount: totamt + tx.Fee,
		Sender: sender,
	}}
}
func (t *Transfer) StateOutputs(tx *Transaction, sender address.Address) []StateOutput {
	o := make([]StateOutput, len(t.Outputs))
	for i, v := range t.Outputs {
		o[i] = StateOutput{
			Recipient: v.Recipient,
			PaymentId: v.PaymentId,
			Amount:    v.Amount,
			Type:      OUT_NORMAL,
		}
	}
	return o
}

// TransactionData: RegisterDelegate

type RegisterDelegate struct {
	Name []byte
	Id   uint64
}

func (t *RegisterDelegate) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_REGISTER_DELEGATE
}
func (t *RegisterDelegate) Serialize(s *binary.Ser) {
	s.AddByteSlice(t.Name)
	s.AddUvarint(t.Id)
}
func (t *RegisterDelegate) Deserialize(d *binary.Des) error {
	t.Name = d.ReadByteSlice()
	t.Id = d.ReadUvarint()
	return d.Error()
}
func (t *RegisterDelegate) String() string {
	return "RegisterDelegate " + strconv.Quote(string(t.Name))
}
func (t *RegisterDelegate) TotalAmount(_ *Transaction) (uint64, error) {
	return config.REGISTER_DELEGATE_BURN, nil
}
func (t *RegisterDelegate) VSize() uint64 {
	return 16 + uint64(len(t.Name))
}
func (t *RegisterDelegate) Prevalidate(_ *Transaction) error {
	if len(t.Name) > 16 {
		return errors.New("RegisterDelegate name is too long")
	}
	if t.Id == 0 {
		return errors.New("delegate id 0 is not valid")
	}
	return nil
}
func (t *RegisterDelegate) StateInputs(tx *Transaction, sender address.Address) []StateInput {
	return []StateInput{{
		Type:   IN_NORMAL,
		Amount: tx.Fee + config.REGISTER_DELEGATE_BURN,
		Sender: sender,
	}}
}
func (t *RegisterDelegate) StateOutputs(tx *Transaction, sender address.Address) []StateOutput {
	return []StateOutput{{
		Type:      OUT_NORMAL,
		Amount:    config.REGISTER_DELEGATE_BURN,
		Recipient: address.INVALID_ADDRESS,
	}}
}

// TransactionData: SetDelegate

type SetDelegate struct {
	DelegateId       uint64
	PreviousDelegate uint64
}

func (t *SetDelegate) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_SET_DELEGATE
}
func (t *SetDelegate) Serialize(s *binary.Ser) {
	s.AddUvarint(t.DelegateId)
	s.AddUvarint(t.PreviousDelegate)
}
func (t *SetDelegate) Deserialize(d *binary.Des) error {
	t.DelegateId = d.ReadUvarint()
	t.PreviousDelegate = d.ReadUvarint()
	return d.Error()
}
func (t *SetDelegate) String() string {
	return fmt.Sprintf("SetDelegate: from %d to %d", t.PreviousDelegate, t.DelegateId)
}
func (t *SetDelegate) TotalAmount(_ *Transaction) (uint64, error) {
	return 0, nil
}
func (t *SetDelegate) VSize() uint64 {
	return config.MAX_TX_PER_BLOCK
}
func (t *SetDelegate) Prevalidate(_ *Transaction) error {
	return nil
}
func (t *SetDelegate) StateInputs(tx *Transaction, sender address.Address) []StateInput {
	return []StateInput{{
		Type:   IN_NORMAL,
		Amount: tx.Fee,
		Sender: sender,
	}}
}
func (t *SetDelegate) StateOutputs(tx *Transaction, sender address.Address) []StateOutput {
	return make([]StateOutput, 0)
}

// TransactionData: Stake

type Stake struct {
	Amount     uint64
	DelegateId uint64
}

func (t *Stake) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_STAKE
}
func (t *Stake) Serialize(s *binary.Ser) {
	s.AddUvarint(t.Amount)
	s.AddUvarint(t.DelegateId)
}
func (t *Stake) Deserialize(d *binary.Des) error {
	t.Amount = d.ReadUvarint()
	t.DelegateId = d.ReadUvarint()
	return d.Error()
}
func (t *Stake) String() string {
	return fmt.Sprintf("Stake: amount %s delegate %d", util.FormatCoin(t.Amount), t.DelegateId)
}
func (t *Stake) TotalAmount(_ *Transaction) (uint64, error) {
	return t.Amount, nil
}
func (t *Stake) VSize() uint64 {
	return 256 // for stake transactions, we use a bigger fee to reduce spam.
}
func (t *Stake) Prevalidate(_ *Transaction) error {
	if t.Amount < config.MIN_STAKE_AMOUNT {
		return fmt.Errorf("could not stake %s: must stake at least %s", util.FormatCoin(t.Amount), util.FormatCoin(config.MIN_STAKE_AMOUNT))
	}
	return nil
}
func (t *Stake) StateInputs(tx *Transaction, sender address.Address) []StateInput {
	return []StateInput{{
		Type:   IN_NORMAL,
		Sender: address.FromPubKey(tx.Signer),
		Amount: t.Amount + tx.Fee,
	}}
}
func (t *Stake) StateOutputs(tx *Transaction, sender address.Address) []StateOutput {
	return []StateOutput{{
		Type:      OUT_STAKE,
		Recipient: address.NewDelegateAddress(t.DelegateId),
		Amount:    t.Amount,
		ExtraData: t.DelegateId,
	}}
}

// TransactionData: Unstake

type Unstake struct {
	Amount     uint64
	DelegateId uint64
}

func (t *Unstake) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_UNSTAKE
}
func (t *Unstake) Serialize(s *binary.Ser) {
	s.AddUvarint(t.Amount)
	s.AddUvarint(t.DelegateId)
}
func (t *Unstake) Deserialize(d *binary.Des) error {
	t.Amount = d.ReadUvarint()
	t.DelegateId = d.ReadUvarint()
	return d.Error()
}
func (t *Unstake) String() string {
	return fmt.Sprintf("Unstake: amount %s delegate %d", util.FormatCoin(t.Amount), t.DelegateId)
}
func (t *Unstake) TotalAmount(tx *Transaction) (uint64, error) {
	if t.Amount < tx.Fee {
		return 0, fmt.Errorf("amount %d < tx fee %d", t.Amount, tx.Fee)
	}
	return t.Amount - tx.Fee, nil
}
func (t *Unstake) VSize() uint64 {
	return 8
}
func (t *Unstake) Prevalidate(tx *Transaction) error {
	if tx.Fee > t.Amount {
		return fmt.Errorf("transaction fee %s is bigger than the staked amount %s", util.FormatCoin(tx.Fee), util.FormatCoin(t.Amount))
	}

	return nil
}
func (t *Unstake) StateInputs(tx *Transaction, sender address.Address) []StateInput {
	return []StateInput{{
		Type:      IN_UNSTAKE,
		Sender:    address.NewDelegateAddress(t.DelegateId),
		Amount:    t.Amount,
		ExtraData: t.DelegateId,
	}}
}
func (t *Unstake) StateOutputs(tx *Transaction, sender address.Address) []StateOutput {
	if tx.Fee > t.Amount {
		panic(fmt.Errorf("unstake: fee %s > amount %s", util.FormatCoin(tx.Fee), util.FormatCoin(t.Amount)))
	}

	return []StateOutput{{
		Type:      OUT_NORMAL,
		Recipient: sender,
		Amount:    t.Amount - tx.Fee,
	}}
}
