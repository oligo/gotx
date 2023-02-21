package gotx

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// const (
// 	contextTxKey            = "txManager:tx"
// 	contextRootTxContextKey = "txManager:rootTxCtx"
// )

// TxManager implements a basic transaction manager
type TxManager struct {
	db    *sqlx.DB
	mux   sync.RWMutex
	txMap map[uint64][]*Transaction
}

func NewTxManager(db *sqlx.DB) *TxManager {
	return &TxManager{
		db:    db,
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
	defer func() {
		for _, t := range tm.currentTXs(goid) {
			// children tx can not be in NotFinished status
			if !t.isDone() && t.err != nil {
				err := t.rollback()
				if err != nil {
					log.Println(err)
				}
			}
		}
	}()

	log.Printf("Before: goroutine ID: %d, txmap: %v", goid, tm.currentTXs(goid))

	trans.execTxFunc(txFunc)

	// If this logical transaction has errors, we rollback it,
	// and this will rollback the physical transaction.
	if trans.err != nil {
		err := trans.rollback()
		if err != nil {
			return err
		}
		return trans.err
	} else {
		return trans.commit()
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
	}
}

func (tm *TxManager) remove(trans *Transaction) {
	goid := curGoroutineID()
	tm.removeTx(goid, trans)
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
			rootTx := tm.newTx(ctx, nil, options)
			tm.appendTx(goid, rootTx)
			return rootTx
		}

		rootTx := tm.currentTXs(goid)[0]
		trans = tm.newTx(ctx, rootTx, options)

	default:
		panic("Unknown propagation type: " + fmt.Sprintf("%d", options.Propagation))
	}

	tm.appendTx(goid, trans)

	return trans
}

func (tm *TxManager) newTx(ctx context.Context, rootTx *Transaction, options *Options) *Transaction {
	txID, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}

	if rootTx != nil {
		return NewTx(rootTx.tx, txID.String(), tm)
	}

	dbTx := newRawTx(tm.db.MustBeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead}))

	return NewTx(dbTx, txID.String(), tm)

}
