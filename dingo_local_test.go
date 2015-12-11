package dingo

import (
	"encoding/json"
	"testing"

	"github.com/mission-liao/dingo/backend"
	"github.com/mission-liao/dingo/broker"
	"github.com/mission-liao/dingo/common"
	"github.com/mission-liao/dingo/transport"
	"github.com/stretchr/testify/suite"
)

//
// local(Broker) + local(Backend)
//

type localTestSuite struct {
	DingoTestSuite
}

func (me *localTestSuite) SetupSuite() {
	me.DingoTestSuite.SetupSuite()

	// broker
	{
		v, err := broker.New("local", me.cfg.Broker())
		me.Nil(err)
		_, used, err := me.app.Use(v, common.InstT.DEFAULT)
		me.Nil(err)
		me.Equal(common.InstT.PRODUCER|common.InstT.CONSUMER, used)
	}

	// backend
	{
		v, err := backend.New("local", me.cfg.Backend())
		me.Nil(err)
		_, used, err := me.app.Use(v, common.InstT.DEFAULT)
		me.Nil(err)
		me.Equal(common.InstT.REPORTER|common.InstT.STORE, used)
	}

	me.Nil(me.app.Init(*me.cfg))
}

func TestDingoLocalSuite(t *testing.T) {
	suite.Run(t, &localTestSuite{})
}

//
// test case
//
// those tests are unrelated to which broker/backend used.
// thus we only test it here.
//

func (me *localTestSuite) TestIgnoreReport() {
	remain, err := me.app.Register(
		"TestIgnoreReport", func() {}, 1, 1,
		transport.Encode.Default, transport.Encode.Default,
	)
	me.Equal(0, remain)
	me.Nil(err)

	// initiate a task with an option(IgnoreReport == true)
	reports, err := me.app.Call(
		"TestIgnoreReport",
		transport.NewOption().SetIgnoreReport(true),
	)
	me.Nil(err)
	me.Nil(reports)
}

//
// a demo for cutomized marshaller/invoker without reflect,
// error-checking are skipped for simplicity.
//

type testMyMarshaller struct{}

func (me *testMyMarshaller) Prepare(string, interface{}) (err error) { return }
func (me *testMyMarshaller) EncodeTask(fn interface{}, task *transport.Task) ([]byte, error) {
	// encode args
	bN, _ := json.Marshal(task.Args()[0])
	bName, _ := json.Marshal(task.Args()[1])
	// encode option
	bOpt, _ := json.Marshal(task.P.O)
	return transport.ComposeBytes(task.H, [][]byte{bN, bName, bOpt})
}

func (me *testMyMarshaller) DecodeTask(h *transport.Header, fn interface{}, b []byte) (task *transport.Task, err error) {
	var (
		n    int
		name string
		o    *transport.Option
	)

	bs, _ := transport.DecomposeBytes(h, b)
	json.Unmarshal(bs[0], &n)
	json.Unmarshal(bs[1], &name)
	json.Unmarshal(bs[2], &o)

	task = &transport.Task{
		H: h,
		P: &transport.TaskPayload{
			O: o,
			A: []interface{}{n, name},
		},
	}
	return
}

func (me *testMyMarshaller) EncodeReport(fn interface{}, report *transport.Report) (b []byte, err error) {
	bs := [][]byte{}

	// encode returns
	if report.OK() {
		bMsg, _ := json.Marshal(report.Return()[0])
		bCount, _ := json.Marshal(report.Return()[1])
		bs = append(bs, bMsg, bCount)
	}

	// encode status
	bStatus, _ := json.Marshal(report.Status())
	// encode error
	bError, _ := json.Marshal(report.Error())
	// encode option
	bOpt, _ := json.Marshal(report.Option())
	bs = append(bs, bStatus, bError, bOpt)

	return transport.ComposeBytes(report.H, bs)
}

func (me *testMyMarshaller) DecodeReport(h *transport.Header, fn interface{}, b []byte) (report *transport.Report, err error) {
	var (
		msg   string
		count int
		s     int16
		e     *transport.Error
		o     *transport.Option
	)
	bs, _ := transport.DecomposeBytes(h, b)
	if len(bs) > 3 {
		// the report might, or might not containing
		// returns.
		json.Unmarshal(bs[0], &msg)
		json.Unmarshal(bs[1], &count)
		bs = bs[2:]
	}
	json.Unmarshal(bs[0], &s)
	json.Unmarshal(bs[1], &e)
	json.Unmarshal(bs[2], &o)
	report = &transport.Report{
		H: h,
		P: &transport.ReportPayload{
			S: s,
			E: e,
			O: o,
			R: []interface{}{msg, count},
		},
	}
	return
}

type testMyInvoker struct{}

func (me *testMyInvoker) Call(f interface{}, param []interface{}) ([]interface{}, error) {
	msg, count := f.(func(int, string) (string, int))(
		param[0].(int),
		param[1].(string),
	)
	return []interface{}{msg, count}, nil
}

