package gotx

import (
	"errors"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/jmoiron/sqlx"
)

var (
	// ErrInvalidTxState is returned when transaction is not initialized
	ErrInvalidTxState = errors.New("gotx: tx is already committed or rolled back")
)

type rawTx struct {
	*sqlx.Tx

	// a flag that marks the tx as committed or rolled back
	// if raw tx is done, repeated commit/rollback will return error
	// done bool
	// A counter that tracks how many logical transactions use this tx
	refCount uint32
}

func newRawTx(tx *sqlx.Tx) *rawTx {
	return &rawTx{Tx: tx}
}

// Transaction is a logical transaction which wraps a underlying db transaction (physical transaction)
type Transaction struct {
	// tx is the underlying physical transaction
	tx   *rawTx
	txID string
	err  error

	// committed marks this logical tx is commited. Later commit operation is not allowed
	committed bool

	// reference to tx manager
	txManager *TxManager
	// ctx       context.Context

	// requiredNew marks if this transaction is created from a new db tx or not
	requiredNew bool
}

func NewTx(t *rawTx, txID string, requiredNew bool, manager *TxManager) *Transaction {
	trans := &Transaction{
		tx:          t,
		txID:        txID,
		txManager:   manager,
		requiredNew: requiredNew,
		committed:   false,
	}

	atomic.AddUint32(&trans.tx.refCount, 1)

	return trans
}

func (t *Transaction) String() string {
	return fmt.Sprintf("tx-%s", t.txID)
}

func (t *Transaction) setError(err error) {
	t.err = err
}

func (t *Transaction) checkState() error {
	if t.tx == nil || t.committed {
		return ErrInvalidTxState
	}

	return nil
}

func (t *Transaction) Commit() error {
	t.txManager.Remove(t)
	var err error

	if t.requiredNew {
		err = t.tx.Commit()
	} else {
		// decrease refCount by one
		leftRefs := atomic.AddUint32(&t.tx.refCount, ^uint32(0))
		// If refCount decreases to zero, do the real commit
		if leftRefs <= 0 {
			err = t.tx.Commit()
		}
	}

	t.committed = true
	log.Printf("%s committed\n", t)
	return err
}

// rollback always do the real rollback. For tx binding to a unique db tx(requiredNew is true),
// rollback do the db rollback directly. For tx sharing a db tx, rollback do rollback only once.
func (t *Transaction) Rollback() error {
	var err error
	if t.requiredNew {
		t.txManager.Remove(t)
		atomic.AddUint32(&t.tx.refCount, ^uint32(0))
		err = t.tx.Rollback()
	} else {
		t.txManager.RemoveAll()
		if atomic.LoadUint32(&t.tx.refCount) > 0 {
			atomic.SwapUint32(&t.tx.refCount, 0)
			err = t.tx.Rollback()
		}
	}

	if err != nil {
		return err
	}

	log.Printf("%s rolledback\n", t)
	return nil
}

func (t *Transaction) execTxFunc(txFunc func(tx *Transaction) error) {
	err := txFunc(t)

	if err != nil {
		t.setError(err)
	}
}

// GetOne is the sqlx.Get wrapper
func (t *Transaction) GetOne(dest interface{}, query string, args ...interface{}) error {
	if err := t.checkState(); err != nil {
		return err
	}

	// dest should be a pointer to a struct/map
	err := t.tx.Get(dest, query, args...)
	if err != nil {
		return err
	}

	return nil
}

// Insert implements sql insert logic and returns generated ID
func (t *Transaction) Insert(query string, arg interface{}) (int64, error) {
	if err := t.checkState(); err != nil {
		return 0, err
	}

	result, err := t.tx.NamedExec(query, arg)
	if err != nil {
		return 0, fmt.Errorf("insert failed: %w", err)
	}

	resultID, err := result.LastInsertId()

	if err != nil {
		return 0, fmt.Errorf("insert failed: %w", err)
	}

	return resultID, nil
}

func (t *Transaction) Select(dest interface{}, query string, args ...interface{}) error {
	if err := t.checkState(); err != nil {
		return err
	}

	err := t.tx.Select(dest, query, args...)

	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	return nil

}

// Update execute a update sql using sqlx NamedExec. Docs from sqlx doc:
//
// Named queries are common to many other database packages. They allow you to use a bindvar syntax which refers
// to the names of struct fields or map keys to bind variables a query, rather than having to refer to everything
// positionally. The struct field naming conventions follow that of StructScan, using the NameMapper and the db struct tag.
func (t *Transaction) Update(query string, arg interface{}) (int64, error) {

	if err := t.checkState(); err != nil {
		return 0, err
	}

	result, err := t.tx.NamedExec(query, arg)
	if err != nil {
		return 0, fmt.Errorf("update failed: %w", err)
	}

	updatedRows, err := result.RowsAffected()

	if err != nil {
		return 0, fmt.Errorf("update entity failed: %w", err)
	}

	return updatedRows, nil
}

func (t *Transaction) Delete(query string, arg interface{}) error {
	if err := t.checkState(); err != nil {
		return err
	}

	result, err := t.tx.NamedExec(query, arg)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	deletedRows, err := result.RowsAffected()

	if err != nil || deletedRows <= 0 {
		return fmt.Errorf("delete entity failed: %w", err)
	}

	if deletedRows <= 0 {
		log.Printf("delete entity failed: %s", err)
	}

	return nil
}
