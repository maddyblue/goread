/*
 * Copyright (c) 2013 Matt Jibson <matt.jibson@gmail.com>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package goon

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/aetest"
	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/memcache"
)

// *[]S, *[]*S, *[]I, []S, []*S, []I
const (
	ivTypePtrToSliceOfStructs = iota
	ivTypePtrToSliceOfPtrsToStruct
	ivTypePtrToSliceOfInterfaces
	ivTypeSliceOfStructs
	ivTypeSliceOfPtrsToStruct
	ivTypeSliceOfInterfaces
	ivTypeTotal
)

const (
	ivModeDatastore = iota
	ivModeMemcache
	ivModeMemcacheAndDatastore
	ivModeLocalcache
	ivModeLocalcacheAndMemcache
	ivModeLocalcacheAndDatastore
	ivModeLocalcacheAndMemcacheAndDatastore
	ivModeTotal
)

// Have a bunch of different supported types to detect any wild errors
// https://developers.google.com/appengine/docs/go/datastore/reference
type ivItem struct {
	Id          int64        `datastore:"-" goon:"id"`
	Int         int          `datastore:"int,noindex"`
	Int8        int8         `datastore:"int8,noindex"`
	Int16       int16        `datastore:"int16,noindex"`
	Int32       int32        `datastore:"int32,noindex"`
	Int64       int64        `datastore:"int64,noindex"`
	Float32     float32      `datastore:"float32,noindex"`
	Float64     float64      `datastore:"float64,noindex"`
	Bool        bool         `datastore:"bool,noindex"`
	String      string       `datastore:"string,noindex"`
	CustomTypes ivItemCustom `datastore:"custom,noindex"`
	SliceTypes  ivItemSlice  `datastore:"slice,noindex"`
	ByteSlice   []byte       `datastore:"byte_slice,noindex"`
	BSSlice     [][]byte     `datastore:"bs_slice,noindex"`
	Time        time.Time    `datastore:"time,noindex"`
	TimeSlice   []time.Time  `datastore:"time_slice,noindex"`
	NoIndex     int          `datastore:",noindex"`
	Casual      string
	Ζεύς        string
	Key         *datastore.Key
	ChildKey    *datastore.Key
	ZeroKey     *datastore.Key
	KeySlice    []*datastore.Key
	KeySliceNil []*datastore.Key
	BlobKey     appengine.BlobKey
	BKSlice     []appengine.BlobKey
	Sub         ivItemSub
	Subs        []ivItemSubs
	ZZZV        []ivZZZV
}

type ivItemInt int
type ivItemInt8 int8
type ivItemInt16 int16
type ivItemInt32 int32
type ivItemInt64 int64
type ivItemFloat32 float32
type ivItemFloat64 float64
type ivItemBool bool
type ivItemString string

type ivItemDeepInt ivItemInt

type ivItemCustom struct {
	Int     ivItemInt
	Int8    ivItemInt8
	Int16   ivItemInt16
	Int32   ivItemInt32
	Int64   ivItemInt64
	Float32 ivItemFloat32
	Float64 ivItemFloat64
	Bool    ivItemBool
	String  ivItemString
	DeepInt ivItemDeepInt
}

type ivItemSlice struct {
	Int      []int
	Int8     []int8
	Int16    []int16
	Int32    []int32
	Int64    []int64
	Float32  []float32
	Float64  []float64
	Bool     []bool
	String   []string
	IntC     []ivItemInt
	Int8C    []ivItemInt8
	Int16C   []ivItemInt16
	Int32C   []ivItemInt32
	Int64C   []ivItemInt64
	Float32C []ivItemFloat32
	Float64C []ivItemFloat64
	BoolC    []ivItemBool
	StringC  []ivItemString
	DeepInt  []ivItemDeepInt
}

type ivItemSub struct {
	Data string `datastore:"data,noindex"`
	Ints []int  `datastore:"ints,noindex"`
}

type ivItemSubs struct {
	Data  string `datastore:"data,noindex"`
	Extra string `datastore:",noindex"`
}

type ivZZZV struct {
	Key  *datastore.Key `datastore:"key,noindex"`
	Data string         `datastore:"data,noindex"`
}

func (ivi *ivItem) ForInterface() {}

type ivItemI interface {
	ForInterface()
}

var ivItems []ivItem

func initializeIvItems(c appengine.Context) {
	t1 := time.Now().Truncate(time.Microsecond)
	t2 := t1.Add(time.Second * 1)
	t3 := t1.Add(time.Second * 2)

	ivItems = []ivItem{
		{Id: 1, Int: 123, Int8: 77, Int16: 13001, Int32: 1234567890, Int64: 123456789012345,
			Float32: (float32(10) / float32(3)), Float64: (float64(10000000) / float64(9998)),
			Bool: true, String: "one",
			CustomTypes: ivItemCustom{Int: 123, Int8: 77, Int16: 13001, Int32: 1234567890, Int64: 123456789012345,
				Float32: ivItemFloat32(float32(10) / float32(3)), Float64: ivItemFloat64(float64(10000000) / float64(9998)),
				Bool: true, String: "one", DeepInt: 1},
			SliceTypes: ivItemSlice{Int: []int{1, 2}, Int8: []int8{1, 2}, Int16: []int16{1, 2}, Int32: []int32{1, 2}, Int64: []int64{1, 2},
				Float32: []float32{1.0, 2.0}, Float64: []float64{1.0, 2.0}, Bool: []bool{true, false}, String: []string{"one", "two"},
				IntC: []ivItemInt{1, 2}, Int8C: []ivItemInt8{1, 2}, Int16C: []ivItemInt16{1, 2}, Int32C: []ivItemInt32{1, 2}, Int64C: []ivItemInt64{1, 2},
				Float32C: []ivItemFloat32{1.0, 2.0}, Float64C: []ivItemFloat64{1.0, 2.0},
				BoolC: []ivItemBool{true, false}, StringC: []ivItemString{"one", "two"}, DeepInt: []ivItemDeepInt{1, 2}},
			ByteSlice: []byte{0xDE, 0xAD}, BSSlice: [][]byte{{0x01, 0x02}, {0x03, 0x04}},
			Time: t1, TimeSlice: []time.Time{t1, t2, t3}, NoIndex: 1,
			Casual: "clothes", Ζεύς: "Zeus",
			Key:         datastore.NewKey(c, "Fruit", "Apple", 0, nil),
			ChildKey:    datastore.NewKey(c, "Person", "Jane", 0, datastore.NewKey(c, "Person", "John", 0, datastore.NewKey(c, "Person", "Jack", 0, nil))),
			KeySlice:    []*datastore.Key{datastore.NewKey(c, "Key", "", 1, nil), datastore.NewKey(c, "Key", "", 2, nil), datastore.NewKey(c, "Key", "", 3, nil)},
			KeySliceNil: []*datastore.Key{datastore.NewKey(c, "Number", "", 1, nil), nil, datastore.NewKey(c, "Number", "", 2, nil)},
			BlobKey:     "fake #1", BKSlice: []appengine.BlobKey{"fake #1.1", "fake #1.2"},
			Sub: ivItemSub{Data: "yay #1", Ints: []int{1, 2, 3}},
			Subs: []ivItemSubs{
				{Data: "sub #1.1", Extra: "xtra #1.1"},
				{Data: "sub #1.2", Extra: "xtra #1.2"},
				{Data: "sub #1.3", Extra: "xtra #1.3"}},
			ZZZV: []ivZZZV{{Data: "None"}, {Key: datastore.NewKey(c, "Fruit", "Banana", 0, nil)}}},
		{Id: 2, Int: 124, Int8: 78, Int16: 13002, Int32: 1234567891, Int64: 123456789012346,
			Float32: (float32(10) / float32(3)), Float64: (float64(10000000) / float64(9998)),
			Bool: true, String: "two",
			CustomTypes: ivItemCustom{Int: 124, Int8: 78, Int16: 13002, Int32: 1234567891, Int64: 123456789012346,
				Float32: ivItemFloat32(float32(10) / float32(3)), Float64: ivItemFloat64(float64(10000000) / float64(9998)),
				Bool: true, String: "two", DeepInt: 2},
			SliceTypes: ivItemSlice{Int: []int{1, 2}, Int8: []int8{1, 2}, Int16: []int16{1, 2}, Int32: []int32{1, 2}, Int64: []int64{1, 2},
				Float32: []float32{1.0, 2.0}, Float64: []float64{1.0, 2.0}, Bool: []bool{true, false}, String: []string{"one", "two"},
				IntC: []ivItemInt{1, 2}, Int8C: []ivItemInt8{1, 2}, Int16C: []ivItemInt16{1, 2}, Int32C: []ivItemInt32{1, 2}, Int64C: []ivItemInt64{1, 2},
				Float32C: []ivItemFloat32{1.0, 2.0}, Float64C: []ivItemFloat64{1.0, 2.0},
				BoolC: []ivItemBool{true, false}, StringC: []ivItemString{"one", "two"}, DeepInt: []ivItemDeepInt{1, 2}},
			ByteSlice: []byte{0xBE, 0xEF}, BSSlice: [][]byte{{0x05, 0x06}, {0x07, 0x08}},
			Time: t2, TimeSlice: []time.Time{t2, t3, t1}, NoIndex: 2,
			Casual: "manners", Ζεύς: "Alcmene",
			Key:         datastore.NewKey(c, "Fruit", "Banana", 0, nil),
			ChildKey:    datastore.NewKey(c, "Person", "Jane", 0, datastore.NewKey(c, "Person", "John", 0, datastore.NewKey(c, "Person", "Jack", 0, nil))),
			KeySlice:    []*datastore.Key{datastore.NewKey(c, "Key", "", 4, nil), datastore.NewKey(c, "Key", "", 5, nil), datastore.NewKey(c, "Key", "", 6, nil)},
			KeySliceNil: []*datastore.Key{datastore.NewKey(c, "Number", "", 3, nil), nil, datastore.NewKey(c, "Number", "", 4, nil)},
			BlobKey:     "fake #2", BKSlice: []appengine.BlobKey{"fake #2.1", "fake #2.2"},
			Sub: ivItemSub{Data: "yay #2", Ints: []int{4, 5, 6}},
			Subs: []ivItemSubs{
				{Data: "sub #2.1", Extra: "xtra #2.1"},
				{Data: "sub #2.2", Extra: "xtra #2.2"},
				{Data: "sub #2.3", Extra: "xtra #2.3"}},
			ZZZV: []ivZZZV{{Data: "None"}, {Key: datastore.NewKey(c, "Fruit", "Banana", 0, nil)}}},
		{Id: 3, Int: 125, Int8: 79, Int16: 13003, Int32: 1234567892, Int64: 123456789012347,
			Float32: (float32(10) / float32(3)), Float64: (float64(10000000) / float64(9998)),
			Bool: true, String: "tri",
			CustomTypes: ivItemCustom{Int: 125, Int8: 79, Int16: 13003, Int32: 1234567892, Int64: 123456789012347,
				Float32: ivItemFloat32(float32(10) / float32(3)), Float64: ivItemFloat64(float64(10000000) / float64(9998)),
				Bool: true, String: "tri", DeepInt: 3},
			SliceTypes: ivItemSlice{Int: []int{1, 2}, Int8: []int8{1, 2}, Int16: []int16{1, 2}, Int32: []int32{1, 2}, Int64: []int64{1, 2},
				Float32: []float32{1.0, 2.0}, Float64: []float64{1.0, 2.0}, Bool: []bool{true, false}, String: []string{"one", "two"},
				IntC: []ivItemInt{1, 2}, Int8C: []ivItemInt8{1, 2}, Int16C: []ivItemInt16{1, 2}, Int32C: []ivItemInt32{1, 2}, Int64C: []ivItemInt64{1, 2},
				Float32C: []ivItemFloat32{1.0, 2.0}, Float64C: []ivItemFloat64{1.0, 2.0},
				BoolC: []ivItemBool{true, false}, StringC: []ivItemString{"one", "two"}, DeepInt: []ivItemDeepInt{1, 2}},
			ByteSlice: []byte{0xF0, 0x0D}, BSSlice: [][]byte{{0x09, 0x0A}, {0x0B, 0x0C}},
			Time: t3, TimeSlice: []time.Time{t3, t1, t2}, NoIndex: 3,
			Casual: "weather", Ζεύς: "Hercules",
			Key:         datastore.NewKey(c, "Fruit", "Cherry", 0, nil),
			ChildKey:    datastore.NewKey(c, "Person", "Jane", 0, datastore.NewKey(c, "Person", "John", 0, datastore.NewKey(c, "Person", "Jack", 0, nil))),
			KeySlice:    []*datastore.Key{datastore.NewKey(c, "Key", "", 7, nil), datastore.NewKey(c, "Key", "", 8, nil), datastore.NewKey(c, "Key", "", 9, nil)},
			KeySliceNil: []*datastore.Key{datastore.NewKey(c, "Number", "", 5, nil), nil, datastore.NewKey(c, "Number", "", 6, nil)},
			BlobKey:     "fake #3", BKSlice: []appengine.BlobKey{"fake #3.1", "fake #3.2"},
			Sub: ivItemSub{Data: "yay #3", Ints: []int{7, 8, 9}},
			Subs: []ivItemSubs{
				{Data: "sub #3.1", Extra: "xtra #3.1"},
				{Data: "sub #3.2", Extra: "xtra #3.2"},
				{Data: "sub #3.3", Extra: "xtra #3.3"}},
			ZZZV: []ivZZZV{{Data: "None"}, {Key: datastore.NewKey(c, "Fruit", "Banana", 0, nil)}}}}
}

func getIVItemCopy(g *Goon, index int) *ivItem {
	// All basic value types are copied easily
	ivi := ivItems[index]

	// .. but pointer based types require extra work
	ivi.SliceTypes.Int = []int{}
	for _, v := range ivItems[index].SliceTypes.Int {
		ivi.SliceTypes.Int = append(ivi.SliceTypes.Int, v)
	}

	ivi.SliceTypes.Int8 = []int8{}
	for _, v := range ivItems[index].SliceTypes.Int8 {
		ivi.SliceTypes.Int8 = append(ivi.SliceTypes.Int8, v)
	}

	ivi.SliceTypes.Int16 = []int16{}
	for _, v := range ivItems[index].SliceTypes.Int16 {
		ivi.SliceTypes.Int16 = append(ivi.SliceTypes.Int16, v)
	}

	ivi.SliceTypes.Int32 = []int32{}
	for _, v := range ivItems[index].SliceTypes.Int32 {
		ivi.SliceTypes.Int32 = append(ivi.SliceTypes.Int32, v)
	}

	ivi.SliceTypes.Int64 = []int64{}
	for _, v := range ivItems[index].SliceTypes.Int64 {
		ivi.SliceTypes.Int64 = append(ivi.SliceTypes.Int64, v)
	}

	ivi.SliceTypes.Float32 = []float32{}
	for _, v := range ivItems[index].SliceTypes.Float32 {
		ivi.SliceTypes.Float32 = append(ivi.SliceTypes.Float32, v)
	}

	ivi.SliceTypes.Float64 = []float64{}
	for _, v := range ivItems[index].SliceTypes.Float64 {
		ivi.SliceTypes.Float64 = append(ivi.SliceTypes.Float64, v)
	}

	ivi.SliceTypes.Bool = []bool{}
	for _, v := range ivItems[index].SliceTypes.Bool {
		ivi.SliceTypes.Bool = append(ivi.SliceTypes.Bool, v)
	}

	ivi.SliceTypes.String = []string{}
	for _, v := range ivItems[index].SliceTypes.String {
		ivi.SliceTypes.String = append(ivi.SliceTypes.String, v)
	}

	ivi.SliceTypes.IntC = []ivItemInt{}
	for _, v := range ivItems[index].SliceTypes.IntC {
		ivi.SliceTypes.IntC = append(ivi.SliceTypes.IntC, v)
	}

	ivi.SliceTypes.Int8C = []ivItemInt8{}
	for _, v := range ivItems[index].SliceTypes.Int8C {
		ivi.SliceTypes.Int8C = append(ivi.SliceTypes.Int8C, v)
	}

	ivi.SliceTypes.Int16C = []ivItemInt16{}
	for _, v := range ivItems[index].SliceTypes.Int16C {
		ivi.SliceTypes.Int16C = append(ivi.SliceTypes.Int16C, v)
	}

	ivi.SliceTypes.Int32C = []ivItemInt32{}
	for _, v := range ivItems[index].SliceTypes.Int32C {
		ivi.SliceTypes.Int32C = append(ivi.SliceTypes.Int32C, v)
	}

	ivi.SliceTypes.Int64C = []ivItemInt64{}
	for _, v := range ivItems[index].SliceTypes.Int64C {
		ivi.SliceTypes.Int64C = append(ivi.SliceTypes.Int64C, v)
	}

	ivi.SliceTypes.Float32C = []ivItemFloat32{}
	for _, v := range ivItems[index].SliceTypes.Float32C {
		ivi.SliceTypes.Float32C = append(ivi.SliceTypes.Float32C, v)
	}

	ivi.SliceTypes.Float64C = []ivItemFloat64{}
	for _, v := range ivItems[index].SliceTypes.Float64C {
		ivi.SliceTypes.Float64C = append(ivi.SliceTypes.Float64C, v)
	}

	ivi.SliceTypes.BoolC = []ivItemBool{}
	for _, v := range ivItems[index].SliceTypes.BoolC {
		ivi.SliceTypes.BoolC = append(ivi.SliceTypes.BoolC, v)
	}

	ivi.SliceTypes.StringC = []ivItemString{}
	for _, v := range ivItems[index].SliceTypes.StringC {
		ivi.SliceTypes.StringC = append(ivi.SliceTypes.StringC, v)
	}

	ivi.SliceTypes.DeepInt = []ivItemDeepInt{}
	for _, v := range ivItems[index].SliceTypes.DeepInt {
		ivi.SliceTypes.DeepInt = append(ivi.SliceTypes.DeepInt, v)
	}

	ivi.ByteSlice = []byte{}
	for _, v := range ivItems[index].ByteSlice {
		ivi.ByteSlice = append(ivi.ByteSlice, v)
	}

	ivi.BSSlice = [][]byte{}
	for _, v := range ivItems[index].BSSlice {
		vCopy := []byte{}
		for _, v := range v {
			vCopy = append(vCopy, v)
		}
		ivi.BSSlice = append(ivi.BSSlice, vCopy)
	}

	ivi.TimeSlice = []time.Time{}
	for _, v := range ivItems[index].TimeSlice {
		ivi.TimeSlice = append(ivi.TimeSlice, v)
	}

	ivi.Key = datastore.NewKey(g.Context, ivItems[index].Key.Kind(), ivItems[index].Key.StringID(), ivItems[index].Key.IntID(), nil)

	ivi.ChildKey = datastore.NewKey(g.Context, ivItems[index].ChildKey.Kind(), ivItems[index].ChildKey.StringID(), ivItems[index].ChildKey.IntID(),
		datastore.NewKey(g.Context, ivItems[index].ChildKey.Parent().Kind(), ivItems[index].ChildKey.Parent().StringID(), ivItems[index].ChildKey.Parent().IntID(),
			datastore.NewKey(g.Context, ivItems[index].ChildKey.Parent().Parent().Kind(), ivItems[index].ChildKey.Parent().Parent().StringID(), ivItems[index].ChildKey.Parent().Parent().IntID(), nil)))

	ivi.KeySlice = []*datastore.Key{}
	for _, key := range ivItems[index].KeySlice {
		ivi.KeySlice = append(ivi.KeySlice, datastore.NewKey(g.Context, key.Kind(), key.StringID(), key.IntID(), nil))
	}

	ivi.KeySliceNil = []*datastore.Key{}
	for _, key := range ivItems[index].KeySliceNil {
		if key == nil {
			ivi.KeySliceNil = append(ivi.KeySliceNil, nil)
		} else {
			ivi.KeySliceNil = append(ivi.KeySliceNil, datastore.NewKey(g.Context, key.Kind(), key.StringID(), key.IntID(), nil))
		}
	}

	ivi.BKSlice = []appengine.BlobKey{}
	for _, v := range ivItems[index].BKSlice {
		ivi.BKSlice = append(ivi.BKSlice, v)
	}

	ivi.Sub = ivItemSub{}
	ivi.Sub.Data = ivItems[index].Sub.Data
	for _, v := range ivItems[index].Sub.Ints {
		ivi.Sub.Ints = append(ivi.Sub.Ints, v)
	}

	ivi.Subs = []ivItemSubs{}
	for _, sub := range ivItems[index].Subs {
		ivi.Subs = append(ivi.Subs, ivItemSubs{Data: sub.Data, Extra: sub.Extra})
	}

	ivi.ZZZV = []ivZZZV{}
	for _, zzzv := range ivItems[index].ZZZV {
		ivi.ZZZV = append(ivi.ZZZV, ivZZZV{Key: zzzv.Key, Data: zzzv.Data})
	}

	return &ivi
}

func getInputVarietySrc(t *testing.T, g *Goon, ivType int, indices ...int) interface{} {
	if ivType >= ivTypeTotal {
		t.Fatalf("Invalid input variety type! %v >= %v", ivType, ivTypeTotal)
		return nil
	}

	var result interface{}

	switch ivType {
	case ivTypePtrToSliceOfStructs:
		s := []ivItem{}
		for _, index := range indices {
			s = append(s, *getIVItemCopy(g, index))
		}
		result = &s
	case ivTypePtrToSliceOfPtrsToStruct:
		s := []*ivItem{}
		for _, index := range indices {
			s = append(s, getIVItemCopy(g, index))
		}
		result = &s
	case ivTypePtrToSliceOfInterfaces:
		s := []ivItemI{}
		for _, index := range indices {
			s = append(s, getIVItemCopy(g, index))
		}
		result = &s
	case ivTypeSliceOfStructs:
		s := []ivItem{}
		for _, index := range indices {
			s = append(s, *getIVItemCopy(g, index))
		}
		result = s
	case ivTypeSliceOfPtrsToStruct:
		s := []*ivItem{}
		for _, index := range indices {
			s = append(s, getIVItemCopy(g, index))
		}
		result = s
	case ivTypeSliceOfInterfaces:
		s := []ivItemI{}
		for _, index := range indices {
			s = append(s, getIVItemCopy(g, index))
		}
		result = s
	}

	return result
}

func getInputVarietyDst(t *testing.T, ivType int) interface{} {
	if ivType >= ivTypeTotal {
		t.Fatalf("Invalid input variety type! %v >= %v", ivType, ivTypeTotal)
		return nil
	}

	var result interface{}

	switch ivType {
	case ivTypePtrToSliceOfStructs:
		result = &[]ivItem{{Id: ivItems[0].Id}, {Id: ivItems[1].Id}, {Id: ivItems[2].Id}}
	case ivTypePtrToSliceOfPtrsToStruct:
		result = &[]*ivItem{{Id: ivItems[0].Id}, {Id: ivItems[1].Id}, {Id: ivItems[2].Id}}
	case ivTypePtrToSliceOfInterfaces:
		result = &[]ivItemI{&ivItem{Id: ivItems[0].Id}, &ivItem{Id: ivItems[1].Id}, &ivItem{Id: ivItems[2].Id}}
	case ivTypeSliceOfStructs:
		result = []ivItem{{Id: ivItems[0].Id}, {Id: ivItems[1].Id}, {Id: ivItems[2].Id}}
	case ivTypeSliceOfPtrsToStruct:
		result = []*ivItem{{Id: ivItems[0].Id}, {Id: ivItems[1].Id}, {Id: ivItems[2].Id}}
	case ivTypeSliceOfInterfaces:
		result = []ivItemI{&ivItem{Id: ivItems[0].Id}, &ivItem{Id: ivItems[1].Id}, &ivItem{Id: ivItems[2].Id}}
	}

	return result
}

func getPrettyIVMode(ivMode int) string {
	result := "N/A"

	switch ivMode {
	case ivModeDatastore:
		result = "DS"
	case ivModeMemcache:
		result = "MC"
	case ivModeMemcacheAndDatastore:
		result = "DS+MC"
	case ivModeLocalcache:
		result = "LC"
	case ivModeLocalcacheAndMemcache:
		result = "MC+LC"
	case ivModeLocalcacheAndDatastore:
		result = "DS+LC"
	case ivModeLocalcacheAndMemcacheAndDatastore:
		result = "DS+MC+LC"
	}

	return result
}

func getPrettyIVType(ivType int) string {
	result := "N/A"

	switch ivType {
	case ivTypePtrToSliceOfStructs:
		result = "*[]S"
	case ivTypePtrToSliceOfPtrsToStruct:
		result = "*[]*S"
	case ivTypePtrToSliceOfInterfaces:
		result = "*[]I"
	case ivTypeSliceOfStructs:
		result = "[]S"
	case ivTypeSliceOfPtrsToStruct:
		result = "[]*S"
	case ivTypeSliceOfInterfaces:
		result = "[]I"
	}

	return result
}

func ivWipe(t *testing.T, g *Goon, prettyInfo string) {
	// Make sure the datastore is clear of any previous tests
	// TODO: Batch this once goon gets more convenient batch delete support
	for _, ivi := range ivItems {
		if err := g.Delete(g.Key(ivi)); err != nil {
			t.Errorf("%s > Unexpected error on delete - %v", prettyInfo, err)
		}
	}

	// Make sure the caches are clear, so any caching is done by our specific test
	g.FlushLocalCache()
	memcache.Flush(g.Context)
}

func ivGetMulti(t *testing.T, g *Goon, ref, dst interface{}, prettyInfo string) error {
	// Get our data back and make sure it's correct
	if err := g.GetMulti(dst); err != nil {
		t.Errorf("%s > Unexpected error on GetMulti - %v", prettyInfo, err)
		return err
	} else {
		dstLen := reflect.Indirect(reflect.ValueOf(dst)).Len()
		refLen := reflect.Indirect(reflect.ValueOf(ref)).Len()

		if dstLen != refLen {
			t.Errorf("%s > Unexpected dst len (%v) doesn't match ref len (%v)", prettyInfo, dstLen, refLen)
		} else if !reflect.DeepEqual(ref, dst) {
			t.Errorf("%s > Expected - %v, %v, %v - got %v, %v, %v", prettyInfo,
				reflect.Indirect(reflect.ValueOf(ref)).Index(0).Interface(),
				reflect.Indirect(reflect.ValueOf(ref)).Index(1).Interface(),
				reflect.Indirect(reflect.ValueOf(ref)).Index(2).Interface(),
				reflect.Indirect(reflect.ValueOf(dst)).Index(0).Interface(),
				reflect.Indirect(reflect.ValueOf(dst)).Index(1).Interface(),
				reflect.Indirect(reflect.ValueOf(dst)).Index(2).Interface())
		}
	}

	return nil
}

func validateInputVariety(t *testing.T, g *Goon, srcType, dstType, mode int) {
	if mode >= ivModeTotal {
		t.Fatalf("Invalid input variety mode! %v >= %v", mode, ivModeTotal)
		return
	}

	// Generate a nice debug info string for clear logging
	prettyInfo := getPrettyIVType(srcType) + " " + getPrettyIVType(dstType) + " " + getPrettyIVMode(mode)

	// This function just gets the entities based on a predefined list, helper for cache population
	loadIVItem := func(indices ...int) {
		for _, index := range indices {
			ivi := &ivItem{Id: ivItems[index].Id}
			if err := g.Get(ivi); err != nil {
				t.Errorf("%s > Unexpected error on get - %v", prettyInfo, err)
			} else if !reflect.DeepEqual(ivItems[index], *ivi) {
				t.Errorf("%s > Expected - %v, got %v", prettyInfo, ivItems[index], *ivi)
			}
		}
	}

	// Start with a clean slate
	ivWipe(t, g, prettyInfo)

	// Generate test data with the specified types
	src := getInputVarietySrc(t, g, srcType, 0, 1, 2)
	ref := getInputVarietySrc(t, g, dstType, 0, 1, 2)
	dst := getInputVarietyDst(t, dstType)

	// Save our test data
	if _, err := g.PutMulti(src); err != nil {
		t.Errorf("%s > Unexpected error on PutMulti - %v", prettyInfo, err)
	}

	// Clear the caches, as we're going to precisely set the caches via Get
	g.FlushLocalCache()
	memcache.Flush(g.Context)

	// Set the caches into proper state based on given mode
	switch mode {
	case ivModeDatastore:
		// Caches already clear
	case ivModeMemcache:
		loadIVItem(0, 1, 2) // Left in memcache
		g.FlushLocalCache()
	case ivModeMemcacheAndDatastore:
		loadIVItem(0, 1) // Left in memcache
		g.FlushLocalCache()
	case ivModeLocalcache:
		loadIVItem(0, 1, 2) // Left in local cache
	case ivModeLocalcacheAndMemcache:
		loadIVItem(0) // Left in memcache
		g.FlushLocalCache()
		loadIVItem(1, 2) // Left in local cache
	case ivModeLocalcacheAndDatastore:
		loadIVItem(0, 1) // Left in local cache
	case ivModeLocalcacheAndMemcacheAndDatastore:
		loadIVItem(0) // Left in memcache
		g.FlushLocalCache()
		loadIVItem(1) // Left in local cache
	}

	// Get our data back and make sure it's correct
	ivGetMulti(t, g, ref, dst, prettyInfo)
}

func validateInputVarietyTXNPut(t *testing.T, g *Goon, srcType, dstType, mode int) {
	if mode >= ivModeTotal {
		t.Fatalf("Invalid input variety mode! %v >= %v", mode, ivModeTotal)
		return
	}

	// The following modes are redundant with the current goon transaction implementation
	switch mode {
	case ivModeMemcache:
		return
	case ivModeMemcacheAndDatastore:
		return
	case ivModeLocalcacheAndMemcache:
		return
	case ivModeLocalcacheAndMemcacheAndDatastore:
		return
	}

	// Generate a nice debug info string for clear logging
	prettyInfo := getPrettyIVType(srcType) + " " + getPrettyIVType(dstType) + " " + getPrettyIVMode(mode) + " TXNPut"

	// Start with a clean slate
	ivWipe(t, g, prettyInfo)

	// Generate test data with the specified types
	src := getInputVarietySrc(t, g, srcType, 0, 1, 2)
	ref := getInputVarietySrc(t, g, dstType, 0, 1, 2)
	dst := getInputVarietyDst(t, dstType)

	// Save our test data
	if err := g.RunInTransaction(func(tg *Goon) error {
		_, err := tg.PutMulti(src)
		return err
	}, &datastore.TransactionOptions{XG: true}); err != nil {
		t.Errorf("%s > Unexpected error on PutMulti - %v", prettyInfo, err)
	}

	// Set the caches into proper state based on given mode
	switch mode {
	case ivModeDatastore:
		g.FlushLocalCache()
		memcache.Flush(g.Context)
	case ivModeLocalcache:
		// Entities already in local cache
	case ivModeLocalcacheAndDatastore:
		g.FlushLocalCache()
		memcache.Flush(g.Context)

		subSrc := getInputVarietySrc(t, g, srcType, 0)

		if err := g.RunInTransaction(func(tg *Goon) error {
			_, err := tg.PutMulti(subSrc)
			return err
		}, &datastore.TransactionOptions{XG: true}); err != nil {
			t.Errorf("%s > Unexpected error on PutMulti - %v", prettyInfo, err)
		}
	}

	// Get our data back and make sure it's correct
	ivGetMulti(t, g, ref, dst, prettyInfo)
}

func validateInputVarietyTXNGet(t *testing.T, g *Goon, srcType, dstType, mode int) {
	if mode >= ivModeTotal {
		t.Fatalf("Invalid input variety mode! %v >= %v", mode, ivModeTotal)
		return
	}

	// The following modes are redundant with the current goon transaction implementation
	switch mode {
	case ivModeMemcache:
		return
	case ivModeMemcacheAndDatastore:
		return
	case ivModeLocalcache:
		return
	case ivModeLocalcacheAndMemcache:
		return
	case ivModeLocalcacheAndDatastore:
		return
	case ivModeLocalcacheAndMemcacheAndDatastore:
		return
	}

	// Generate a nice debug info string for clear logging
	prettyInfo := getPrettyIVType(srcType) + " " + getPrettyIVType(dstType) + " " + getPrettyIVMode(mode) + " TXNGet"

	// Start with a clean slate
	ivWipe(t, g, prettyInfo)

	// Generate test data with the specified types
	src := getInputVarietySrc(t, g, srcType, 0, 1, 2)
	ref := getInputVarietySrc(t, g, dstType, 0, 1, 2)
	dst := getInputVarietyDst(t, dstType)

	// Save our test data
	if _, err := g.PutMulti(src); err != nil {
		t.Errorf("%s > Unexpected error on PutMulti - %v", prettyInfo, err)
	}

	// Set the caches into proper state based on given mode
	// TODO: Instead of clear, fill the caches with invalid data, because we're supposed to always fetch from the datastore
	switch mode {
	case ivModeDatastore:
		g.FlushLocalCache()
		memcache.Flush(g.Context)
	}

	// Get our data back and make sure it's correct
	if err := g.RunInTransaction(func(tg *Goon) error {
		return ivGetMulti(t, tg, ref, dst, prettyInfo)
	}, &datastore.TransactionOptions{XG: true}); err != nil {
		t.Errorf("%s > Unexpected error on transaction - %v", prettyInfo, err)
	}
}

func TestInputVariety(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	g := FromContext(c)

	initializeIvItems(c)

	for srcType := 0; srcType < ivTypeTotal; srcType++ {
		for dstType := 0; dstType < ivTypeTotal; dstType++ {
			for mode := 0; mode < ivModeTotal; mode++ {
				validateInputVariety(t, g, srcType, dstType, mode)
				validateInputVarietyTXNPut(t, g, srcType, dstType, mode)
				validateInputVarietyTXNGet(t, g, srcType, dstType, mode)
			}
		}
	}
}

type MigrationA struct {
	_kind     string            `goon:"kind,Migration"`
	Id        int64             `datastore:"-" goon:"id"`
	Number    int32             `datastore:"number,noindex"`
	Word      string            `datastore:"word,noindex"`
	Car       string            `datastore:"car,noindex"`
	Holiday   time.Time         `datastore:"holiday,noindex"`
	α         int               `datastore:",noindex"`
	Level     MigrationIntA     `datastore:"level,noindex"`
	Floor     MigrationIntA     `datastore:"floor,noindex"`
	Sub       MigrationSub      `datastore:"sub,noindex"`
	Son       MigrationPerson   `datastore:"son,noindex"`
	Daughter  MigrationPerson   `datastore:"daughter,noindex"`
	Parents   []MigrationPerson `datastore:"parents,noindex"`
	DeepSlice MigrationDeepA    `datastore:"deep,noindex"`
	ZZs       []ZigZag          `datastore:"zigzag,noindex"`
	ZeroKey   *datastore.Key    `datastore:",noindex"`
	File      []byte
}

type MigrationSub struct {
	Data  string          `datastore:"data,noindex"`
	Noise []int           `datastore:"noise,noindex"`
	Sub   MigrationSubSub `datastore:"sub,noindex"`
}

type MigrationSubSub struct {
	Data string `datastore:"data,noindex"`
}

type MigrationPerson struct {
	Name string `datastore:"name,noindex"`
	Age  int    `datastore:"age,noindex"`
}

type MigrationDeepA struct {
	Deep MigrationDeepB `datastore:"deep,noindex"`
}

type MigrationDeepB struct {
	Deep MigrationDeepC `datastore:"deep,noindex"`
}

type MigrationDeepC struct {
	Slice []int `datastore:"slice,noindex"`
}

type ZigZag struct {
	Zig int `datastore:"zig,noindex"`
	Zag int `datastore:"zag,noindex"`
}

type ZigZags struct {
	Zig []int `datastore:"zig,noindex"`
	Zag []int `datastore:"zag,noindex"`
}

type MigrationIntA int
type MigrationIntB int

type MigrationB struct {
	_kind          string            `goon:"kind,Migration"`
	Identification int64             `datastore:"-" goon:"id"`
	FancyNumber    int32             `datastore:"number,noindex"`
	Slang          string            `datastore:"word,noindex"`
	Cars           []string          `datastore:"car,noindex"`
	Holidays       []time.Time       `datastore:"holiday,noindex"`
	β              int               `datastore:"α,noindex"`
	Level          MigrationIntB     `datastore:"level,noindex"`
	Floors         []MigrationIntB   `datastore:"floor,noindex"`
	Animal         string            `datastore:"sub.data,noindex"`
	Music          []int             `datastore:"sub.noise,noindex"`
	Flower         string            `datastore:"sub.sub.data,noindex"`
	Sons           []MigrationPerson `datastore:"son,noindex"`
	DaughterName   string            `datastore:"daughter.name,noindex"`
	DaughterAge    int               `datastore:"daughter.age,noindex"`
	OldFolks       []MigrationPerson `datastore:"parents,noindex"`
	FarSlice       MigrationDeepA    `datastore:"deep,noindex"`
	ZZs            ZigZags           `datastore:"zigzag,noindex"`
	Keys           []*datastore.Key  `datastore:"ZeroKey,noindex"`
	Files          [][]byte          `datastore:"File,noindex"`
}

func TestMigration(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	g := FromContext(c)

	// Create & save an entity with the original structure
	migA := &MigrationA{Id: 1, Number: 123, Word: "rabbit", Car: "BMW",
		Holiday: time.Now().Truncate(time.Microsecond), α: 1, Level: 9001, Floor: 5,
		Sub: MigrationSub{Data: "fox", Noise: []int{1, 2, 3}, Sub: MigrationSubSub{Data: "rose"}},
		Son: MigrationPerson{Name: "John", Age: 5}, Daughter: MigrationPerson{Name: "Nancy", Age: 6},
		Parents:   []MigrationPerson{{Name: "Sven", Age: 56}, {Name: "Sonya", Age: 49}},
		DeepSlice: MigrationDeepA{Deep: MigrationDeepB{Deep: MigrationDeepC{Slice: []int{1, 2, 3}}}},
		ZZs:       []ZigZag{{Zig: 1}, {Zag: 1}}, File: []byte{0xF0, 0x0D}}
	if _, err := g.Put(migA); err != nil {
		t.Errorf("Unexpected error on Put: %v", err)
	}

	// Clear the local cache, because we want this data in memcache
	g.FlushLocalCache()

	// Get it back, so it's in the cache
	migA = &MigrationA{Id: 1}
	if err := g.Get(migA); err != nil {
		t.Errorf("Unexpected error on Get: %v", err)
	}

	// Clear the local cache, because it doesn't need to support migration
	g.FlushLocalCache()

	// Test whether memcache supports migration
	verifyMigration(t, g, migA, "MC")

	// Clear all the caches
	g.FlushLocalCache()
	memcache.Flush(c)

	// Test whether datastore supports migration
	verifyMigration(t, g, migA, "DS")
}

func verifyMigration(t *testing.T, g *Goon, migA *MigrationA, debugInfo string) {
	migB := &MigrationB{Identification: migA.Id}
	if err := g.Get(migB); err != nil {
		t.Errorf("%v > Unexpected error on Get: %v", debugInfo, err)
	} else if migA.Id != migB.Identification {
		t.Errorf("%v > Ids don't match: %v != %v", debugInfo, migA.Id, migB.Identification)
	} else if migA.Number != migB.FancyNumber {
		t.Errorf("%v > Numbers don't match: %v != %v", debugInfo, migA.Number, migB.FancyNumber)
	} else if migA.Word != migB.Slang {
		t.Errorf("%v > Words don't match: %v != %v", debugInfo, migA.Word, migB.Slang)
	} else if len(migB.Cars) != 1 {
		t.Errorf("%v > Expected 1 car! Got: %v", debugInfo, len(migB.Cars))
	} else if migA.Car != migB.Cars[0] {
		t.Errorf("%v > Cars don't match: %v != %v", debugInfo, migA.Car, migB.Cars[0])
	} else if len(migB.Holidays) != 1 {
		t.Errorf("%v > Expected 1 holiday! Got: %v", debugInfo, len(migB.Holidays))
	} else if migA.Holiday != migB.Holidays[0] {
		t.Errorf("%v > Holidays don't match: %v != %v", debugInfo, migA.Holiday, migB.Holidays[0])
	} else if migA.α != migB.β {
		t.Errorf("%v > Greek doesn't match: %v != %v", debugInfo, migA.α, migB.β)
	} else if int(migA.Level) != int(migB.Level) {
		t.Errorf("%v > Level doesn't match: %v != %v", debugInfo, migA.Level, migB.Level)
	} else if len(migB.Floors) != 1 {
		t.Errorf("%v > Expected 1 floor! Got: %v", debugInfo, len(migB.Floors))
	} else if int(migA.Floor) != int(migB.Floors[0]) {
		t.Errorf("%v > Floor doesn't match: %v != %v", debugInfo, migA.Floor, migB.Floors[0])
	} else if migA.Sub.Data != migB.Animal {
		t.Errorf("%v > Animal doesn't match: %v != %v", debugInfo, migA.Sub.Data, migB.Animal)
	} else if !reflect.DeepEqual(migA.Sub.Noise, migB.Music) {
		t.Errorf("%v > Music doesn't match: %v != %v", debugInfo, migA.Sub.Noise, migB.Music)
	} else if migA.Sub.Sub.Data != migB.Flower {
		t.Errorf("%v > Flower doesn't match: %v != %v", debugInfo, migA.Sub.Sub.Data, migB.Flower)
	} else if len(migB.Sons) != 1 {
		t.Errorf("%v > Expected 1 son! Got: %v", debugInfo, len(migB.Sons))
	} else if migA.Son.Name != migB.Sons[0].Name {
		t.Errorf("%v > Son names don't match: %v != %v", debugInfo, migA.Son.Name, migB.Sons[0].Name)
	} else if migA.Son.Age != migB.Sons[0].Age {
		t.Errorf("%v > Son ages don't match: %v != %v", debugInfo, migA.Son.Age, migB.Sons[0].Age)
	} else if migA.Daughter.Name != migB.DaughterName {
		t.Errorf("%v > Daughter names don't match: %v != %v", debugInfo, migA.Daughter.Name, migB.DaughterName)
	} else if migA.Daughter.Age != migB.DaughterAge {
		t.Errorf("%v > Daughter ages don't match: %v != %v", debugInfo, migA.Daughter.Age, migB.DaughterAge)
	} else if !reflect.DeepEqual(migA.Parents, migB.OldFolks) {
		t.Errorf("%v > Parents don't match: %v != %v", debugInfo, migA.Parents, migB.OldFolks)
	} else if !reflect.DeepEqual(migA.DeepSlice, migB.FarSlice) {
		t.Errorf("%v > Deep slice doesn't match: %v != %v", debugInfo, migA.DeepSlice, migB.FarSlice)
	} else if len(migB.ZZs.Zig) != 2 {
		t.Errorf("%v > Expected 2 Zigs, got: %v", debugInfo, len(migB.ZZs.Zig))
	} else if len(migB.ZZs.Zag) != 2 {
		t.Errorf("%v > Expected 2 Zags, got: %v", debugInfo, len(migB.ZZs.Zag))
	} else if migA.ZZs[0].Zig != migB.ZZs.Zig[0] {
		t.Errorf("%v > Invalid zig #1: %v != %v", debugInfo, migA.ZZs[0].Zig, migB.ZZs.Zig[0])
	} else if migA.ZZs[1].Zig != migB.ZZs.Zig[1] {
		t.Errorf("%v > Invalid zig #2: %v != %v", debugInfo, migA.ZZs[1].Zig, migB.ZZs.Zig[1])
	} else if migA.ZZs[0].Zag != migB.ZZs.Zag[0] {
		t.Errorf("%v > Invalid zag #1: %v != %v", debugInfo, migA.ZZs[0].Zag, migB.ZZs.Zag[0])
	} else if migA.ZZs[1].Zag != migB.ZZs.Zag[1] {
		t.Errorf("%v > Invalid zag #2: %v != %v", debugInfo, migA.ZZs[1].Zag, migB.ZZs.Zag[1])
	} else if len(migB.Keys) != 1 {
		t.Errorf("%v > Expected 1 keys, got %v", debugInfo, len(migB.Keys))
	} else if len(migB.Files) != 1 {
		t.Errorf("%v > Expected 1 file, got %v", debugInfo, len(migB.Files))
	} else if !reflect.DeepEqual(migA.File, migB.Files[0]) {
		t.Errorf("%v > Files don't match: %v != %v", debugInfo, migA.File, migB.Files[0])
	}
}

func TestTXNRace(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	g := FromContext(c)

	// Create & store some test data
	hid := &HasId{Id: 1, Name: "foo"}
	if _, err := g.Put(hid); err != nil {
		t.Errorf("Unexpected error on Put %v", err)
	}

	// Get this data back, to populate caches
	if err := g.Get(hid); err != nil {
		t.Errorf("Unexpected error on Get %v", err)
	}

	// Clear the local cache, as we are testing for proper memcache usage
	g.FlushLocalCache()

	// Update the test data inside a transction
	if err := g.RunInTransaction(func(tg *Goon) error {
		// Get the current data
		thid := &HasId{Id: 1}
		if err := tg.Get(thid); err != nil {
			t.Errorf("Unexpected error on TXN Get %v", err)
			return err
		}

		// Update the data
		thid.Name = "bar"
		if _, err := tg.Put(thid); err != nil {
			t.Errorf("Unexpected error on TXN Put %v", err)
			return err
		}

		// Concurrent request emulation
		//   We are running this inside the transaction block to always get the correct timing for testing.
		//   In the real world, this concurrent request may run in another instance.
		//   The transaction block may contain multiple other operations after the preceding Put(),
		//   allowing for ample time for the concurrent request to run before the transaction is committed.
		if err := g.Get(hid); err != nil {
			t.Errorf("Unexpected error on Get %v", err)
		} else if hid.Name != "foo" {
			t.Errorf("Expected 'foo', got %v", hid.Name)
		}

		// Commit the transaction
		return nil
	}, &datastore.TransactionOptions{XG: false}); err != nil {
		t.Errorf("Unexpected error with TXN - %v", err)
	}

	// Clear the local cache, as we are testing for proper memcache usage
	g.FlushLocalCache()

	// Get the data back again, to confirm it was changed in the transaction
	if err := g.Get(hid); err != nil {
		t.Errorf("Unexpected error on Get %v", err)
	} else if hid.Name != "bar" {
		t.Errorf("Expected 'bar', got %v", hid.Name)
	}

	// Clear the local cache, as we are testing for proper memcache usage
	g.FlushLocalCache()

	// Delete the test data inside a transction
	if err := g.RunInTransaction(func(tg *Goon) error {
		thid := &HasId{Id: 1}
		if err := tg.Delete(tg.Key(thid)); err != nil {
			t.Errorf("Unexpected error on TXN Delete %v", err)
			return err
		}

		// Concurrent request emulation
		if err := g.Get(hid); err != nil {
			t.Errorf("Unexpected error on Get %v", err)
		} else if hid.Name != "bar" {
			t.Errorf("Expected 'bar', got %v", hid.Name)
		}

		// Commit the transaction
		return nil
	}, &datastore.TransactionOptions{XG: false}); err != nil {
		t.Errorf("Unexpected error with TXN - %v", err)
	}

	// Clear the local cache, as we are testing for proper memcache usage
	g.FlushLocalCache()

	// Attempt to get the data back again, to confirm it was deleted in the transaction
	if err := g.Get(hid); err != datastore.ErrNoSuchEntity {
		t.Errorf("Expected ErrNoSuchEntity, got %v", err)
	}
}

func TestNegativeCacheHit(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	g := FromContext(c)

	hid := &HasId{Id: 1}

	if err := g.Get(hid); err != datastore.ErrNoSuchEntity {
		t.Errorf("Expected ErrNoSuchEntity, got %v", err)
	}

	// Do a sneaky save straight to the datastore
	if _, err := datastore.Put(c, datastore.NewKey(c, "HasId", "", 1, nil), &HasId{Id: 1, Name: "one"}); err != nil {
		t.Errorf("Unexpected error on datastore.Put: %v", err)
	}

	// Get the entity again via goon, to make sure we cached the non-existance
	if err := g.Get(hid); err != datastore.ErrNoSuchEntity {
		t.Errorf("Expected ErrNoSuchEntity, got %v", err)
	}
}

func TestCaches(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	g := FromContext(c)

	// Put *struct{}
	phid := &HasId{Name: "cacheFail"}
	_, err = g.Put(phid)
	if err != nil {
		t.Errorf("Unexpected error on put - %v", err)
	}

	// fetch *struct{} from cache
	ghid := &HasId{Id: phid.Id}
	err = g.Get(ghid)
	if err != nil {
		t.Errorf("Unexpected error on get - %v", err)
	}
	if !reflect.DeepEqual(phid, ghid) {
		t.Errorf("Expected - %v, got %v", phid, ghid)
	}

	// fetch []struct{} from cache
	ghids := []HasId{{Id: phid.Id}}
	err = g.GetMulti(&ghids)
	if err != nil {
		t.Errorf("Unexpected error on get - %v", err)
	}
	if !reflect.DeepEqual(*phid, ghids[0]) {
		t.Errorf("Expected - %v, got %v", *phid, ghids[0])
	}

	// Now flush localcache and fetch them again
	g.FlushLocalCache()
	// fetch *struct{} from memcache
	ghid = &HasId{Id: phid.Id}
	err = g.Get(ghid)
	if err != nil {
		t.Errorf("Unexpected error on get - %v", err)
	}
	if !reflect.DeepEqual(phid, ghid) {
		t.Errorf("Expected - %v, got %v", phid, ghid)
	}

	g.FlushLocalCache()
	// fetch []struct{} from memcache
	ghids = []HasId{{Id: phid.Id}}
	err = g.GetMulti(&ghids)
	if err != nil {
		t.Errorf("Unexpected error on get - %v", err)
	}
	if !reflect.DeepEqual(*phid, ghids[0]) {
		t.Errorf("Expected - %v, got %v", *phid, ghids[0])
	}
}

func TestGoon(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	n := FromContext(c)

	// Don't want any of these tests to hit the timeout threshold on the devapp server
	MemcacheGetTimeout = time.Second
	MemcachePutTimeoutLarge = time.Second
	MemcachePutTimeoutSmall = time.Second

	// key tests
	noid := NoId{}
	if k, err := n.KeyError(noid); err == nil && !k.Incomplete() {
		t.Error("expected incomplete on noid")
	}
	if n.Key(noid) == nil {
		t.Error("expected to find a key")
	}

	var keyTests = []keyTest{
		{
			HasDefaultKind{},
			datastore.NewKey(c, "DefaultKind", "", 0, nil),
		},
		{
			HasId{Id: 1},
			datastore.NewKey(c, "HasId", "", 1, nil),
		},
		{
			HasKind{Id: 1, Kind: "OtherKind"},
			datastore.NewKey(c, "OtherKind", "", 1, nil),
		},

		{
			HasDefaultKind{Id: 1, Kind: "OtherKind"},
			datastore.NewKey(c, "OtherKind", "", 1, nil),
		},
		{
			HasDefaultKind{Id: 1},
			datastore.NewKey(c, "DefaultKind", "", 1, nil),
		},
		{
			HasString{Id: "new"},
			datastore.NewKey(c, "HasString", "new", 0, nil),
		},
	}

	for _, kt := range keyTests {
		if k, err := n.KeyError(kt.obj); err != nil {
			t.Errorf("error: %v", err)
		} else if !k.Equal(kt.key) {
			t.Errorf("keys not equal")
		}
	}

	if _, err := n.KeyError(TwoId{IntId: 1, StringId: "1"}); err == nil {
		t.Errorf("expected key error")
	}

	// datastore tests
	keys, _ := datastore.NewQuery("HasId").KeysOnly().GetAll(c, nil)
	datastore.DeleteMulti(c, keys)
	memcache.Flush(c)
	if err := n.Get(&HasId{Id: 0}); err == nil {
		t.Errorf("ds: expected error, we're fetching from the datastore on an incomplete key!")
	}
	if err := n.Get(&HasId{Id: 1}); err != datastore.ErrNoSuchEntity {
		t.Errorf("ds: expected no such entity")
	}
	// run twice to make sure autocaching works correctly
	if err := n.Get(&HasId{Id: 1}); err != datastore.ErrNoSuchEntity {
		t.Errorf("ds: expected no such entity")
	}
	es := []*HasId{
		{Id: 1, Name: "one"},
		{Id: 2, Name: "two"},
	}
	var esk []*datastore.Key
	for _, e := range es {
		esk = append(esk, n.Key(e))
	}
	nes := []*HasId{
		{Id: 1},
		{Id: 2},
	}
	if err := n.GetMulti(es); err == nil {
		t.Errorf("ds: expected error")
	} else if !NotFound(err, 0) {
		t.Errorf("ds: not found error 0")
	} else if !NotFound(err, 1) {
		t.Errorf("ds: not found error 1")
	} else if NotFound(err, 2) {
		t.Errorf("ds: not found error 2")
	}

	if keys, err := n.PutMulti(es); err != nil {
		t.Errorf("put: unexpected error")
	} else if len(keys) != len(esk) {
		t.Errorf("put: got unexpected number of keys")
	} else {
		for i, k := range keys {
			if !k.Equal(esk[i]) {
				t.Errorf("put: got unexpected keys")
			}
		}
	}
	if err := n.GetMulti(nes); err != nil {
		t.Errorf("put: unexpected error")
	} else if *es[0] != *nes[0] || *es[1] != *nes[1] {
		t.Errorf("put: bad results")
	} else {
		nesk0 := n.Key(nes[0])
		if !nesk0.Equal(datastore.NewKey(c, "HasId", "", 1, nil)) {
			t.Errorf("put: bad key")
		}
		nesk1 := n.Key(nes[1])
		if !nesk1.Equal(datastore.NewKey(c, "HasId", "", 2, nil)) {
			t.Errorf("put: bad key")
		}
	}
	if _, err := n.Put(HasId{Id: 3}); err == nil {
		t.Errorf("put: expected error")
	}
	// force partial fetch from memcache and then datastore
	memcache.Flush(c)
	if err := n.Get(nes[0]); err != nil {
		t.Errorf("get: unexpected error")
	}
	if err := n.GetMulti(nes); err != nil {
		t.Errorf("get: unexpected error")
	}

	// put a HasId resource, then test pulling it from memory, memcache, and datastore
	hi := &HasId{Name: "hasid"} // no id given, should be automatically created by the datastore
	if _, err := n.Put(hi); err != nil {
		t.Errorf("put: unexpected error - %v", err)
	}
	if n.Key(hi) == nil {
		t.Errorf("key should not be nil")
	} else if n.Key(hi).Incomplete() {
		t.Errorf("key should not be incomplete")
	}

	hi2 := &HasId{Id: hi.Id}
	if err := n.Get(hi2); err != nil {
		t.Errorf("get: unexpected error - %v", err)
	}
	if hi2.Name != hi.Name {
		t.Errorf("Could not fetch HasId object from memory - %#v != %#v, memory=%#v", hi, hi2, n.cache[memkey(n.Key(hi2))])
	}

	hi3 := &HasId{Id: hi.Id}
	delete(n.cache, memkey(n.Key(hi)))
	if err := n.Get(hi3); err != nil {
		t.Errorf("get: unexpected error - %v", err)
	}
	if hi3.Name != hi.Name {
		t.Errorf("Could not fetch HasId object from memory - %#v != %#v", hi, hi3)
	}

	hi4 := &HasId{Id: hi.Id}
	delete(n.cache, memkey(n.Key(hi4)))
	if memcache.Flush(n.Context) != nil {
		t.Errorf("Unable to flush memcache")
	}
	if err := n.Get(hi4); err != nil {
		t.Errorf("get: unexpected error - %v", err)
	}
	if hi4.Name != hi.Name {
		t.Errorf("Could not fetch HasId object from datastore- %#v != %#v", hi, hi4)
	}

	// Now do the opposite also using hi
	// Test pulling from local cache and memcache when datastore result is different
	// Note that this shouldn't happen with real goon usage,
	//   but this tests that goon isn't still pulling from the datastore (or memcache) unnecessarily
	// hi in datastore Name = hasid
	hiPull := &HasId{Id: hi.Id}
	n.cacheLock.Lock()
	n.cache[memkey(n.Key(hi))].(*HasId).Name = "changedincache"
	n.cacheLock.Unlock()
	if err := n.Get(hiPull); err != nil {
		t.Errorf("get: unexpected error - %v", err)
	}
	if hiPull.Name != "changedincache" {
		t.Errorf("hiPull.Name should be 'changedincache' but got %s", hiPull.Name)
	}

	hiPush := &HasId{Id: hi.Id, Name: "changedinmemcache"}
	n.putMemcache([]interface{}{hiPush}, []byte{1})
	n.cacheLock.Lock()
	delete(n.cache, memkey(n.Key(hi)))
	n.cacheLock.Unlock()

	hiPull = &HasId{Id: hi.Id}
	if err := n.Get(hiPull); err != nil {
		t.Errorf("get: unexpected error - %v", err)
	}
	if hiPull.Name != "changedinmemcache" {
		t.Errorf("hiPull.Name should be 'changedinmemcache' but got %s", hiPull.Name)
	}

	// Since the datastore can't assign a key to a String ID, test to make sure goon stops it from happening
	hasString := new(HasString)
	_, err = n.Put(hasString)
	if err == nil {
		t.Errorf("Cannot put an incomplete string Id object as the datastore will populate an int64 id instead- %v", hasString)
	}
	hasString.Id = "hello"
	_, err = n.Put(hasString)
	if err != nil {
		t.Errorf("Error putting hasString object - %v", hasString)
	}

	// Test queries!

	// Test that zero result queries work properly
	qiZRes := []QueryItem{}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem"), &qiZRes); err != nil {
		t.Errorf("GetAll Zero: unexpected error: %v", err)
	} else if len(dskeys) != 0 {
		t.Errorf("GetAll Zero: expected 0 keys, got %v", len(dskeys))
	}

	// Create some entities that we will query for
	if getKeys, err := n.PutMulti([]*QueryItem{{Id: 1, Data: "one"}, {Id: 2, Data: "two"}}); err != nil {
		t.Errorf("PutMulti: unexpected error: %v", err)
	} else {
		// do a datastore Get by *Key so that data is written to the datstore and indexes generated before subsequent query
		if err := datastore.GetMulti(c, getKeys, make([]QueryItem, 2)); err != nil {
			t.Error(err)
		}
	}

	// Clear the local memory cache, because we want to test it being filled correctly by GetAll
	n.FlushLocalCache()

	// Get the entity using a slice of structs
	qiSRes := []QueryItem{}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "one"), &qiSRes); err != nil {
		t.Errorf("GetAll SoS: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll SoS: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 1 {
		t.Errorf("GetAll SoS: expected key IntID to be 1, got %v", dskeys[0].IntID())
	} else if len(qiSRes) != 1 {
		t.Errorf("GetAll SoS: expected 1 result, got %v", len(qiSRes))
	} else if qiSRes[0].Id != 1 {
		t.Errorf("GetAll SoS: expected entity id to be 1, got %v", qiSRes[0].Id)
	} else if qiSRes[0].Data != "one" {
		t.Errorf("GetAll SoS: expected entity data to be 'one', got '%v'", qiSRes[0].Data)
	}

	// Get the entity using normal Get to test local cache (provided the local cache actually got saved)
	qiS := &QueryItem{Id: 1}
	if err := n.Get(qiS); err != nil {
		t.Errorf("Get SoS: unexpected error: %v", err)
	} else if qiS.Id != 1 {
		t.Errorf("Get SoS: expected entity id to be 1, got %v", qiS.Id)
	} else if qiS.Data != "one" {
		t.Errorf("Get SoS: expected entity data to be 'one', got '%v'", qiS.Data)
	}

	// Clear the local memory cache, because we want to test it being filled correctly by GetAll
	n.FlushLocalCache()

	// Get the entity using a slice of pointers to struct
	qiPRes := []*QueryItem{}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "one"), &qiPRes); err != nil {
		t.Errorf("GetAll SoPtS: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll SoPtS: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 1 {
		t.Errorf("GetAll SoPtS: expected key IntID to be 1, got %v", dskeys[0].IntID())
	} else if len(qiPRes) != 1 {
		t.Errorf("GetAll SoPtS: expected 1 result, got %v", len(qiPRes))
	} else if qiPRes[0].Id != 1 {
		t.Errorf("GetAll SoPtS: expected entity id to be 1, got %v", qiPRes[0].Id)
	} else if qiPRes[0].Data != "one" {
		t.Errorf("GetAll SoPtS: expected entity data to be 'one', got '%v'", qiPRes[0].Data)
	}

	// Get the entity using normal Get to test local cache (provided the local cache actually got saved)
	qiP := &QueryItem{Id: 1}
	if err := n.Get(qiP); err != nil {
		t.Errorf("Get SoPtS: unexpected error: %v", err)
	} else if qiP.Id != 1 {
		t.Errorf("Get SoPtS: expected entity id to be 1, got %v", qiP.Id)
	} else if qiP.Data != "one" {
		t.Errorf("Get SoPtS: expected entity data to be 'one', got '%v'", qiP.Data)
	}

	// Clear the local memory cache, because we want to test it being filled correctly by Next
	n.FlushLocalCache()

	// Get the entity using an iterator
	qiIt := n.Run(datastore.NewQuery("QueryItem").Filter("data=", "one"))

	qiItRes := &QueryItem{}
	if dskey, err := qiIt.Next(qiItRes); err != nil {
		t.Errorf("Next: unexpected error: %v", err)
	} else if dskey.IntID() != 1 {
		t.Errorf("Next: expected key IntID to be 1, got %v", dskey.IntID())
	} else if qiItRes.Id != 1 {
		t.Errorf("Next: expected entity id to be 1, got %v", qiItRes.Id)
	} else if qiItRes.Data != "one" {
		t.Errorf("Next: expected entity data to be 'one', got '%v'", qiItRes.Data)
	}

	// Make sure the iterator ends correctly
	if _, err := qiIt.Next(&QueryItem{}); err != datastore.Done {
		t.Errorf("Next: expected iterator to end with the error datastore.Done, got %v", err)
	}

	// Get the entity using normal Get to test local cache (provided the local cache actually got saved)
	qiI := &QueryItem{Id: 1}
	if err := n.Get(qiI); err != nil {
		t.Errorf("Get Iterator: unexpected error: %v", err)
	} else if qiI.Id != 1 {
		t.Errorf("Get Iterator: expected entity id to be 1, got %v", qiI.Id)
	} else if qiI.Data != "one" {
		t.Errorf("Get Iterator: expected entity data to be 'one', got '%v'", qiI.Data)
	}

	// Clear the local memory cache, because we want to test it not being filled incorrectly when supplying a non-zero slice
	n.FlushLocalCache()

	// Get the entity using a non-zero slice of structs
	qiNZSRes := []QueryItem{{Id: 1, Data: "invalid cache"}}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "two"), &qiNZSRes); err != nil {
		t.Errorf("GetAll NZSoS: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll NZSoS: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 2 {
		t.Errorf("GetAll NZSoS: expected key IntID to be 2, got %v", dskeys[0].IntID())
	} else if len(qiNZSRes) != 2 {
		t.Errorf("GetAll NZSoS: expected slice len to be 2, got %v", len(qiNZSRes))
	} else if qiNZSRes[0].Id != 1 {
		t.Errorf("GetAll NZSoS: expected entity id to be 1, got %v", qiNZSRes[0].Id)
	} else if qiNZSRes[0].Data != "invalid cache" {
		t.Errorf("GetAll NZSoS: expected entity data to be 'invalid cache', got '%v'", qiNZSRes[0].Data)
	} else if qiNZSRes[1].Id != 2 {
		t.Errorf("GetAll NZSoS: expected entity id to be 2, got %v", qiNZSRes[1].Id)
	} else if qiNZSRes[1].Data != "two" {
		t.Errorf("GetAll NZSoS: expected entity data to be 'two', got '%v'", qiNZSRes[1].Data)
	}

	// Get the entities using normal GetMulti to test local cache
	qiNZSs := []QueryItem{{Id: 1}, {Id: 2}}
	if err := n.GetMulti(qiNZSs); err != nil {
		t.Errorf("GetMulti NZSoS: unexpected error: %v", err)
	} else if len(qiNZSs) != 2 {
		t.Errorf("GetMulti NZSoS: expected slice len to be 2, got %v", len(qiNZSs))
	} else if qiNZSs[0].Id != 1 {
		t.Errorf("GetMulti NZSoS: expected entity id to be 1, got %v", qiNZSs[0].Id)
	} else if qiNZSs[0].Data != "one" {
		t.Errorf("GetMulti NZSoS: expected entity data to be 'one', got '%v'", qiNZSs[0].Data)
	} else if qiNZSs[1].Id != 2 {
		t.Errorf("GetMulti NZSoS: expected entity id to be 2, got %v", qiNZSs[1].Id)
	} else if qiNZSs[1].Data != "two" {
		t.Errorf("GetMulti NZSoS: expected entity data to be 'two', got '%v'", qiNZSs[1].Data)
	}

	// Clear the local memory cache, because we want to test it not being filled incorrectly when supplying a non-zero slice
	n.FlushLocalCache()

	// Get the entity using a non-zero slice of pointers to struct
	qiNZPRes := []*QueryItem{{Id: 1, Data: "invalid cache"}}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "two"), &qiNZPRes); err != nil {
		t.Errorf("GetAll NZSoPtS: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll NZSoPtS: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 2 {
		t.Errorf("GetAll NZSoPtS: expected key IntID to be 2, got %v", dskeys[0].IntID())
	} else if len(qiNZPRes) != 2 {
		t.Errorf("GetAll NZSoPtS: expected slice len to be 2, got %v", len(qiNZPRes))
	} else if qiNZPRes[0].Id != 1 {
		t.Errorf("GetAll NZSoPtS: expected entity id to be 1, got %v", qiNZPRes[0].Id)
	} else if qiNZPRes[0].Data != "invalid cache" {
		t.Errorf("GetAll NZSoPtS: expected entity data to be 'invalid cache', got '%v'", qiNZPRes[0].Data)
	} else if qiNZPRes[1].Id != 2 {
		t.Errorf("GetAll NZSoPtS: expected entity id to be 2, got %v", qiNZPRes[1].Id)
	} else if qiNZPRes[1].Data != "two" {
		t.Errorf("GetAll NZSoPtS: expected entity data to be 'two', got '%v'", qiNZPRes[1].Data)
	}

	// Get the entities using normal GetMulti to test local cache
	qiNZPs := []*QueryItem{{Id: 1}, {Id: 2}}
	if err := n.GetMulti(qiNZPs); err != nil {
		t.Errorf("GetMulti NZSoPtS: unexpected error: %v", err)
	} else if len(qiNZPs) != 2 {
		t.Errorf("GetMulti NZSoPtS: expected slice len to be 2, got %v", len(qiNZPs))
	} else if qiNZPs[0].Id != 1 {
		t.Errorf("GetMulti NZSoPtS: expected entity id to be 1, got %v", qiNZPs[0].Id)
	} else if qiNZPs[0].Data != "one" {
		t.Errorf("GetMulti NZSoPtS: expected entity data to be 'one', got '%v'", qiNZPs[0].Data)
	} else if qiNZPs[1].Id != 2 {
		t.Errorf("GetMulti NZSoPtS: expected entity id to be 2, got %v", qiNZPs[1].Id)
	} else if qiNZPs[1].Data != "two" {
		t.Errorf("GetMulti NZSoPtS: expected entity data to be 'two', got '%v'", qiNZPs[1].Data)
	}

	// Clear the local memory cache, because we want to test it not being filled incorrectly by a keys-only query
	n.FlushLocalCache()

	// Test the simplest keys-only query
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "one").KeysOnly(), nil); err != nil {
		t.Errorf("GetAll KeysOnly: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll KeysOnly: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 1 {
		t.Errorf("GetAll KeysOnly: expected key IntID to be 1, got %v", dskeys[0].IntID())
	}

	// Get the entity using normal Get to test that the local cache wasn't filled with incomplete data
	qiKO := &QueryItem{Id: 1}
	if err := n.Get(qiKO); err != nil {
		t.Errorf("Get KeysOnly: unexpected error: %v", err)
	} else if qiKO.Id != 1 {
		t.Errorf("Get KeysOnly: expected entity id to be 1, got %v", qiKO.Id)
	} else if qiKO.Data != "one" {
		t.Errorf("Get KeysOnly: expected entity data to be 'one', got '%v'", qiKO.Data)
	}

	// Clear the local memory cache, because we want to test it not being filled incorrectly by a keys-only query
	n.FlushLocalCache()

	// Test the keys-only query with slice of structs
	qiKOSRes := []QueryItem{}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "one").KeysOnly(), &qiKOSRes); err != nil {
		t.Errorf("GetAll KeysOnly SoS: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll KeysOnly SoS: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 1 {
		t.Errorf("GetAll KeysOnly SoS: expected key IntID to be 1, got %v", dskeys[0].IntID())
	} else if len(qiKOSRes) != 1 {
		t.Errorf("GetAll KeysOnly SoS: expected 1 result, got %v", len(qiKOSRes))
	} else if k := reflect.TypeOf(qiKOSRes[0]).Kind(); k != reflect.Struct {
		t.Errorf("GetAll KeysOnly SoS: expected struct, got %v", k)
	} else if qiKOSRes[0].Id != 1 {
		t.Errorf("GetAll KeysOnly SoS: expected entity id to be 1, got %v", qiKOSRes[0].Id)
	} else if qiKOSRes[0].Data != "" {
		t.Errorf("GetAll KeysOnly SoS: expected entity data to be empty, got '%v'", qiKOSRes[0].Data)
	}

	// Get the entity using normal Get to test that the local cache wasn't filled with incomplete data
	if err := n.GetMulti(qiKOSRes); err != nil {
		t.Errorf("Get KeysOnly SoS: unexpected error: %v", err)
	} else if qiKOSRes[0].Id != 1 {
		t.Errorf("Get KeysOnly SoS: expected entity id to be 1, got %v", qiKOSRes[0].Id)
	} else if qiKOSRes[0].Data != "one" {
		t.Errorf("Get KeysOnly SoS: expected entity data to be 'one', got '%v'", qiKOSRes[0].Data)
	}

	// Clear the local memory cache, because we want to test it not being filled incorrectly by a keys-only query
	n.FlushLocalCache()

	// Test the keys-only query with slice of pointers to struct
	qiKOPRes := []*QueryItem{}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "one").KeysOnly(), &qiKOPRes); err != nil {
		t.Errorf("GetAll KeysOnly SoPtS: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll KeysOnly SoPtS: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 1 {
		t.Errorf("GetAll KeysOnly SoPtS: expected key IntID to be 1, got %v", dskeys[0].IntID())
	} else if len(qiKOPRes) != 1 {
		t.Errorf("GetAll KeysOnly SoPtS: expected 1 result, got %v", len(qiKOPRes))
	} else if k := reflect.TypeOf(qiKOPRes[0]).Kind(); k != reflect.Ptr {
		t.Errorf("GetAll KeysOnly SoPtS: expected pointer, got %v", k)
	} else if qiKOPRes[0].Id != 1 {
		t.Errorf("GetAll KeysOnly SoPtS: expected entity id to be 1, got %v", qiKOPRes[0].Id)
	} else if qiKOPRes[0].Data != "" {
		t.Errorf("GetAll KeysOnly SoPtS: expected entity data to be empty, got '%v'", qiKOPRes[0].Data)
	}

	// Get the entity using normal Get to test that the local cache wasn't filled with incomplete data
	if err := n.GetMulti(qiKOPRes); err != nil {
		t.Errorf("Get KeysOnly SoPtS: unexpected error: %v", err)
	} else if qiKOPRes[0].Id != 1 {
		t.Errorf("Get KeysOnly SoPtS: expected entity id to be 1, got %v", qiKOPRes[0].Id)
	} else if qiKOPRes[0].Data != "one" {
		t.Errorf("Get KeysOnly SoPtS: expected entity data to be 'one', got '%v'", qiKOPRes[0].Data)
	}

	// Clear the local memory cache, because we want to test it not being filled incorrectly when supplying a non-zero slice
	n.FlushLocalCache()

	// Test the keys-only query with non-zero slice of structs
	qiKONZSRes := []QueryItem{{Id: 1, Data: "invalid cache"}}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "two").KeysOnly(), &qiKONZSRes); err != nil {
		t.Errorf("GetAll KeysOnly NZSoS: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll KeysOnly NZSoS: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 2 {
		t.Errorf("GetAll KeysOnly NZSoS: expected key IntID to be 2, got %v", dskeys[0].IntID())
	} else if len(qiKONZSRes) != 2 {
		t.Errorf("GetAll KeysOnly NZSoS: expected slice len to be 2, got %v", len(qiKONZSRes))
	} else if qiKONZSRes[0].Id != 1 {
		t.Errorf("GetAll KeysOnly NZSoS: expected entity id to be 1, got %v", qiKONZSRes[0].Id)
	} else if qiKONZSRes[0].Data != "invalid cache" {
		t.Errorf("GetAll KeysOnly NZSoS: expected entity data to be 'invalid cache', got '%v'", qiKONZSRes[0].Data)
	} else if k := reflect.TypeOf(qiKONZSRes[1]).Kind(); k != reflect.Struct {
		t.Errorf("GetAll KeysOnly NZSoS: expected struct, got %v", k)
	} else if qiKONZSRes[1].Id != 2 {
		t.Errorf("GetAll KeysOnly NZSoS: expected entity id to be 2, got %v", qiKONZSRes[1].Id)
	} else if qiKONZSRes[1].Data != "" {
		t.Errorf("GetAll KeysOnly NZSoS: expected entity data to be empty, got '%v'", qiKONZSRes[1].Data)
	}

	// Get the entities using normal GetMulti to test local cache
	if err := n.GetMulti(qiKONZSRes); err != nil {
		t.Errorf("GetMulti NZSoS: unexpected error: %v", err)
	} else if len(qiKONZSRes) != 2 {
		t.Errorf("GetMulti NZSoS: expected slice len to be 2, got %v", len(qiKONZSRes))
	} else if qiKONZSRes[0].Id != 1 {
		t.Errorf("GetMulti NZSoS: expected entity id to be 1, got %v", qiKONZSRes[0].Id)
	} else if qiKONZSRes[0].Data != "one" {
		t.Errorf("GetMulti NZSoS: expected entity data to be 'one', got '%v'", qiKONZSRes[0].Data)
	} else if qiKONZSRes[1].Id != 2 {
		t.Errorf("GetMulti NZSoS: expected entity id to be 2, got %v", qiKONZSRes[1].Id)
	} else if qiKONZSRes[1].Data != "two" {
		t.Errorf("GetMulti NZSoS: expected entity data to be 'two', got '%v'", qiKONZSRes[1].Data)
	}

	// Clear the local memory cache, because we want to test it not being filled incorrectly when supplying a non-zero slice
	n.FlushLocalCache()

	// Test the keys-only query with non-zero slice of pointers to struct
	qiKONZPRes := []*QueryItem{{Id: 1, Data: "invalid cache"}}
	if dskeys, err := n.GetAll(datastore.NewQuery("QueryItem").Filter("data=", "two").KeysOnly(), &qiKONZPRes); err != nil {
		t.Errorf("GetAll KeysOnly NZSoPtS: unexpected error: %v", err)
	} else if len(dskeys) != 1 {
		t.Errorf("GetAll KeysOnly NZSoPtS: expected 1 key, got %v", len(dskeys))
	} else if dskeys[0].IntID() != 2 {
		t.Errorf("GetAll KeysOnly NZSoPtS: expected key IntID to be 2, got %v", dskeys[0].IntID())
	} else if len(qiKONZPRes) != 2 {
		t.Errorf("GetAll KeysOnly NZSoPtS: expected slice len to be 2, got %v", len(qiKONZPRes))
	} else if qiKONZPRes[0].Id != 1 {
		t.Errorf("GetAll KeysOnly NZSoPtS: expected entity id to be 1, got %v", qiKONZPRes[0].Id)
	} else if qiKONZPRes[0].Data != "invalid cache" {
		t.Errorf("GetAll KeysOnly NZSoPtS: expected entity data to be 'invalid cache', got '%v'", qiKONZPRes[0].Data)
	} else if k := reflect.TypeOf(qiKONZPRes[1]).Kind(); k != reflect.Ptr {
		t.Errorf("GetAll KeysOnly NZSoPtS: expected pointer, got %v", k)
	} else if qiKONZPRes[1].Id != 2 {
		t.Errorf("GetAll KeysOnly NZSoPtS: expected entity id to be 2, got %v", qiKONZPRes[1].Id)
	} else if qiKONZPRes[1].Data != "" {
		t.Errorf("GetAll KeysOnly NZSoPtS: expected entity data to be empty, got '%v'", qiKONZPRes[1].Data)
	}

	// Get the entities using normal GetMulti to test local cache
	if err := n.GetMulti(qiKONZPRes); err != nil {
		t.Errorf("GetMulti NZSoPtS: unexpected error: %v", err)
	} else if len(qiKONZPRes) != 2 {
		t.Errorf("GetMulti NZSoPtS: expected slice len to be 2, got %v", len(qiKONZPRes))
	} else if qiKONZPRes[0].Id != 1 {
		t.Errorf("GetMulti NZSoPtS: expected entity id to be 1, got %v", qiKONZPRes[0].Id)
	} else if qiKONZPRes[0].Data != "one" {
		t.Errorf("GetMulti NZSoPtS: expected entity data to be 'one', got '%v'", qiKONZPRes[0].Data)
	} else if qiKONZPRes[1].Id != 2 {
		t.Errorf("GetMulti NZSoPtS: expected entity id to be 2, got %v", qiKONZPRes[1].Id)
	} else if qiKONZPRes[1].Data != "two" {
		t.Errorf("GetMulti NZSoPtS: expected entity data to be 'two', got '%v'", qiKONZPRes[1].Data)
	}
}

type keyTest struct {
	obj interface{}
	key *datastore.Key
}

type NoId struct {
}

type HasId struct {
	Id   int64 `datastore:"-" goon:"id"`
	Name string
}

type HasKind struct {
	Id   int64  `datastore:"-" goon:"id"`
	Kind string `datastore:"-" goon:"kind"`
	Name string
}

type HasDefaultKind struct {
	Id   int64  `datastore:"-" goon:"id"`
	Kind string `datastore:"-" goon:"kind,DefaultKind"`
	Name string
}

type QueryItem struct {
	Id   int64  `datastore:"-" goon:"id"`
	Data string `datastore:"data"`
}

type HasString struct {
	Id string `datastore:"-" goon:"id"`
}

type TwoId struct {
	IntId    int64  `goon:"id"`
	StringId string `goon:"id"`
}

type PutGet struct {
	ID    int64 `datastore:"-" goon:"id"`
	Value int32
}

// Commenting out for issue https://code.google.com/p/googleappengine/issues/detail?id=10493
//func TestMemcachePutTimeout(t *testing.T) {
//	c, err := aetest.NewContext(nil)
//	if err != nil {
//		t.Fatalf("Could not start aetest - %v", err)
//	}
//	defer c.Close()
//	g := FromContext(c)
//	MemcachePutTimeoutSmall = time.Second
//	// put a HasId resource, then test pulling it from memory, memcache, and datastore
//	hi := &HasId{Name: "hasid"} // no id given, should be automatically created by the datastore
//	if _, err := g.Put(hi); err != nil {
//		t.Errorf("put: unexpected error - %v", err)
//	}

//	MemcachePutTimeoutSmall = 0
//	MemcacheGetTimeout = 0
//	if err := g.putMemcache([]interface{}{hi}); !appengine.IsTimeoutError(err) {
//		t.Errorf("Request should timeout - err = %v", err)
//	}
//	MemcachePutTimeoutSmall = time.Second
//	MemcachePutTimeoutThreshold = 0
//	MemcachePutTimeoutLarge = 0
//	if err := g.putMemcache([]interface{}{hi}); !appengine.IsTimeoutError(err) {
//		t.Errorf("Request should timeout - err = %v", err)
//	}

//	MemcachePutTimeoutLarge = time.Second
//	if err := g.putMemcache([]interface{}{hi}); err != nil {
//		t.Errorf("putMemcache: unexpected error - %v", err)
//	}

//	g.FlushLocalCache()
//	memcache.Flush(c)
//	// time out Get
//	MemcacheGetTimeout = 0
//	// time out Put too
//	MemcachePutTimeoutSmall = 0
//	MemcachePutTimeoutThreshold = 0
//	MemcachePutTimeoutLarge = 0
//	hiResult := &HasId{Id: hi.Id}
//	if err := g.Get(hiResult); err != nil {
//		t.Errorf("Request should not timeout cause we'll fetch from the datastore but got error  %v", err)
//		// Put timing out should also error, but it won't be returned here, just logged
//	}
//	if !reflect.DeepEqual(hi, hiResult) {
//		t.Errorf("Fetched object isn't accurate - want %v, fetched %v", hi, hiResult)
//	}

//	hiResult = &HasId{Id: hi.Id}
//	g.FlushLocalCache()
//	MemcacheGetTimeout = time.Second
//	if err := g.Get(hiResult); err != nil {
//		t.Errorf("Request should not timeout cause we'll fetch from memcache successfully but got error %v", err)
//	}
//	if !reflect.DeepEqual(hi, hiResult) {
//		t.Errorf("Fetched object isn't accurate - want %v, fetched %v", hi, hiResult)
//	}
//}

// This test won't fail but if run with -race flag, it will show known race conditions
// Using multiple goroutines per http request is recommended here:
// http://talks.golang.org/2013/highperf.slide#22
func TestRace(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	g := FromContext(c)

	var hasIdSlice []*HasId
	for x := 1; x <= 4000; x++ {
		hasIdSlice = append(hasIdSlice, &HasId{Id: int64(x), Name: "Race"})
	}
	_, err = g.PutMulti(hasIdSlice)
	if err != nil {
		t.Fatalf("Could not put Race entities - %v", err)
	}
	hasIdSlice = hasIdSlice[:0]
	for x := 1; x <= 4000; x++ {
		hasIdSlice = append(hasIdSlice, &HasId{Id: int64(x)})
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		err := g.Get(hasIdSlice[0])
		if err != nil {
			t.Errorf("Error fetching id #0 - %v", err)
		}
		wg.Done()
	}()
	go func() {
		err := g.GetMulti(hasIdSlice[1:1500])
		if err != nil {
			t.Errorf("Error fetching ids 1 through 1499 - %v", err)
		}
		wg.Done()
	}()
	go func() {
		err := g.GetMulti(hasIdSlice[1500:])
		if err != nil {
			t.Errorf("Error fetching id #1500 through 4000 - %v", err)
		}
		wg.Done()
	}()
	wg.Wait()
	for x, hi := range hasIdSlice {
		if hi.Name != "Race" {
			t.Errorf("Object #%d not fetched properly, fetched instead - %v", x, hi)
		}
	}
}

func TestPutGet(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	g := FromContext(c)

	key, err := g.Put(&PutGet{ID: 12, Value: 15})
	if err != nil {
		t.Fatal(err)
	}
	if key.IntID() != 12 {
		t.Fatal("ID should be 12 but is", key.IntID())
	}

	// Datastore Get
	dsPutGet := &PutGet{}
	err = datastore.Get(c,
		datastore.NewKey(c, "PutGet", "", 12, nil), dsPutGet)
	if err != nil {
		t.Fatal(err)
	}
	if dsPutGet.Value != 15 {
		t.Fatal("dsPutGet.Value should be 15 but is",
			dsPutGet.Value)
	}

	// Goon Get
	goonPutGet := &PutGet{ID: 12}
	err = g.Get(goonPutGet)
	if err != nil {
		t.Fatal(err)
	}
	if goonPutGet.ID != 12 {
		t.Fatal("goonPutGet.ID should be 12 but is", goonPutGet.ID)
	}
	if goonPutGet.Value != 15 {
		t.Fatal("goonPutGet.Value should be 15 but is",
			goonPutGet.Value)
	}
}

func prefixKindName(src interface{}) string {
	return "prefix." + DefaultKindName(src)
}

func TestCustomKindName(t *testing.T) {
	opts := &aetest.Options{StronglyConsistentDatastore: true}
	c, err := aetest.NewContext(opts)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	g := FromContext(c)

	hi := HasId{Name: "Foo"}

	//gate
	if kind := g.Kind(hi); kind != "HasId" {
		t.Fatal("HasId King should not have a prefix, but instead is, ", kind)
	}

	g.KindNameResolver = prefixKindName

	if kind := g.Kind(hi); kind != "prefix.HasId" {
		t.Fatal("HasId King should have a prefix, but instead is, ", kind)
	}

	_, err = g.Put(&hi)

	if err != nil {
		t.Fatal("Should be able to put a record: ", err)
	}

	reget1 := []HasId{}
	query := datastore.NewQuery("prefix.HasId")
	query.GetAll(c, &reget1)

	if len(reget1) != 1 {
		t.Fatal("Should have 1 record stored in datastore ", reget1)
	}

	if reget1[0].Name != "Foo" {
		t.Fatal("Name should be Foo ", reget1[0].Name)
	}
}

func TestMultis(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	n := FromContext(c)

	testAmounts := []int{1, 999, 1000, 1001, 1999, 2000, 2001, 2510}
	for _, x := range testAmounts {
		memcache.Flush(c)
		objects := make([]*HasId, x)
		for y := 0; y < x; y++ {
			objects[y] = &HasId{Id: int64(y + 1)}
		}
		if _, err := n.PutMulti(objects); err != nil {
			t.Fatalf("Error in PutMulti for %d objects - %v", x, err)
		}
		n.FlushLocalCache() // Put just put them in the local cache, get rid of it before doing the Get
		if err := n.GetMulti(objects); err != nil {
			t.Fatalf("Error in GetMulti - %v", err)
		}
	}

	// do it again, but only write numbers divisible by 100
	for _, x := range testAmounts {
		memcache.Flush(c)
		getobjects := make([]*HasId, 0, x)
		putobjects := make([]*HasId, 0, x/100+1)
		keys := make([]*datastore.Key, x)
		for y := 0; y < x; y++ {
			keys[y] = datastore.NewKey(c, "HasId", "", int64(y+1), nil)
		}
		if err := n.DeleteMulti(keys); err != nil {
			t.Fatalf("Error deleting keys - %v", err)
		}
		for y := 0; y < x; y++ {
			getobjects = append(getobjects, &HasId{Id: int64(y + 1)})
			if y%100 == 0 {
				putobjects = append(putobjects, &HasId{Id: int64(y + 1)})
			}
		}

		_, err := n.PutMulti(putobjects)
		if err != nil {
			t.Fatalf("Error in PutMulti for %d objects - %v", x, err)
		}
		n.FlushLocalCache() // Put just put them in the local cache, get rid of it before doing the Get
		err = n.GetMulti(getobjects)
		if err == nil && x != 1 { // a test size of 1 has no objects divisible by 100, so there's no cache miss to return
			t.Errorf("Should be receiving a multiError on %d objects, but got no errors", x)
			continue
		}

		merr, ok := err.(appengine.MultiError)
		if ok {
			if len(merr) != len(getobjects) {
				t.Errorf("Should have received a MultiError object of length %d but got length %d instead", len(getobjects), len(merr))
			}
			for x := range merr {
				switch { // record good conditions, fail in other conditions
				case merr[x] == nil && x%100 == 0:
				case merr[x] != nil && x%100 != 0:
				default:
					t.Errorf("Found bad condition on object[%d] and error %v", x+1, merr[x])
				}
			}
		} else if x != 1 {
			t.Errorf("Did not return a multierror on fetch but when fetching %d objects, received - %v", x, merr)
		}
	}
}

type root struct {
	Id   int64 `datastore:"-" goon:"id"`
	Data int
}

type normalChild struct {
	Id     int64          `datastore:"-" goon:"id"`
	Parent *datastore.Key `datastore:"-" goon:"parent"`
	Data   int
}

type coolKey *datastore.Key

type derivedChild struct {
	Id     int64   `datastore:"-" goon:"id"`
	Parent coolKey `datastore:"-" goon:"parent"`
	Data   int
}

func TestParents(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Could not start aetest - %v", err)
	}
	defer c.Close()
	n := FromContext(c)

	r := &root{1, 10}
	rootKey, err := n.Put(r)
	if err != nil {
		t.Fatalf("couldn't Put(%+v)", r)
	}

	// Put exercises both get and set, since Id is uninitialized
	nc := &normalChild{0, rootKey, 20}
	nk, err := n.Put(nc)
	if err != nil {
		t.Fatalf("couldn't Put(%+v)", nc)
	}
	if nc.Parent == rootKey {
		t.Fatalf("derived parent key pointer value didn't change")
	}
	if !(*datastore.Key)(nc.Parent).Equal(rootKey) {
		t.Fatalf("parent of key not equal '%s' v '%s'! ", (*datastore.Key)(nc.Parent), rootKey)
	}
	if !nk.Parent().Equal(rootKey) {
		t.Fatalf("parent of key not equal '%s' v '%s'! ", nk, rootKey)
	}

	dc := &derivedChild{0, (coolKey)(rootKey), 12}
	dk, err := n.Put(dc)
	if err != nil {
		t.Fatalf("couldn't Put(%+v)", dc)
	}
	if dc.Parent == rootKey {
		t.Fatalf("derived parent key pointer value didn't change")
	}
	if !(*datastore.Key)(dc.Parent).Equal(rootKey) {
		t.Fatalf("parent of key not equal '%s' v '%s'! ", (*datastore.Key)(dc.Parent), rootKey)
	}
	if !dk.Parent().Equal(rootKey) {
		t.Fatalf("parent of key not equal '%s' v '%s'! ", dk, rootKey)
	}
}
