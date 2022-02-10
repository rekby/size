package sizeof

import (
	"errors"
	"reflect"
	"sort"
	"unsafe"

	"github.com/rekby/objwalker"
)

var (
	ErrMustBePointer = errors.New("v must be a pointer")
)

const (
	maxAlign = 8

	mapItemOverhead = 13 // average map item overhead from go 1.17 https://github.com/golang/go/blob/6a70ee2873b2367e2a0d6e7d7e167c072b99daf0/src/runtime/map.go#L41
	hmapSize        = unsafe.Sizeof(hmap{}) + uintptr(-int(unsafe.Sizeof(hmap{}))&(maxAlign-1))
)

// Of return size of object, calculated by reflection walk
// if some errors happened it return err AND calced size, which can be used
// as the best effort size
func Of(v interface{}) (int, error) {
	if v == nil {
		return 0, nil
	}
	if reflect.ValueOf(v).Kind() != reflect.Ptr {
		return 0, ErrMustBePointer
	}

	calcState := newState()
	err := objwalker.New(calcState.addObject).Walk(v)
	size := calcState.calcSize()
	return size, err
}

type empty struct{}

type memRange struct {
	First, Last uintptr
}

type state struct {
	ranges             []memRange
	visited            map[uintptr]empty
	unaddressedSizeVal uintptr
	err                error
}

func (s *state) addUnaddressableValue(size uintptr) {
	s.unaddressedSizeVal += upToCluster(size)
}

func newState() *state {
	return &state{
		visited: map[uintptr]empty{},
	}
}

func (s *state) addObject(info *objwalker.WalkInfo) error {
	if info.HasDirectPointer() {
		s.addRange(info.DirectPointer, info.Value.Type().Size())
	} else {
		s.addUnaddressableValue(info.Value.Type().Size())
	}

	switch info.Value.Kind() {
	case reflect.Slice:
		s.addSlice(info)
	case reflect.String:
		s.addString(info)
	case reflect.Map:
		s.addMap(info)
	case reflect.Chan:
		s.addChan(info)
	default:
		// pass
	}
	return nil
}

func (s *state) addMap(info *objwalker.WalkInfo) {
	s.addUnaddressableValue(uintptr(info.Value.Len()) * mapItemOverhead)
	s.addUnaddressableValue(hmapSize)
}

func (s *state) addChan(info *objwalker.WalkInfo) {
	if info.HasDirectPointer() {
		pointerToOriginalChanVariable := info.DirectPointer
		pointerToPointerToHChan := (*unsafe.Pointer)(pointerToOriginalChanVariable)
		pointerToHChan := uintptr(*pointerToPointerToHChan)
		if _, ok := s.visited[pointerToHChan]; ok {
			return
		}
		s.visited[pointerToHChan] = empty{}
	}
	itemSize := info.Value.Type().Elem().Size()
	s.addUnaddressableValue(itemSize * uintptr(info.Value.Cap()))
}

func (s *state) addSlice(info *objwalker.WalkInfo) {
	itemSize := info.Value.Type().Elem().Size()
	sliceSize := itemSize * uintptr(info.Value.Cap())
	if info.HasDirectPointer() {
		sliceHeader := (*reflect.SliceHeader)(info.DirectPointer)
		s.addRange(unsafe.Pointer(sliceHeader.Data), sliceSize)
	} else {
		s.addUnaddressableValue(sliceSize)
	}
}

func (s *state) addString(info *objwalker.WalkInfo) {
	if info.HasDirectPointer() {
		stringHeader := (*reflect.StringHeader)(info.DirectPointer)
		s.addRange(unsafe.Pointer(stringHeader.Data), uintptr(info.Value.Len()))
	} else {
		s.addUnaddressableValue(uintptr(info.Value.Len()))
	}
}

func (s *state) addRange(start unsafe.Pointer, length uintptr) {
	if length == 0 {
		return
	}

	s.ranges = append(s.ranges, memRange{First: uintptr(start), Last: uintptr(start) + length - 1})
}

func (s *state) calcSize() int {
	s.compress()
	return s.sum()
}

func (s *state) compress() {
	sort.Slice(s.ranges, func(i, j int) bool {
		left, right := s.ranges[i], s.ranges[j]
		switch {
		case left.First < right.First:
			return true
		case left.First == right.First && left.Last > right.Last:
			return true
		default:
			return false
		}
	})

	res := s.ranges[:0]
	var lastAddedRange memRange
	for _, r := range s.ranges {
		if r.First == lastAddedRange.First {
			continue
		}
		if r.Last <= lastAddedRange.Last {
			continue
		}
		res = append(res, r)
		lastAddedRange = r
	}
	s.ranges = res
}

func (s *state) sum() int {
	var res uintptr
	for _, r := range s.ranges {
		res += upToCluster(r.Last - r.First + 1)
	}
	res += s.unaddressedSizeVal
	var zeroPointer unsafe.Pointer
	res -= unsafe.Sizeof(zeroPointer)
	return int(res)
}

const _NumSizeClasses = 68

var class_to_size = [_NumSizeClasses]uintptr{0, 8, 16, 24, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192, 208, 224, 240, 256, 288, 320, 352, 384, 416, 448, 480, 512, 576, 640, 704, 768, 896, 1024, 1152, 1280, 1408, 1536, 1792, 2048, 2304, 2688, 3072, 3200, 3456, 4096, 4864, 5376, 6144, 6528, 6784, 6912, 8192, 9472, 9728, 10240, 10880, 12288, 13568, 14336, 16384, 18432, 19072, 20480, 21760, 24576, 27264, 28672, 32768}

const (
	_PageShift = 13
	_PageSize  = 1 << _PageShift
)

func upToCluster(size uintptr) uintptr {
	for _, clusterSize := range class_to_size {
		if size <= clusterSize {
			return clusterSize
		}
	}
	if size%_PageSize != 0 {
		size = ((size / _PageSize) + 1) * _PageSize
	}
	return size
}
