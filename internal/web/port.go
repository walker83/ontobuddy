package web

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

// isAddrInUse 检查错误是否为"地址已被使用"。
func isAddrInUse(err error) bool {
	if opErr, ok := err.(*net.OpError); ok {
		if sysErr, ok := opErr.Err.(*os.SyscallError); ok {
			return sysErr.Err == syscall.EADDRINUSE
		}
	}
	return false
}

// isOwnServer 检查指定地址上运行的是否是自己的进程。
// 通过尝试连接并检查响应头来判断。
func isOwnServer(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 200*1e6) // 200ms
	if err != nil {
		return false
	}
	defer conn.Close()

	// 设置读写超时
	conn.SetDeadline(time.Now().Add(500 * time.Millisecond))

	fmt.Fprintf(conn, "GET /api/stats HTTP/1.0\r\nHost: %s\r\n\r\n", addr)
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}
	resp := string(buf[:n])

	// 检查是否是 myonto 的响应（包含 total_triples 字段）
	return strings.Contains(resp, "total_triples")
}

// FindAvailablePort 从指定端口开始查找可用端口。
func FindAvailablePort(start int) (int, error) {
	for port := start; port < start+100; port++ {
		addr := fmt.Sprintf("localhost:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", start, start+99)
}
