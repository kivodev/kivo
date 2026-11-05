package ffmpeg

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// adapter is a GPU as DXGI lists it. index is its place in DXGI's list,
// which is how ffmpeg names a D3D11 device.
type adapter struct {
	index  int
	vendor uint32 // its PCI vendor ID
}

var (
	createDXGIFactory1 = windows.NewLazySystemDLL("dxgi.dll").NewProc("CreateDXGIFactory1")
	iidDXGIFactory1    = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba,
		Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
)

// The methods called, by their place in the vtables of IDXGIFactory1 and
// IDXGIAdapter1.
const (
	methodRelease      = 2
	methodGetDesc1     = 10
	methodEnumAdapters = 12 // EnumAdapters1
)

// adapterDesc is DXGI_ADAPTER_DESC1, of which only the vendor is read.
type adapterDesc struct {
	_        [128]uint16 // Description
	vendorID uint32
	_        [3]uint32  // DeviceId, SubSysId, Revision
	_        [3]uintptr // the memory sizes
	_        windows.LUID
	_        uint32 // Flags
}

// comObject is a COM interface pointer.
type comObject struct{ vtbl *[16]uintptr }

func (o *comObject) call(method int, args ...uintptr) int32 {
	hr, _, _ := syscall.SyscallN(o.vtbl[method], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return int32(hr)
}

// adapters lists the PC's GPUs in DXGI's order. It lists none if DXGI
// can't be reached.
func adapters() []adapter {
	if createDXGIFactory1.Find() != nil {
		return nil
	}
	var factory *comObject
	if hr, _, _ := createDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidDXGIFactory1)), uintptr(unsafe.Pointer(&factory))); int32(hr) < 0 || factory == nil {
		return nil
	}
	defer factory.call(methodRelease)
	var list []adapter
	for i := 0; ; i++ {
		var a *comObject
		if factory.call(methodEnumAdapters, uintptr(i), uintptr(unsafe.Pointer(&a))) < 0 || a == nil {
			return list // past the last adapter
		}
		var d adapterDesc
		hr := a.call(methodGetDesc1, uintptr(unsafe.Pointer(&d)))
		a.call(methodRelease)
		if hr >= 0 {
			list = append(list, adapter{index: i, vendor: d.vendorID})
		}
	}
}
