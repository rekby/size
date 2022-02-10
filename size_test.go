package sizeof

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestOf(t *testing.T) {
	var zeroPointer unsafe.Pointer
	pointerSize := int(unsafe.Sizeof(zeroPointer))

	vInt32 := int32(0)
	vInt64 := int64(0)
	vInt := 0

	emptySliceWithCapacity := make([]int32, 5)
	vBool := false
	vBoolPointer := &vBool

	table := []struct {
		name string
		obj  interface{}
		size int
	}{
		{"nil", nil, 0},
		{"Int", &vInt, int(unsafe.Sizeof(vInt))},
		{"Int32", &vInt32, 4},
		{"Int64", &vInt64, 8},
		{"Int32Arr", &[4]int32{1, 2, 3, 4}, 16},
		{"Int32Slice", &[]int32{1, 2, 3, 4}, int(unsafe.Sizeof(reflect.SliceHeader{})) + 16},
		{"Int32EmptySliceWithCapacity", &emptySliceWithCapacity, int(unsafe.Sizeof(reflect.SliceHeader{})) + 20},
		{"bool", &vBool, 1},
		{"boolPointer", &vBoolPointer, pointerSize + int(unsafe.Alignof(vBool))}, // pointer size + 1
		{"StructZero", &struct{}{}, 0},
		{"StructInt32", &struct{ v int32 }{}, 4},
		{"StructInt32", &struct{ v1, v2 int32 }{}, 8},
		{"StructUnAligned", &struct {
			v1 bool
			v2 int32
		}{}, int(unsafe.Offsetof(struct {
			v1 bool
			v2 int32
		}{}.
			v2)) + 4},
		{"StructHierarcy", &struct {
			v struct{ v int32 }
		}{}, 4},
	}

	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			size, err := Of(test.obj)
			require.NoError(t, err)
			require.Equal(t, test.size, size)
		})
	}
}
