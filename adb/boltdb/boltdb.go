package boltdb

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/virel-project/virel-blockchain/v3/adb"

	bolt "go.etcd.io/bbolt"
)

var _ adb.DB = &DB{}

type DB struct {
	db *bolt.DB
}

func New(dbpath string, filemode os.FileMode) (*DB, error) {
	var err error

	d := &DB{}

	dbpath, err = filepath.Abs(dbpath)
	if err != nil {
		return nil, err
	}

	d.db, err = bolt.Open(dbpath, filemode, &bolt.Options{
		NoFreelistSync: true,
	})

	return d, err
}

func (d *DB) Index(name string) adb.Index {
	err := d.db.Update(func(txn *bolt.Tx) error {
		var err error
		_, err = txn.CreateBucketIfNotExists([]byte(name))
		return err
	})
	if err != nil {
		panic(err)
	}

	return []byte(name)
}

func (d *DB) View(f func(txn adb.Txn) error) error {
	return d.db.View(func(t *bolt.Tx) error {
		txn := &Txn{
			txn: t,
		}

		return f(txn)
	})
}

func (d *DB) Update(f func(txn adb.Txn) error) error {
	return d.db.Update(func(t *bolt.Tx) error {
		txn := &Txn{
			txn: t,
		}

		return f(txn)
	})
}

func (d *DB) Close() error {
	return d.db.Close()
}

type Txn struct {
	txn *bolt.Tx
}

func (t *Txn) Get(d adb.Index, key []byte) []byte {
	dbi := d.([]byte)
	r := t.txn.Bucket(dbi).Get(key)
	return r
}

func (t *Txn) Put(d adb.Index, key []byte, value []byte) error {
	dbi := d.([]byte)
	return t.txn.Bucket(dbi).Put(key, value)
}

func (t *Txn) Del(d adb.Index, key []byte) error {
	dbi := d.([]byte)
	return t.txn.Bucket(dbi).Delete(key)
}

func (t *Txn) ForEach(d adb.Index, f func(k, v []byte) error) error {
	return t.txn.Bucket(d.([]byte)).ForEach(f)
}
func (t *Txn) ForEachInterrupt(d adb.Index, f func(k, v []byte) (bool, error)) error {
	b := t.txn.Bucket(d.([]byte))

	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		interrupt, err := f(k, v)
		if err != nil {
			return err
		}
		if interrupt {
			break
		}
	}

	return nil
}

func (t *Txn) Entries(d adb.Index) (uint64, error) {
	buck := t.txn.Bucket(d.([]byte))

	if buck == nil {
		return 0, fmt.Errorf("bucket not found")
	}

	s := buck.Stats()

	return uint64(s.KeyN), nil
}
