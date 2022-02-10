package sizeof

import (
	"math"
	"net/url"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestOf(t *testing.T) {
	var zeroPointer unsafe.Pointer
	pointerSize := unsafe.Sizeof(zeroPointer)

	vInt32 := int32(0)
	vInt64 := int64(0)
	vInt := 0

	emptySliceWithCapacity := make([]int32, 5)
	vBool := false
	vBoolPointer := &vBool

	table := []struct {
		name string
		obj  interface{}
		size uintptr
	}{
		{"nil", nil, 0},
		{"Int", &vInt, upToCluster(unsafe.Sizeof(vInt))},
		{"Int32", &vInt32, upToCluster(4)},
		{"Int64", &vInt64, upToCluster(8)},
		{"Int32Arr", &[4]int32{1, 2, 3, 4}, upToCluster(16)},
		{"Int32Slice", &[]int32{1, 2, 3, 4}, upToCluster(unsafe.Sizeof(reflect.SliceHeader{})) + 16},
		{"Int32EmptySliceWithCapacity", &emptySliceWithCapacity, upToCluster(unsafe.Sizeof(reflect.SliceHeader{})) + upToCluster(20)},
		{"bool", &vBool, upToCluster(1)},
		{"boolPointer", &vBoolPointer, pointerSize + upToCluster(1)}, // pointer size + 1
		{"StructZero", &struct{}{}, upToCluster(0)},
		{"StructInt32", &struct{ v int32 }{}, upToCluster(4)},
		{"StructInt32", &struct{ v1, v2 int32 }{}, upToCluster(8)},
		{"StructUnAligned", &struct {
			v1 bool
			v2 int32
		}{}, upToCluster(unsafe.Offsetof(struct {
			v1 bool
			v2 int32
		}{}.
			v2) + 4)},
		{"StructHierarcy", &struct {
			v struct{ v int32 }
		}{}, upToCluster(4)},
	}

	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			size, err := Of(test.obj)
			require.NoError(t, err)
			require.Equal(t, test.size, uintptr(size))
		})
	}
}

var (
	arrPointer interface{}
)

func TestRealDatameasure(t *testing.T) {
	type itemLoad struct {
		url url.URL
		val int
	}

	type itemDescription struct {
		isTrue    bool
		counter   int
		user      *string
		data      []byte
		dataStart []byte
	}

	type itemType struct {
		title string
		load  itemLoad
		desc  *itemDescription
	}

	strings := []string{"asd", "sdfad", "dfasfsdf", "dfasdfasdf"}
	dataLen := 1000
	createitem := func() *itemType {
		data := make([]byte, dataLen)
		for i := 0; i < len(data); i++ {
			data[i] = byte(i % 255)
		}
		user := strings[0]
		desc := itemDescription{
			user:      &user,
			data:      data,
			dataStart: data[:10],
		}
		load := itemLoad{
			url: url.URL{
				Scheme: strings[1],
				Host:   strings[2],
				Path:   strings[3],
			},
			val: 123,
		}
		item := itemType{
			title: strings[1],
			load:  load,
			desc:  &desc,
		}
		return &item
	}

	itemCount := 100000
	arr := make([]*itemType, itemCount)
	arrPointer = arr
	runtime.GC()
	var memStatStart runtime.MemStats
	runtime.ReadMemStats(&memStatStart)
	for i := range arr {
		arr[i] = createitem()
	}
	runtime.GC()
	var memStatFinish runtime.MemStats
	runtime.ReadMemStats(&memStatFinish)

	totalSize := memStatFinish.HeapAlloc - memStatStart.HeapAlloc
	itemSize := int(totalSize) / itemCount

	for _, s := range strings {
		itemSize += len(s)
	}

	calcSize, err := Of(arr[0])

	require.NoError(t, err)
	require.True(t, math.Abs(float64(calcSize)-float64(itemSize)) < 20)
}
