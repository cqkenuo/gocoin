package btc

import (
	"os"
	"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/others/sys"
)


// Used during BrowseUTXO()
const (
	WALK_ABORT   = 0x00000001 // Abort browsing
	WALK_NOMORE  = 0x00000002 // Do not browse through it anymore
)
type FunctionWalkUnspent func(*qdb.DB, qdb.KeyType, *OneWalkRecord) uint32

// Used to pass block's changes to UnspentDB
type BlockChanges struct {
	Height uint32
	LastKnownHeight uint32  // put here zero to disable this feature
	AddedTxs map[TxPrevOut] *TxOut
	DeledTxs map[TxPrevOut] *TxOut
}


type UnspentDB struct {
	unspent *unspentDb
	unwind *unwindDb
}


func NewUnspentDb(dir string, init bool, ch *Chain) *UnspentDB {
	db := new(UnspentDB)

	if init {
		os.RemoveAll(dir+"unspent3")
	}

	if AbortNow {
		return nil
	}
	db.unwind = newUnwindDB(dir+"unspent3"+string(os.PathSeparator))

	if AbortNow {
		return nil
	}

	db.unspent = newUnspentDB(dir+"unspent3"+string(os.PathSeparator), db.unwind.lastBlockHeight, ch)

	return db
}


// The name is self explaining
func (db *UnspentDB) GetLastBlockHash() ([]byte) {
	return db.unwind.GetLastBlockHash()
}


// Commit the given add/del transactions to UTXO and Wnwind DBs
func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	// First the unwind data
	db.nosync()
	db.unspent.lastHeight = changes.Height
	db.unwind.commit(changes, blhash)
	db.unspent.commit(changes)
	if changes.Height >= changes.LastKnownHeight {
		db.Sync()
	}
	sys.FreeMem()
	return
}


// Return DB statistics
func (db *UnspentDB) GetStats() (s string) {
	s += db.unspent.stats()
	s += db.unwind.stats()
	return
}


// Flush all the data to files
func (db *UnspentDB) Sync() {
	db.unwind.sync()
	db.unspent.sync()
}


// Hold on writing data to disk untill next sync is called
func (db *UnspentDB) nosync() {
	db.unwind.nosync()
	db.unspent.nosync()
}


// Flush the data and close all the files
func (db *UnspentDB) Close() {
	db.unwind.close()
	db.unspent.close()
}


// Call it when the main thread is idle - this will do DB defrag
func (db *UnspentDB) Idle() bool {
	if db.unspent.idle() {
		return true
	}
	return db.unwind.idle()
}


// Flush all the data to disk
func (db *UnspentDB) Save() {
	db.unwind.save()
	db.unspent.save()
}

func (db *UnspentDB) UndoBlockTransactions(height uint32) {
	db.nosync()
	db.unwind.undo(height, db.unspent)
	db.unspent.lastHeight = height-1
	db.Sync()
}


// Get ne unspent output
func (db *UnspentDB) UnspentGet(po *TxPrevOut) (res *TxOut, e error) {
	return db.unspent.get(po)
}


// Browse through all unspent outputs
func (db *UnspentDB) BrowseUTXO(quick bool, walk FunctionWalkUnspent) {
	db.unspent.browse(walk, quick)
}
