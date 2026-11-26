package traffic

import (
	"bufio"
	"bytes"
	json "encoding/json/v2"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func readBridge(bridge *net.UnixConn) (net.Conn, flow, error) {
	_ = bridge.SetDeadline(time.Now().Add(time.Second))
	var data [4096]byte
	control := make([]byte, unix.CmsgSpace(4))
	n, used, flags, _, err := bridge.ReadMsgUnix(data[:], control)
	if err != nil {
		return nil, flow{}, err
	}
	messages, err := unix.ParseSocketControlMessage(control[:used])
	if err != nil {
		return nil, flow{}, err
	}
	var files []*os.File
	defer func() {
		for _, file := range files {
			file.Close()
		}
	}()
	for _, message := range messages {
		fds, err := unix.ParseUnixRights(&message)
		if err != nil {
			return nil, flow{}, err
		}
		for _, fd := range fds {
			unix.CloseOnExec(fd)
			files = append(files, os.NewFile(uintptr(fd), "capture-upstream"))
		}
	}
	if flags&unix.MSG_CTRUNC != 0 || len(files) != 1 {
		return nil, flow{}, errBridge
	}
	reader := bufio.NewReaderSize(io.MultiReader(bytes.NewReader(data[:n]), bridge), len(data))
	line, err := reader.ReadSlice('\n')
	var metadata flow
	if err != nil || json.Unmarshal(line, &metadata) != nil || metadata.Destination == "" {
		return nil, flow{}, errBridge
	}
	if _, _, err := net.SplitHostPort(metadata.Destination); err != nil {
		return nil, flow{}, errBridge
	}
	upstream, err := net.FileConn(files[0])
	if err != nil {
		return nil, flow{}, err
	}
	return upstream, metadata, nil
}