func (me *testMyInvoker) Return(f interface{}, returns []interface{}) ([]interface{}, error) {
	return returns, nil
}

func (me *localTestSuite) TestMyMarshaller() {
	fn := func(n int, name string) (msg string, count int) {
		msg = name + "_'s message"
		count = n + 1
		return
	}

	// register marshaller
	mid := int16(101)
	err := me.app.AddMarshaller(mid, &struct {
		testMyInvoker
		testMyMarshaller
	}{})
	me.Nil(err)

	// allocate workers
	remain, err := me.app.Register("TestMyMarshaller", fn, 1, 1, mid, mid)
	me.Equal(0, remain)
	me.Nil(err)

	// initiate a task with an option(IgnoreReport == true)
	reports, err := me.app.Call(
		"TestMyMarshaller", transport.NewOption().SetOnlyResult(true), 12345, "mission",
	)
	me.Nil(err)
	r := <-reports
	me.Equal("mission_'s message", r.Return()[0].(string))
	me.Equal(int(12346), r.Return()[1].(int))
}

func (me *localTestSuite) TestCustomMarshaller() {
	fn := func(n int, name string) (msg string, count int) {
		msg = name + "_'s message"
		count = n + 1
		return
	}

	// register marshaller
	mid := int16(102)
	err := me.app.AddMarshaller(mid, &struct {
		testMyInvoker
		transport.CustomMarshaller
	}{
		testMyInvoker{},
		transport.CustomMarshaller{
			Encode: func(output bool, val []interface{}) (bs [][]byte, err error) {
				bs = [][]byte{}
				if output {
					bMsg, _ := json.Marshal(val[0])
					bCount, _ := json.Marshal(val[1])
					bs = append(bs, bMsg, bCount)
				} else {
					bN, _ := json.Marshal(val[0])
					bName, _ := json.Marshal(val[1])
					bs = append(bs, bN, bName)
				}
				return
			},
			Decode: func(output bool, bs [][]byte) (val []interface{}, err error) {
				val = []interface{}{}
				if output {
					var (
						msg string
						n   int
					)
					json.Unmarshal(bs[0], &msg)
					json.Unmarshal(bs[1], &n)
					val = append(val, msg, n)
				} else {
					var (
						n    int
						name string
					)
					json.Unmarshal(bs[0], &n)
					json.Unmarshal(bs[1], &name)
					val = append(val, n, name)
				}

				return
			},
		},
	})
	me.Nil(err)

	// allocate workers
	remain, err := me.app.Register("TestCustomMarshaller", fn, 1, 1, mid, mid)
	me.Equal(0, remain)
	me.Nil(err)

	// initiate a task with an option(IgnoreReport == true)
	reports, err := me.app.Call(
		"TestCustomMarshaller", transport.NewOption(), 12345, "mission",
	)
	me.Nil(err)

finished:
	for {
		select {
		case r := <-reports:
			if r.OK() {
				me.Equal("mission_'s message", r.Return()[0].(string))
				me.Equal(int(12346), r.Return()[1].(int))
			}
			if r.Fail() {
				me.Fail("this task is failed, unexpected")
			}
			if r.Done() {
				break finished
			}
		}
	}
}

type testMyInvoker2 struct{}

func (me *testMyInvoker2) Call(f interface{}, param []interface{}) ([]interface{}, error) {
	f.(func())()
	return nil, nil
}

func (me *testMyInvoker2) Return(f interface{}, returns []interface{}) ([]interface{}, error) {
	return returns, nil
}

func (me *localTestSuite) TestCustomMarshallerWithMinimalFunc() {
	called := false
	fn := func() {
		called = true
	}

	// register marshaller
	mid := int16(103)
	err := me.app.AddMarshaller(mid, &struct {
		testMyInvoker2
		transport.CustomMarshaller
	}{
		testMyInvoker2{},
		transport.CustomMarshaller{
			Encode: func(output bool, val []interface{}) (bs [][]byte, err error) {
				return
			},
			Decode: func(output bool, bs [][]byte) (val []interface{}, err error) {
				return
			},
		},
	})
	me.Nil(err)

	// allocate workers
	remain, err := me.app.Register("TestCustomMarshallerWithMinimalFunc", fn, 1, 1, mid, mid)
	me.Equal(0, remain)
	me.Nil(err)

	// initiate a task with an option(IgnoreReport == true)
	reports, err := me.app.Call(
		"TestCustomMarshallerWithMinimalFunc", transport.NewOption(),
	)
	me.Nil(err)

finished:
	for {
		select {
		case r := <-reports:
			if r.OK() {
				me.Len(r.Return(), 0)
			}
			if r.Fail() {
				me.Fail("this task is failed, unexpected")
			}
			if r.Done() {
				break finished
			}
		}
	}
	me.True(called)
}
