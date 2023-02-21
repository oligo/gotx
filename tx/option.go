package gotx

// PropagationType is an alias of uint8
type PropagationType uint8

// constants that defines transaction propagation patterns
const (
	// Required specifies the txFunc will be run in an existing db tx. If there's no existing tx,
	// tx manager will create a new one for it.
	PropagationRequired = PropagationType(0)

	// New specifies the txFunc will be run in a separated new db tx.
	PropagationNew = PropagationType(1)
)

// Options declares some configurable options when starts a transaction
type Options struct {
	// PropagationType specifies how the tx manager manages transaction propagation
	Propagation PropagationType
}

func defaultOptions() *Options {
	return &Options{
		Propagation: PropagationRequired,
	}
}
