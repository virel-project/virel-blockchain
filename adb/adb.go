package adb

type DB interface {
	Index(string) Index

	View(func(txn Txn) error) error
	Update(func(txn Txn) error) error
	Close() error
}

type Index any

type Txn interface {
	Get(Index, []byte) []byte
	Put(Index, []byte, []byte) error
	Del(Index, []byte) error
	ForEach(Index, func(k, v []byte) error) error
	ForEachInterrupt(Index, func(k, v []byte) (bool, error)) error
	Entries(Index) (uint64, error)
}
