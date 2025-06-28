package lmdb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"virel-blockchain/adb"
	"virel-blockchain/logger"

	lmdb "github.com/PowerDNS/lmdb-go/lmdb"
)

var _ adb.DB = &DB{}

type DB struct {
	env *lmdb.Env

	log *logger.Log

	resizeLock sync.Mutex
}

func New(dbpath string, filemode os.FileMode, log *logger.Log) (*DB, error) {
	var err error

	d := &DB{
		log: log,
	}

	d.env, err = lmdb.NewEnv()
	if err != nil {
		return nil, err
	}

	d.env.SetMaxDBs(16)
	d.env.SetMapSize(512 * 1024)
	d.env.SetFlags(lmdb.WriteMap)

	dbpath, err = filepath.Abs(dbpath)
	if err != nil {
		return nil, err
	}

	err = os.Mkdir(dbpath, filemode)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	err = verifyDirPermissions(dbpath)
	if err != nil {
		return nil, err
	}

	// we currently have NoMetaSync (database durability not guaranteed since it's not critical here)
	// TODO: make this configurable
	err = d.env.Open(dbpath, lmdb.NoMetaSync, filemode)
	if err != nil {
		d.env.Close()
		return nil, err
	}

	return d, nil
}

func verifyDirPermissions(path string) error {
	// Check if directory exists and is writable
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("directory access error: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}

	// Test write permission
	testFile := filepath.Join(path, "permission_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("write permission denied: %w", err)
	}
	os.Remove(testFile)

	return nil
}

func (d *DB) Index(name string) (dbi adb.Index) {
	err := d.env.Update(func(txn *lmdb.Txn) error {
		var err error
		dbi, err = txn.CreateDBI(name)
		return err
	})
	if err != nil {
		panic(err)
	}

	return
}

func (d *DB) View(f func(txn adb.Txn) error) error {
	return d.env.View(func(t *lmdb.Txn) error {
		txn := &Txn{
			txn: t,
		}

		return f(txn)
	})
}

const MAX_RESIZE_ATTEMPTS = 4
const GB = 1024 * 1024 * 1024

func (d *DB) Update(f func(txn adb.Txn) error) error {

	info, err := d.env.Info()
	if err != nil {
		return err
	}

	stat, err := d.env.Stat()
	if err != nil {
		return err
	}

	size_used := int64(stat.PSize) * info.LastPNO

	free := 1 - float64(size_used)/float64(info.MapSize)

	if free < 0.1 {
		err := func() error {
			d.resizeLock.Lock()
			defer d.resizeLock.Unlock()

			newSize := info.MapSize * 2
			// increment at most by 1 GiB
			if info.MapSize > 1*GB {
				newSize = info.MapSize + GB
			}

			d.log.Infof("LMDB mapsize increase needed: %vMiB -> %vMiB", float64(info.MapSize)/1024/1024, float64(newSize)/1024/1024)

			return d.env.SetMapSize(newSize)
		}()
		if err != nil {
			return err
		}
	}

	return d.env.Update(func(t *lmdb.Txn) error {
		txn := &Txn{
			txn: t,
		}

		return f(txn)
	})
}

func (d *DB) Close() error {
	return d.env.Close()
}

type Txn struct {
	txn *lmdb.Txn
}

func (t *Txn) Get(d adb.Index, key []byte) []byte {
	dbi := d.(lmdb.DBI)
	r, err := t.txn.Get(dbi, key)
	if err != nil {
		return nil
	}
	return r
}

func (t *Txn) Put(d adb.Index, key []byte, value []byte) error {
	dbi := d.(lmdb.DBI)
	return t.txn.Put(dbi, key, value, 0)
}

func (t *Txn) Del(d adb.Index, key []byte) error {
	dbi := d.(lmdb.DBI)
	return t.txn.Del(dbi, key, nil)
}

func (t *Txn) ForEach(d adb.Index, f func(k, v []byte) error) error {
	dbi := d.(lmdb.DBI)
	cursor, err := t.txn.OpenCursor(dbi)
	if err != nil {
		return err
	}

	defer cursor.Close()

	// Iterate through all key-value pairs
	for {
		key, value, err := cursor.Get(nil, nil, lmdb.Next)
		if lmdb.IsNotFound(err) {
			// End of iteration
			break
		}
		if err != nil {
			return fmt.Errorf("cursor get: %w", err)
		}

		err = f(key, value)
		if err != nil {
			return err
		}
	}

	return nil
}
