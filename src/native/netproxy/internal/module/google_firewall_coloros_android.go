//go:build android && arm64

package module

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ColorOS 16 uses three shared socket filters. Keep the filters in place and
// remove only the matching Google UID entries, never a default or wildcard.
func repairColorOSGoogleFirewall(uids map[uint32]bool) (int, error) {
	var removed int
	var failures error
	for _, kind := range []string{"wlan", "qcom", "mtk"} {
		count, err := repairColorOSGoogleMap(kind, uids)
		removed += count
		if err != nil && !errors.Is(err, unix.ENOENT) {
			failures = errors.Join(failures, fmt.Errorf("ColorOS %s: %w", kind, err))
		}
	}
	return removed, failures
}

func repairColorOSGoogleMap(kind string, uids map[uint32]bool) (int, error) {
	prog, err := openPinnedColorOSBPF("/sys/fs/bpf/prog_oplus-netd_skfilter_reject_" + kind + "_uid")
	if err != nil {
		return 0, err
	}
	defer unix.Close(prog)
	m, err := openPinnedColorOSBPF("/sys/fs/bpf/map_oplus-netd_app_" + kind + "_socket_uid_limit_map")
	if err != nil {
		return 0, err
	}
	defer unix.Close(m)

	var mapInfo struct{ Type, ID, KeySize, ValueSize uint32 }
	if err := colorOSBPFInfo(m, unsafe.Pointer(&mapInfo), unsafe.Sizeof(mapInfo)); err != nil {
		return 0, err
	}
	var id uint32
	// Prefix of Linux UAPI struct bpf_prog_info, through map_ids.
	progInfo := struct {
		Type, ID            uint32
		Tag                 [8]byte
		JitedLen, XlatedLen uint32
		Jited, Xlated       uint64
		LoadTime            uint64
		CreatedByUID        uint32
		MapCount            uint32
		MapIDs              unsafe.Pointer
	}{MapCount: 1, MapIDs: unsafe.Pointer(&id)}
	if err := colorOSBPFInfo(prog, unsafe.Pointer(&progInfo), unsafe.Sizeof(progInfo)); err != nil {
		return 0, err
	}
	tag := [8]byte{0x71, 0x0a, 0xde, 0xf2, 0xa6, 0x3c, 0x67, 0xac}
	if progInfo.Type != unix.BPF_PROG_TYPE_SOCKET_FILTER || progInfo.Tag != tag ||
		progInfo.MapCount != 1 || id != mapInfo.ID || mapInfo.Type != unix.BPF_MAP_TYPE_HASH ||
		mapInfo.KeySize != 4 || mapInfo.ValueSize != 4 {
		return 0, errors.New("未识别的厂商 BPF 程序或表结构，已跳过")
	}
	count := 0
	for uid := range uids {
		var value uint32
		attr := struct {
			FD, _      uint32
			Key, Value unsafe.Pointer
		}{FD: uint32(m), Key: unsafe.Pointer(&uid), Value: unsafe.Pointer(&value)}
		_, err := colorOSBPF(unix.BPF_MAP_LOOKUP_ELEM, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return count, err
		}
		if value != 1 {
			continue
		}
		attr.Value = nil
		_, err = colorOSBPF(unix.BPF_MAP_DELETE_ELEM, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func openPinnedColorOSBPF(path string) (int, error) {
	name, err := unix.BytePtrFromString(path)
	if err != nil {
		return 0, err
	}
	attr := struct{ Path *byte }{Path: name}
	fd, err := colorOSBPF(unix.BPF_OBJ_GET, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	return int(fd), err
}

func colorOSBPFInfo(fd int, info unsafe.Pointer, size uintptr) error {
	attr := struct {
		FD, Len uint32
		Info    unsafe.Pointer
	}{uint32(fd), uint32(size), info}
	_, err := colorOSBPF(unix.BPF_OBJ_GET_INFO_BY_FD, unsafe.Pointer(&attr), unsafe.Sizeof(attr))
	return err
}

func colorOSBPF(cmd uintptr, attr unsafe.Pointer, size uintptr) (uintptr, error) {
	ret, _, errno := unix.Syscall(unix.SYS_BPF, cmd, uintptr(attr), size)
	runtime.KeepAlive(attr)
	if errno != 0 {
		return 0, errno
	}
	return ret, nil
}
