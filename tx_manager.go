package gotx

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"sync"

	"github.com/jmoiron/sqlx"
)

const bytesForKey = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generateRandomKey(size int) string {
	keyBytes := make([]byte, size)
	for i := range keyBytes {
		keyBytes[i] = bytesForKey[rand.Intn(len(bytesForKey))]
	}

	return string(keyBytes)
}

// TxManager implements a basic transaction manager
type TxManager struct {
	db    *sqlx.DB
	mux   *sync.Mutex
	txMap map[uint64][]*Transaction
}

func NewTxManager(db *sqlx.DB) *TxManager {
	return &TxManager{
		db:    db,
		mux:   &sync.Mutex{},
		txMap: make(map[uint64][]*Transaction),
	}
}

func (tm *TxManager) Exec(ctx context.Context, txFunc func(tx *Transaction) error, options *Options) error {
	if ctx == nil {
		panic("context must not be nil")
	}

	var opt *Options

	if options == nil {
		opt = defaultOptions()
	} else {
		opt = options
	}

	log.Printf("Tx caller: %s\n", getCaller())
	goid := curGoroutineID()
	trans := tm.startTx(ctx, goid, opt)

	// rollback the tx when this Exec function panics before tx is committed or rolled back.
	defer func(id uint64) {
		if r := recover(); r != nil {
			for _, t := range tm.currentTXs(id) {
				err := t.Rollback()
				if err != nil {
					log.Printf("rollback failure: %+v", err)
				}

			}
		}
	}(goid)

	log.Printf("tx started in goroutine[%d], nested logical tx: %v", goid, tm.currentTXs(goid))

	trans.execTxFunc(txFunc)

	// If this logical transaction has errors, we rollback it,
	// and this will rollback the physical transaction.
	if trans.err != nil {
		err := trans.Rollback()
		if err != nil {
			return err
		}
		return trans.err
	} else {
		return trans.Commit()
	}
}

func (tm *TxManager) currentTXs(goid uint64) []*Transaction {
	if txMap, ok := tm.txMap[goid]; !ok {
		if txMap == nil {
			tm.txMap[goid] = make([]*Transaction, 0)
		}
	}

	return tm.txMap[goid]
}

func (tm *TxManager) appendTx(goid uint64, trans *Transaction) {
	tm.mux.Lock()
	defer tm.mux.Unlock()
	tm.txMap[goid] = append(tm.txMap[goid], trans)
}

func (tm *TxManager) removeTx(goid uint64, trans *Transaction) {
	for idx, t := range tm.txMap[goid] {
		if t == trans {
			prev := make([]*Transaction, 0)
			prev = append(prev, tm.txMap[goid][:idx]...)
			tm.txMap[goid] = append(prev, tm.txMap[goid][idx+1:]...)
			break
		}
	}

	if len(tm.txMap[goid]) == 0 {
		delete(tm.txMap, goid)
		log.Printf("tx %s in GOROUTINE %d removed", trans, goid)
	}

}

func (tm *TxManager) Remove(trans *Transaction) {
	tm.mux.Lock()
	defer tm.mux.Unlock()
	goid := curGoroutineID()
	tm.removeTx(goid, trans)
}

func (tm *TxManager) RemoveAll() {
	tm.mux.Lock()
	defer tm.mux.Unlock()
	goid := curGoroutineID()
	delete(tm.txMap, goid)
}

func (tm *TxManager) startTx(ctx context.Context, goid uint64, options *Options) *Transaction {
	var trans *Transaction

	switch options.Propagation {
	case PropagationNew:
		// new db tx is requested
		trans = tm.newTx(ctx, nil, options)

	case PropagationRequired:
		// sharing the same physical transaction with root tx
		if txMap := tm.currentTXs(goid); len(txMap) == 0 {
			trans = tm.newTx(ctx, nil, options)
			// tm.appendTx(goid, rootTx)
			// return rootTx
		} else {
			rootTx := tm.currentTXs(goid)[0]
			trans = tm.newTx(ctx, rootTx, options)
		}

	default:
		panic("Unknown propagation type: " + fmt.Sprintf("%d", options.Propagation))
	}

	tm.appendTx(goid, trans)
	log.Printf("%s started\n", trans)
	return trans
}

func (tm *TxManager) newTx(ctx context.Context, rootTx *Transaction, options *Options) *Transaction {
	// txID, err := uuid.NewRandom()
	// if err != nil {
	// 	panic(err)
	// }

	txID := generateRandomKey(10)

	if rootTx != nil {
		return NewTx(rootTx.tx, txID, options.Propagation == PropagationNew, tm)
	}

	dbTx := newRawTx(tm.db.MustBeginTx(ctx, &sql.TxOptions{Isolation: options.IsolationLevel}))

	return NewTx(dbTx, txID, options.Propagation == PropagationNew, tm)

}
