//go:build windows

package app

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	modDXGI                = syscall.NewLazyDLL("dxgi.dll")
	procCreateDXGIFactory1 = modDXGI.NewProc("CreateDXGIFactory1")
	iidIDXGIFactory1       = guid{
		Data1: 0x770aae78,
		Data2: 0xf26f,
		Data3: 0x4dba,
		Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87},
	}
)

type dxgiAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLUID           [2]uint32
	Flags                 uint32
}

const (
	dxgiErrorNotFound       = 0x887A0002
	dxgiAdapterFlagSoftware = 2
)

func vendorLabel(vendorID uint32) string {
	switch vendorID {
	case 0x10DE:
		return "NVIDIA"
	case 0x1002, 0x1022:
		return "AMD"
	case 0x8086:
		return "Intel"
	default:
		return fmt.Sprintf("PCI:%04x", vendorID)
	}
}

type dxgiFactory1Obj struct {
	vtbl *[14]uintptr
}

type dxgiAdapter1Obj struct {
	vtbl *[11]uintptr
}

func enumeratePlatformGPUs() []GPU {
	if err := procCreateDXGIFactory1.Find(); err != nil {
		return nil
	}
	var factory *dxgiFactory1Obj
	hr, _, _ := procCreateDXGIFactory1.Call(
		uintptr(unsafe.Pointer(&iidIDXGIFactory1)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if hr != 0 || factory == nil || factory.vtbl == nil {
		return nil
	}
	factoryPtr := uintptr(unsafe.Pointer(factory))
	defer syscall.SyscallN(factory.vtbl[2], factoryPtr) // IUnknown::Release

	var out []GPU
	for idx := uint32(0); idx < 16; idx++ {
		var adapter *dxgiAdapter1Obj
		r, _, _ := syscall.SyscallN(
			factory.vtbl[12], // IDXGIFactory1::EnumAdapters1
			factoryPtr,
			uintptr(idx),
			uintptr(unsafe.Pointer(&adapter)),
		)
		if uint32(r) == dxgiErrorNotFound || r != 0 || adapter == nil || adapter.vtbl == nil {
			break
		}
		adapterPtr := uintptr(unsafe.Pointer(adapter))
		var desc dxgiAdapterDesc1
		gr, _, _ := syscall.SyscallN(
			adapter.vtbl[10], // IDXGIAdapter1::GetDesc1
			adapterPtr,
			uintptr(unsafe.Pointer(&desc)),
		)
		_, _, _ = syscall.SyscallN(adapter.vtbl[2], adapterPtr) // IUnknown::Release
		if gr != 0 {
			continue
		}
		if desc.Flags&dxgiAdapterFlagSoftware != 0 {
			continue
		}
		name := strings.TrimSpace(syscall.UTF16ToString(desc.Description[:]))
		if name == "" {
			continue
		}
		vram := uint64(desc.DedicatedVideoMemory)
		avail := vram
		if vram == 0 && desc.SharedSystemMemory > 0 {
			// Integrated GPU using shared system memory
			vram = uint64(desc.SharedSystemMemory)
			avail = vram / 2
		}
		out = append(out, GPU{
			Name:      name,
			Driver:    "DXGI (" + vendorLabel(desc.VendorID) + ")",
			Total:     vram,
			Available: avail,
		})
	}
	return out
}
