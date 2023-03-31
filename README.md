# gotx


[![Go Reference](https://pkg.go.dev/badge/github.com/oligo/gotx.svg)](https://pkg.go.dev/github.com/oligo/gotx)


A transaction manager based on sqlx(github.com/jmoiron/sqlx) for Golang. Since it is based on sqlx, the query methods in gotx(GetOne, Select, Insert, etc.) are similar to those in sqlx.

GoTx manages logical transactions binded to a goroutine in a group when the propagation type is set to `PropagationRequired`. These logical transactions are then binded to one underlying database transaction. GoTx uses goroutine ID to track logical transactions in a group.

When the propagation type is set to `PropagationNew`. Each logical transaction is binded to one underlying database transaction.  

In the case of `PropagationRequired`, GoTx supports nested logical transactions so please feel free to call other transaction wrapped functions/methods from a transaction function/method in your service layer. With a nested transaction a commit does not persist changes until the top level transaction commit. While a rollback works regardless of the level of the transactions. That is when a rollback is triggered, all changes of nested logical transactions are discarded.  When using `PropagationNew` every transaction do commit and rollback independently.

NOT READY FOR PRODUCTION USE. PLEASE USE IT AT YOUR OWN RISK.

## install

    go get github.com/oligo/gotx


## usage

```go

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

```

## Issues

TODO.
