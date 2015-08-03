package bql

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	_ "pfi/sensorbee/sensorbee/bql/udf/builtin"
	"pfi/sensorbee/sensorbee/core"
	"pfi/sensorbee/sensorbee/data"
	"testing"
)

func setupTopology(stmt string, trace bool) (*TopologyBuilder, error) {
	// create a stream from a dummy source
	dt := newTestTopology()
	dt.Context().Flags.TupleTrace.Set(trace)

	tb, err := NewTopologyBuilder(dt)
	if err != nil {
		return nil, err
	}

	err = addBQLToTopology(tb, "CREATE PAUSED SOURCE source TYPE dummy WITH num=4")
	if err != nil {
		return nil, err
	}
	// issue BQL statement (inserts box)
	if err := addBQLToTopology(tb, stmt); err != nil {
		return nil, err
	}
	// sink
	err = addBQLToTopology(tb, `
		CREATE SINK snk TYPE collector;
		INSERT INTO snk FROM box;
		RESUME SOURCE source;`)
	if err != nil {
		return nil, err
	}
	return tb, err
}

func TestBasicBQLBoxConnectivity(t *testing.T) {
	tuples := mkTuples(4)
	tup2 := *tuples[1]
	tup4 := *tuples[3]

	Convey("Given an ISTREAM/2 SECONDS BQL statement", t, func() {
		s := "CREATE STREAM box AS SELECT " +
			"ISTREAM int, str((int+1) % 3) AS x FROM source [RANGE 1 TUPLES] WHERE int % 2 = 0"
		tb, err := setupTopology(s, true)
		So(err, ShouldBeNil)
		dt := tb.Topology()
		Reset(func() {
			dt.Stop()
		})

		sin, err := dt.Sink("snk")
		So(err, ShouldBeNil)
		si := sin.Sink().(*tupleCollectorSink)

		Convey("When 4 tuples are emitted by the source", func() {
			tup2.Data["x"] = data.String(fmt.Sprintf("%d", ((2 + 1) % 3)))
			tup4.Data["x"] = data.String(fmt.Sprintf("%d", ((4 + 1) % 3)))

			Convey("Then the sink receives 2 tuples", func() {
				si.Wait(2)
				So(si.Tuples, ShouldNotBeNil)
				So(len(si.Tuples), ShouldEqual, 2)

				Convey("And the first tuple has tup2's data and timestamp", func() {
					si.Tuples[0].InputName = "input"
					si.Tuples[0].Trace = nil // don't check trace here
					So(*si.Tuples[0], ShouldResemble, tup2)
				})

				Convey("And the first tuple has trace", func() {
					ts := si.Tuples[0].Trace
					So(len(ts), ShouldEqual, 4)
					So(ts[0].Type, ShouldEqual, core.ETOutput)
					So(ts[0].Msg, ShouldEqual, "source")
					So(ts[1].Type, ShouldEqual, core.ETInput)
					So(ts[1].Msg, ShouldEqual, "box")
					So(ts[2].Type, ShouldEqual, core.ETOutput)
					So(ts[2].Msg, ShouldEqual, "box")
					So(ts[3].Type, ShouldEqual, core.ETInput)
					So(ts[3].Msg, ShouldEqual, "snk")
				})

				Convey("And the second tuple has tup4's data and timestamp", func() {
					si.Tuples[1].InputName = "input"
					si.Tuples[1].Trace = nil // don't check trace here
					So(*si.Tuples[1], ShouldResemble, tup4)
				})

				Convey("And the second tuple has trace", func() {
					ts := si.Tuples[1].Trace
					So(len(ts), ShouldEqual, 4)
					So(ts[0].Type, ShouldEqual, core.ETOutput)
					So(ts[0].Msg, ShouldEqual, "source")
					So(ts[1].Type, ShouldEqual, core.ETInput)
					So(ts[1].Msg, ShouldEqual, "box")
					So(ts[2].Type, ShouldEqual, core.ETOutput)
					So(ts[2].Msg, ShouldEqual, "box")
					So(ts[3].Type, ShouldEqual, core.ETInput)
					So(ts[3].Msg, ShouldEqual, "snk")
				})
			})
		})

		Convey("When rewinding the source", func() {
			si.Wait(2)
			So(addBQLToTopology(tb, `REWIND SOURCE source;`), ShouldBeNil)

			Convey("Then the sinkreceives tuples again", func() {
				si.Wait(4)
				So(len(si.Tuples), ShouldEqual, 4)
			})
		})
	})
}

