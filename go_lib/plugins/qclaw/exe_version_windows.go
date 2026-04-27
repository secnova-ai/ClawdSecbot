//go:build windows

package qclaw

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modversion                  = windows.NewLazySystemDLL("version.dll")
	procGetFileVersionInfoSizeW = modversion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = modversion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = modversion.NewProc("VerQueryValueW")
)

// verQueryValuePtr returns a pointer and raw length for a version-info sub-block.
func verQueryValuePtr(data []byte, subBlock string) (uintptr, uint32, bool) {
	if len(data) == 0 {
		return 0, 0, false
	}
	sb, err := windows.UTF16PtrFromString(subBlock)
	if err != nil {
		return 0, 0, false
	}
	var outPtr uintptr
	var outLen uint32
	r, _, _ := procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(sb)),
		uintptr(unsafe.Pointer(&outPtr)),
		uintptr(unsafe.Pointer(&outLen)),
	)
	if r == 0 || outPtr == 0 || outLen == 0 {
		return 0, 0, false
	}
	return outPtr, outLen, true
}

// verQueryValueBytes returns a byte view for binary version-info sub-blocks.
func verQueryValueBytes(data []byte, subBlock string) ([]byte, bool) {
	outPtr, outLen, ok := verQueryValuePtr(data, subBlock)
	if !ok {
		return nil, false
	}
	return versionDataSlice(data, outPtr, uintptr(outLen))
}

// verQueryValueUTF16String returns a decoded UTF-16 string sub-block.
func verQueryValueUTF16String(data []byte, subBlock string) (string, bool) {
	outPtr, outLen, ok := verQueryValuePtr(data, subBlock)
	if !ok {
		return "", false
	}
	raw, ok := versionDataSlice(data, outPtr, uintptr(outLen)*2)
	if !ok || len(raw) < 2 {
		return "", false
	}
	return decodeUTF16LEVersionString(raw), true
}

// versionDataSlice returns a bounded slice view into the GetFileVersionInfo buffer.
func versionDataSlice(data []byte, outPtr uintptr, byteLen uintptr) ([]byte, bool) {
	if len(data) == 0 || byteLen == 0 {
		return nil, false
	}
	base := uintptr(unsafe.Pointer(&data[0]))
	end := base + uintptr(len(data))
	if outPtr < base || outPtr >= end {
		return nil, false
	}
	off := int(outPtr - base)
	if byteLen > uintptr(len(data)-off) {
		return nil, false
	}
	return data[off : off+int(byteLen)], true
}

// readQClawExecutableVersion reads the FileVersion string from Windows PE version resources.
func readQClawExecutableVersion(exePath string) string {
	exePath = strings.TrimSpace(exePath)
	if exePath == "" {
		return ""
	}
	pathPtr, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return ""
	}
	var verHandle uint32
	size, _, _ := procGetFileVersionInfoSizeW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&verHandle)),
	)
	if size == 0 {
		return ""
	}
	data := make([]byte, size)
	gfiRet, _, _ := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(verHandle),
		size,
		uintptr(unsafe.Pointer(&data[0])),
	)
	if gfiRet == 0 {
		return ""
	}
	subBlock, err := fileVersionSubBlock(data)
	if err != nil {
		return ""
	}
	version, ok := verQueryValueUTF16String(data, subBlock)
	if !ok {
		return ""
	}
	return version
}

// fileVersionSubBlock builds the \\StringFileInfo\\{lang:cp}\\FileVersion query path.
func fileVersionSubBlock(data []byte) (string, error) {
	trans, ok := verQueryValueBytes(data, `\VarFileInfo\Translation`)
	if !ok || len(trans) < 4 {
		return "", fmt.Errorf("no translation block")
	}
	d := binary.LittleEndian.Uint32(trans[:4])
	// Translation stores language ID in the low word and code page in the high word.
	lang := d & 0xffff
	cp := (d >> 16) & 0xffff
	return fmt.Sprintf(`\StringFileInfo\%04x%04x\FileVersion`, lang, cp), nil
}

// decodeUTF16LEVersionString decodes a UTF-16 LE byte slice and trims trailing NUL.
func decodeUTF16LEVersionString(raw []byte) string {
	if len(raw) < 2 {
		return ""
	}
	n := len(raw) / 2
	u16 := make([]uint16, n)
	for i := 0; i < n; i++ {
		u16[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	for n > 0 && u16[n-1] == 0 {
		n--
	}
	if n == 0 {
		return ""
	}
	return string(utf16.Decode(u16[:n]))
}
