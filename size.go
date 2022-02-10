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
	ranges          []memRange
	visited         map[uintptr]empty
	unaddressedSize uintptr
	err             error
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
		s.unaddressedSize += info.Value.Type().Size()
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
	s.unaddressedSize += uintptr(info.Value.Len()) * mapItemOverhead
	s.unaddressedSize += hmapSize
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
	s.unaddressedSize += itemSize * uintptr(info.Value.Cap())
}

func (s *state) addSlice(info *objwalker.WalkInfo) {
	itemSize := info.Value.Type().Elem().Size()
	sliceSize := itemSize * uintptr(info.Value.Cap())
	if info.HasDirectPointer() {
		sliceHeader := (*reflect.SliceHeader)(info.DirectPointer)
		s.addRange(unsafe.Pointer(sliceHeader.Data), sliceSize)
	} else {
		s.unaddressedSize += sliceSize
	}
}

func (s *state) addString(info *objwalker.WalkInfo) {
	if info.HasDirectPointer() {
		stringHeader := (*reflect.StringHeader)(info.DirectPointer)
		s.addRange(unsafe.Pointer(stringHeader.Data), uintptr(info.Value.Len()))
	} else {
		s.unaddressedSize += uintptr(info.Value.Len())
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
		res += r.Last - r.First + 1
	}
	res += s.unaddressedSize
	var zeroPointer unsafe.Pointer
	res -= unsafe.Sizeof(zeroPointer)
	return int(res)
}