func TestBasicBQLBoxUnionCapability(t *testing.T) {
	Convey("Given a UNION over two identical streams in BQL", t, func() {
		s := "CREATE STREAM box AS " +
			"SELECT ISTREAM int FROM source [RANGE 1 TUPLES] WHERE int % 2 = 0 " +
			"UNION ALL SELECT ISTREAM int FROM source [RANGE 1 TUPLES] WHERE int % 2 = 0"
		tb, err := setupTopology(s, false)
		So(err, ShouldBeNil)
		dt := tb.Topology()
		Reset(func() {
			dt.Stop()
		})

		sin, err := dt.Sink("snk")
		So(err, ShouldBeNil)
		si := sin.Sink().(*tupleCollectorSink)

		Convey("When 4 tuples are emitted by the source", func() {
			Convey("Then the sink receives 4 tuples", func() {
				si.Wait(4)
				So(si.Tuples, ShouldNotBeNil)
				So(len(si.Tuples), ShouldEqual, 4)

				Convey("And they are the union of two filtered streams", func() {
					found := map[int64]bool{}
					for _, t := range si.Tuples {
						v := t.Data["int"]
						i, _ := data.AsInt(v)
						found[i] = true
					}
					So(found, ShouldResemble, map[int64]bool{
						2: true, 4: true,
					})
				})
			})
		})

		Convey("When rewinding the source", func() {
			si.Wait(2)
			So(addBQLToTopology(tb, `REWIND SOURCE source;`), ShouldBeNil)

			Convey("Then the sinkreceives tuples again", func() {
				si.Wait(8)
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})
	})

	Convey("Given a UNION over two disjoint streams in BQL", t, func() {
		s := "CREATE STREAM box AS " +
			"SELECT ISTREAM int FROM source [RANGE 1 TUPLES] WHERE int % 2 = 0 " +
			"UNION ALL SELECT ISTREAM int FROM source [RANGE 1 TUPLES] WHERE int % 2 = 1"
		tb, err := setupTopology(s, false)
		So(err, ShouldBeNil)
		dt := tb.Topology()
		Reset(func() {
			dt.Stop()
		})

		sin, err := dt.Sink("snk")
		So(err, ShouldBeNil)
		si := sin.Sink().(*tupleCollectorSink)

		Convey("When 4 tuples are emitted by the source", func() {
			Convey("Then the sink receives 4 tuples", func() {
				si.Wait(4)
				So(si.Tuples, ShouldNotBeNil)
				So(len(si.Tuples), ShouldEqual, 4)

				Convey("And they are the union of two filtered streams", func() {
					found := map[int64]bool{}
					for _, t := range si.Tuples {
						v := t.Data["int"]
						i, _ := data.AsInt(v)
						found[i] = true
					}
					So(found, ShouldResemble, map[int64]bool{
						1: true, 2: true, 3: true, 4: true,
					})
				})
			})
		})

		Convey("When rewinding the source", func() {
			si.Wait(2)
			So(addBQLToTopology(tb, `REWIND SOURCE source;`), ShouldBeNil)

			Convey("Then the sinkreceives tuples again", func() {
				si.Wait(8)
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})
	})

	Convey("Given a UNION over three disjoint streams in BQL", t, func() {
		s := "CREATE STREAM box AS " +
			"SELECT ISTREAM int, 'a' AS x FROM source [RANGE 1 TUPLES] WHERE int = 0 " +
			"UNION ALL SELECT ISTREAM int, 'b' AS y FROM source [RANGE 1 TUPLES] WHERE int = 1" +
			"UNION ALL SELECT ISTREAM int, 'c' AS z FROM source [RANGE 1 TUPLES] WHERE int >= 2"
		tb, err := setupTopology(s, false)
		So(err, ShouldBeNil)
		dt := tb.Topology()
		Reset(func() {
			dt.Stop()
		})

		sin, err := dt.Sink("snk")
		So(err, ShouldBeNil)
		si := sin.Sink().(*tupleCollectorSink)

		Convey("When 4 tuples are emitted by the source", func() {
			Convey("Then the sink receives 4 tuples", func() {
				si.Wait(4)
				So(si.Tuples, ShouldNotBeNil)
				So(len(si.Tuples), ShouldEqual, 4)

				Convey("And they are the union of three filtered streams", func() {
					for _, t := range si.Tuples {
						v := t.Data["int"]
						i, _ := data.AsInt(v)
						if i == 0 {
							So(len(t.Data), ShouldEqual, 2)
							So(t.Data["x"], ShouldResemble, data.String("a"))
						} else if i == 1 {
							So(len(t.Data), ShouldEqual, 2)
							So(t.Data["y"], ShouldResemble, data.String("b"))
						} else {
							So(len(t.Data), ShouldEqual, 2)
							So(t.Data["z"], ShouldResemble, data.String("c"))
						}
					}
				})
			})
		})

		Convey("When rewinding the source", func() {
			si.Wait(2)
			So(addBQLToTopology(tb, `REWIND SOURCE source;`), ShouldBeNil)

			Convey("Then the sinkreceives tuples again", func() {
				si.Wait(8)
				So(len(si.Tuples), ShouldEqual, 8)
			})
		})
	})
}

func TestBQLBoxJoinCapability(t *testing.T) {
	tuples := mkTuples(4)

	Convey("Given an RSTREAM statement with a lot of joins", t, func() {
		s := `CREATE STREAM box AS SELECT RSTREAM
		source:int AS a, s2:int AS b, duplicate:int AS c, d2:int AS d
		FROM source [RANGE 1 TUPLES],
		     source [RANGE 1 TUPLES] AS s2,
		     duplicate('source', 3) [RANGE 1 TUPLES],
		     duplicate('source', 2) [RANGE 1 TUPLES] AS d2
		`
		tb, err := setupTopology(s, false)
		So(err, ShouldBeNil)
		dt := tb.Topology()
		Reset(func() {
			dt.Stop()
		})

		sin, err := dt.Sink("snk")
		So(err, ShouldBeNil)
		si := sin.Sink().(*tupleCollectorSink)

		Convey("When 4 tuples are emitted by the source", func() {
			Convey("Then the sink receives a number of tuples", func() {
				si.Wait(2)
				So(si.Tuples, ShouldNotBeNil)
				So(len(si.Tuples), ShouldBeGreaterThanOrEqualTo, 2)

				// the number and order or result tuples varies,
				// so there is not a lot of stuff we can check...
				Convey("And all tuples should have keys a,b,c,d", func() {
					t := si.Tuples[0]
					// the first tuple should definitely have the same timestamp
					// as the first tuple in the input set
					So(t.Timestamp, ShouldResemble, tuples[0].Timestamp)

					for _, tup := range si.Tuples {
						_, hasA := tup.Data["a"]
						So(hasA, ShouldBeTrue)
						_, hasB := tup.Data["d"]
						So(hasB, ShouldBeTrue)
						_, hasC := tup.Data["c"]
						So(hasC, ShouldBeTrue)
						_, hasD := tup.Data["d"]
						So(hasD, ShouldBeTrue)
					}
				})
			})
		})
	})
}

func TestBQLBoxGroupByCapability(t *testing.T) {
	Convey("Given an ISTREAM/2 SECONDS BQL statement", t, func() {
		s := "CREATE STREAM box AS SELECT " +
			"ISTREAM count(1) FROM source [RANGE 2 SECONDS] WHERE int % 2 = 0"
		tb, err := setupTopology(s, false)
		So(err, ShouldBeNil)
		dt := tb.Topology()
		Reset(func() {
			dt.Stop()
		})

		sin, err := dt.Sink("snk")
		So(err, ShouldBeNil)
		si := sin.Sink().(*tupleCollectorSink)

		Convey("When 4 tuples are emitted by the source", func() {

			Convey("Then the sink receives 3 tuples", func() {
				si.Wait(3)
				So(si.Tuples, ShouldNotBeNil)
				So(len(si.Tuples), ShouldEqual, 3)

				Convey("And the tuples have the correct counts", func() {
					So(si.Tuples[0].Data["count"], ShouldResemble, data.Int(0))
					So(si.Tuples[1].Data["count"], ShouldResemble, data.Int(1))
					// the third tuple is not counted because of WHERE, so
					// ISTREAM doesn't emit anything
					So(si.Tuples[2].Data["count"], ShouldResemble, data.Int(2))
				})
			})
		})
	})
}

func TestBQLBoxUDSF(t *testing.T) {
	Convey("Given a topology using UDSF", t, func() {
		tb, err := setupTopology(`CREATE STREAM box AS SELECT ISTREAM duplicate:int FROM duplicate('source', 3) [RANGE 1 TUPLES]`, false)
		So(err, ShouldBeNil)
		dt := tb.Topology()
		Reset(func() {
			dt.Stop()
		})

		sin, err := dt.Sink("snk")
		So(err, ShouldBeNil)
		si := sin.Sink().(*tupleCollectorSink)

		Convey("When 4 tuples are emitted by the source", func() {
			Convey("Then the sink should receive 12 tuples", func() {
				si.Wait(12)
				So(len(si.Tuples), ShouldEqual, 12)
			})
		})
	})
}
