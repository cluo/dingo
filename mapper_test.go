package dingo

import (
	"testing"

	"github.com/mission-liao/dingo/broker"
	"github.com/mission-liao/dingo/common"
	"github.com/mission-liao/dingo/transport"
	"github.com/stretchr/testify/suite"
)

type MapperTestSuite struct {
	suite.Suite

	_mps            *_mappers
	_invoker        transport.Invoker
	_tasks          chan *transport.Task
	_countOfMappers int
	_receiptsMux    *common.Mux
	_receipts       chan *broker.Receipt
}

func TestMapperSuite(t *testing.T) {
	suite.Run(t, &MapperTestSuite{
		_tasks:          make(chan *transport.Task, 5),
		_countOfMappers: 3,
		_invoker:        transport.NewDefaultInvoker(),
		_receiptsMux:    common.NewMux(),
		_receipts:       make(chan *broker.Receipt, 1),
	})
}

func (me *MapperTestSuite) SetupSuite() {
	var err error
	me._mps, err = newMappers()
	me.Nil(err)

	// allocate 3 mapper routines
	for remain := me._countOfMappers; remain > 0; remain-- {
		receipts := make(chan *broker.Receipt, 10)
		me._mps.more(me._tasks, receipts)
		_, err := me._receiptsMux.Register(receipts, 0)
		me.Nil(err)
	}
	remain, err := me._receiptsMux.More(3)
	me.Equal(0, remain)
	me.Nil(err)

	me._receiptsMux.Handle(func(val interface{}, _ int) {
		me._receipts <- val.(*broker.Receipt)
	})
}

func (me *MapperTestSuite) TearDownSuite() {
	me.Nil(me._mps.Close())
	close(me._tasks)
	me._receiptsMux.Close()
	close(me._receipts)
}

//
// test cases
//

func (me *MapperTestSuite) TestParellelMapping() {
	// make sure those mapper routines would be used
	// when one is blocked.

	// the bottleneck of mapper are:
	// - length of receipt channel
	// - count of mapper routines
	count := me._countOfMappers + cap(me._tasks)
	stepIn := make(chan int, count)
	stepOut := make(chan int, count)
	reports, remain, err := me._mps.allocateWorkers("ParellelMapping", func(i int) {
		stepIn <- i
		// workers would be blocked here
		<-stepOut
	}, 1, 0)
	me.Nil(err)
	me.Equal(0, remain)
	me.Len(reports, 1)

	// send enough tasks to fill mapper routines & tasks channel
	for i := 0; i < count; i++ {
		// compose corresponding task
		t, err := me._invoker.ComposeTask("ParellelMapping", []interface{}{i})
		me.Nil(err)

		// should not be blocked here
		me._tasks <- t
	}

	// unless worked as expected, or we won't reach
	// this line
	rets := []int{}
	for i := 0; i < count; i++ {
		// consume 1 receipts
		<-me._receipts

		// consume 2 report
		<-reports[0]
		<-reports[0]

		// let 1 worker get out
		stepOut <- 1

		// consume another report
		<-reports[0]

		rets = append(rets, <-stepIn)
	}

	me.Len(rets, count)
}
