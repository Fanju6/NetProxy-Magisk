//go:build !linux

package traffic

import "net"

func readBridge(*net.UnixConn) (net.Conn, flow, error) { return nil, flow{}, errBridge }
