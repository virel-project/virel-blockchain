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

type TransactionData interface {
	AssociatedTransactionVersion() uint8
	Serialize(s *binary.Ser)
	Deserialize(d *binary.Des) error
	String() string
	VSize() uint64
	TotalAmount() (uint64, error)
	Prevalidate(tx *Transaction) error
	// The inputs of this transaction.
	// The sum of the amounts of this method must be equal to TransactionData's TotalAmount() + the transaction fee.
	StateInputs(tx *Transaction, signer address.Address) []Input
	// Adds the balances of this transaction data to the blockchain state.
	// The sum of the amounts of this method must be equal to TransactionData's TotalAmount().
	StateOutputs(tx *Transaction, signer address.Address) []Output
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
func (t *Transfer) TotalAmount() (uint64, error) {
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
func (t *Transfer) StateInputs(tx *Transaction, sender address.Address) []Input {
	totamt, _ := t.TotalAmount()
	return []Input{{
		Amount: totamt + tx.Fee,
		Sender: sender,
	}}
}
func (t *Transfer) StateOutputs(tx *Transaction, sender address.Address) []Output {
	return t.Outputs
}

// TransactionData: RegisterDelegate

type RegisterDelegate struct {
	Name []byte
}

func (t *RegisterDelegate) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_REGISTER_DELEGATE
}
func (t *RegisterDelegate) Serialize(s *binary.Ser) {
	s.AddByteSlice(t.Name)
}
func (t *RegisterDelegate) Deserialize(d *binary.Des) error {
	t.Name = d.ReadByteSlice()
	return d.Error()
}
func (t *RegisterDelegate) String() string {
	return "RegisterDelegate " + strconv.Quote(string(t.Name))
}
func (t *RegisterDelegate) TotalAmount() (uint64, error) {
	return 0, nil
}
func (t *RegisterDelegate) VSize() uint64 {
	return 16 + uint64(len(t.Name))
}
func (t *RegisterDelegate) Prevalidate(_ *Transaction) error {
	if len(t.Name) > 16 {
		return errors.New("RegisterDelegate name is too long")
	}
	return nil
}
func (t *RegisterDelegate) StateInputs(tx *Transaction, sender address.Address) []Input {
	return []Input{{
		Amount: tx.Fee + config.REGISTER_DELEGATE_BURN,
		Sender: sender,
	}}
}
func (t *RegisterDelegate) StateOutputs(tx *Transaction, sender address.Address) []Output {
	return []Output{{
		Amount:    config.REGISTER_DELEGATE_BURN,
		Recipient: address.INVALID_ADDRESS,
	}}
}

// TransactionData: SetDelegate

type SetDelegate struct {
	DelegateId uint64
}

func (t *SetDelegate) AssociatedTransactionVersion() uint8 {
	return TX_VERSION_SET_DELEGATE
}
func (t *SetDelegate) Serialize(s *binary.Ser) {
	s.AddUvarint(t.DelegateId)
}
func (t *SetDelegate) Deserialize(d *binary.Des) error {
	t.DelegateId = d.ReadUvarint()
	return d.Error()
}
func (t *SetDelegate) String() string {
	return fmt.Sprintf("SetDelegate: %d", t.DelegateId)
}
func (t *SetDelegate) TotalAmount() (uint64, error) {
	return 0, nil
}
func (t *SetDelegate) VSize() uint64 {
	return 4
}
func (t *SetDelegate) Prevalidate(_ *Transaction) error {
	return nil
}
func (t *SetDelegate) StateInputs(tx *Transaction, sender address.Address) []Input {
	return []Input{{
		Amount: tx.Fee,
		Sender: sender,
	}}
}
func (t *SetDelegate) StateOutputs(tx *Transaction, sender address.Address) []Output {
	return make([]Output, 0)
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
func (t *Stake) TotalAmount() (uint64, error) {
	return t.Amount * config.MIN_STAKE_AMOUNT, nil
}
func (t *Stake) VSize() uint64 {
	return 8
}
func (t *Stake) Prevalidate(_ *Transaction) error {
	return nil
}
func (t *Stake) StateInputs(tx *Transaction, sender address.Address) []Input {
	return []Input{{
		Sender: address.FromPubKey(tx.Signer),
		Amount: t.Amount*config.MIN_STAKE_AMOUNT + tx.Fee,
	}}
}
func (t *Stake) StateOutputs(tx *Transaction, sender address.Address) []Output {
	return []Output{{
		Recipient: address.NewDelegateAddress(t.DelegateId),
		Amount:    t.Amount * config.MIN_STAKE_AMOUNT,
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
func (t *Unstake) TotalAmount() (uint64, error) {
	return t.Amount * config.MIN_STAKE_AMOUNT, nil
}
func (t *Unstake) VSize() uint64 {
	return 8
}
func (t *Unstake) Prevalidate(tx *Transaction) error {
	if tx.Fee > t.Amount*config.MIN_STAKE_AMOUNT {
		return fmt.Errorf("transaction fee %s is bigger than the staked amount %d", util.FormatCoin(tx.Fee), t.Amount)
	}

	return nil
}
func (t *Unstake) StateInputs(tx *Transaction, sender address.Address) []Input {
	return []Input{{
		Sender: address.NewDelegateAddress(t.DelegateId),
		Amount: t.Amount * config.MIN_STAKE_AMOUNT,
	}}
}
func (t *Unstake) StateOutputs(tx *Transaction, sender address.Address) []Output {
	if tx.Fee > t.Amount*config.MIN_STAKE_AMOUNT {
		panic(fmt.Errorf("unstake: fee %s > amount %s", util.FormatCoin(tx.Fee), util.FormatCoin(t.Amount*config.MIN_STAKE_AMOUNT)))
	}

	return []Output{{
		Recipient: sender,
		Amount:    t.Amount*config.MIN_STAKE_AMOUNT - tx.Fee,
	}}
}
