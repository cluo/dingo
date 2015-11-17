package dingo

import (
	"github.com/mission-liao/dingo/backend"
	"github.com/mission-liao/dingo/broker"
	"github.com/mission-liao/dingo/common"
	"github.com/mission-liao/dingo/transport"
)

type Bridge interface {
	Close() (err error)
	Events() ([]<-chan *common.Event, error)

	//
	// Register function pointer for fixing returns
	Register(name string, fn interface{}) (err error)

	//
	// proxy for broker.Producer
	//
	SendTask(t *transport.Task) (err error)

	//
	// proxy for broker.Consumer
	//
	AddNamedListener(name string, receipts <-chan *broker.Receipt) (tasks <-chan *transport.Task, err error)
	AddListener(rcpt <-chan *broker.Receipt) (tasks <-chan *transport.Task, err error)
	StopAllListeners() (err error)

	//
	// proxy for backend.Reporter
	//
	Report(reports <-chan *transport.Report) (err error)

	//
	// proxy for backend.Store
	//
	Poll(t *transport.Task) (reports <-chan *transport.Report, err error)

	//
	// setter
	//
	AttachReporter(r backend.Reporter) (err error)
	AttachStore(s backend.Store) (err error)
	AttachProducer(p broker.Producer) (err error)
	AttachConsumer(c broker.Consumer, nc broker.NamedConsumer) (err error)

	//
	// existence checker
	//
	Exists(it int) bool
}

func NewBridge(which string, m *transport.Marshallers, args ...interface{}) Bridge {
	switch which {
	case "local":
		return newLocalBridge(args...)
	default:
		return newDefaultBridge(m)
	}

	return nil
}