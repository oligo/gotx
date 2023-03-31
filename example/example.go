// This package shows useage of gotx

package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/oligo/gotx"
)

// account model as an example here.
// We use db tag here to do column mapping
type Account struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

func main() {
	//using a DNS for mysql here
	dsn := "root:12345@tcp(127.0.0.1:3306)/db"

	db, err := sqlx.Connect("mysql", dsn)

	if err != nil {
		panic(err)
	}

	txManager := gotx.NewTxManager(db)

	// declares the tx options here. Please note that if options is omitted,
	// the default option is used. By default we use PropagationRequired for propagation
	// and RepeatableRead for isolation level.
	opts := &gotx.Options{
		Propagation:    gotx.PropagationRequired,
		IsolationLevel: sql.LevelRepeatableRead,
	}

	// use userID=10 for example
	userID := 10
	var account Account
	_ = txManager.Exec(context.Background(), func(tx *gotx.Transaction) error {
		// for more query method, please consult the godoc of this project
		err := tx.GetOne(&account, "select id, name from account where id=?", userID)
		if err != nil {
			fmt.Println(err)
		}
		// Any error returned in this function will trigger the rollback of the transaction.
		return err

	}, opts)

}
