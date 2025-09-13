//go:build linux

package client

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/amitschendel/curing/pkg/common"
	"github.com/amitschendel/curing/pkg/config"
	"github.com/iceber/iouring-go"
)

type CommandPuller struct {
	executer   IExecuter
	ring       *iouring.IOURing
	cfg        *config.Config
	resultChan chan iouring.Result
	ctx        context.Context
	cancelFunc context.CancelFunc
	interval   time.Duration
	closeOnce  sync.Once
}

func NewCommandPuller(cfg *config.Config, ctx context.Context, executer IExecuter) (*CommandPuller, error) {
	ring, err := iouring.New(32)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return &CommandPuller{
		executer:   executer,
		ring:       ring,
		cfg:        cfg,
		ctx:        ctx,
		cancelFunc: cancel,
		resultChan: make(chan iouring.Result, 32),
		interval:   time.Duration(cfg.ConnectIntervalSec) * time.Second,
	}, nil
}

// SetInterval allows configuring the connection interval
func (cp *CommandPuller) SetInterval(d time.Duration) {
	cp.interval = d
}

func (cp *CommandPuller) Run() {
	ticker := time.NewTicker(cp.interval)
	defer ticker.Stop()

	slog.Info("Starting CommandPuller")
	cp.connectReadAndProcess()

	for {
		select {
		case <-cp.ctx.Done():
			cp.Close()
			return
		case <-ticker.C:
			cp.connectReadAndProcess()
		}
	}
}

func (cp *CommandPuller) connectReadAndProcess() {
	// Connect
	conn, err := cp.connect()
	if err != nil {
		slog.Error("Error connecting to server", "error", err)
		return
	}

	defer func() {
		if err := cp.close(conn); err != nil {
			slog.Error("Error closing connection", "error", err)
		}
	}()

	// Create NetworkRWer
	urw := &NetworkRWer{
		conn:       conn,
		resultChan: cp.resultChan,
		ring:       cp.ring,
		useTCP:     cp.cfg.UseTCPNetwork,
	}

	// Send GetCommands request
	req := &common.Request{
		AgentID: cp.cfg.AgentID,
		Groups:  cp.cfg.Groups,
		Type:    common.GetCommands,
	}
	if err := cp.sendGobRequest(urw, req); err != nil {
		slog.Error("Error sending request", "error", err)
		return
	}

	// Read and decode commands with retries
	commands, err := cp.readGobCommands(urw)
	if err != nil {
		slog.Error("Error reading commands", "error", err)
		return
	}

	if len(commands) > 0 {
		cp.processCommands(commands)
	}
}

func (cp *CommandPuller) sendGobRequest(urw *NetworkRWer, req *common.Request) error {
	encoder := gob.NewEncoder(urw)
	if err := encoder.Encode(req); err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	return nil
}

func (cp *CommandPuller) readGobCommands(urw *NetworkRWer) ([]common.Command, error) {
	// Try decoding immediately first
	decoder := gob.NewDecoder(urw)
	var commands []common.Command
	if err := decoder.Decode(&commands); err != nil {
		return nil, fmt.Errorf("failed to decode commands: %w", err)
	}
	return commands, nil
}

func (cp *CommandPuller) sendResults(urw *NetworkRWer, results []common.Result) error {
	req := &common.Request{
		AgentID: cp.cfg.AgentID,
		Groups:  cp.cfg.Groups,
		Type:    common.SendResults,
		Results: results,
	}
	return cp.sendGobRequest(urw, req)
}

func (cp *CommandPuller) processCommands(commands []common.Command) {
	commandChan := cp.executer.GetCommandChannel()
	outputChan := cp.executer.GetOutputChannel()

	for _, cmd := range commands {
		slog.Info("Sending command to executer", "command", cmd)
		select {
		case commandChan <- cmd:
			slog.Info("Command sent to executer", "command", cmd)
		case <-cp.ctx.Done():
			return
		}

		// Wait for result with timeout
		select {
		case result := <-outputChan:
			conn, err := cp.connect()
			if err != nil {
				slog.Error("Error connecting to send results", "error", err)
				continue
			}

			// Create NetworkRWer
			urw := &NetworkRWer{
				conn:       conn,
				resultChan: cp.resultChan,
				ring:       cp.ring,
				useTCP:     cp.cfg.UseTCPNetwork,
			}

			if err := cp.sendResults(urw, []common.Result{result}); err != nil {
				slog.Error("Error sending results", "error", err)
			}

			cp.close(conn)
		case <-time.After(time.Second):
			slog.Info("No immediate result for command", "command", cmd)
		case <-cp.ctx.Done():
			return
		}
	}
}

// connect establishes a connection to the server
func (cp *CommandPuller) connect() (interface{}, error) {
	slog.Info("Connecting to server", "host", cp.cfg.Server.Host, "port", cp.cfg.Server.Port)

	if cp.cfg.UseTCPNetwork {
		// Use standard TCP connection
		address := fmt.Sprintf("%s:%d", cp.cfg.Server.Host, cp.cfg.Server.Port)
		conn, err := net.DialTimeout("tcp", address, 10*time.Second)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to server %s: %w", address, err)
		}
		slog.Info("Connected to server via TCP", "address", address)
		return conn, nil
	} else {
		// Use io_uring connection (original behavior)
		sockfd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
		if err != nil {
			return -1, err
		}

		ips, err := net.LookupIP(cp.cfg.Server.Host)
		if err != nil {
			return -1, fmt.Errorf("cannot lookup IP address: %s", cp.cfg.Server.Host)
		}
		slog.Info("NSLookup", "ips", ips)

		// Find the first IPv4 address
		var ip4 net.IP
		for _, ip := range ips {
			if ip4 = ip.To4(); ip4 != nil {
				break
			}
		}
		if ip4 == nil {
			return -1, fmt.Errorf("no IPv4 address found for: %s", cp.cfg.Server.Host)
		}
		slog.Info("IP address", "ip", ip4)

		request, err := iouring.Connect(sockfd, &syscall.SockaddrInet4{
			Port: cp.cfg.Server.Port,
			Addr: func() [4]byte {
				var addr [4]byte
				copy(addr[:], ip4)
				return addr
			}(),
		})
		if err != nil {
			slog.Error("Error connecting to server", "error", err)
			syscall.Close(sockfd)
			return -1, err
		}

		if _, err := cp.ring.SubmitRequest(request, cp.resultChan); err != nil {
			slog.Error("Error submitting request to ring", "error", err)
			syscall.Close(sockfd)
			return -1, err
		}

		result := <-cp.resultChan
		if result.Err() != nil {
			slog.Error("Error getting result from ring", "error", result.Err())
			syscall.Close(sockfd)
			return -1, result.Err()
		}

		slog.Info("Connected to server via io_uring", "sockfd", sockfd)
		return sockfd, nil
	}
}

type NetworkRWer struct {
	conn       interface{} // Can be net.Conn or int (file descriptor)
	resultChan chan iouring.Result
	ring       *iouring.IOURing
	useTCP     bool
}

var _ io.Reader = (*NetworkRWer)(nil)
var _ io.Writer = (*NetworkRWer)(nil)

func (nr *NetworkRWer) Read(buf []byte) (int, error) {
	if nr.useTCP {
		// Use standard TCP Read
		conn := nr.conn.(net.Conn)
		return conn.Read(buf)
	} else {
		// Use io_uring Read (original behavior)
		fd := nr.conn.(int)
		request := iouring.Read(fd, buf)
		if _, err := nr.ring.SubmitRequest(request, nr.resultChan); err != nil {
			return -1, err
		}

		result := <-nr.resultChan
		if result.Err() != nil {
			return -1, result.Err()
		}

		n := result.ReturnValue0().(int)
		readBuf, _ := result.GetRequestBuffer()
		// Copy the data into the provided buffer
		copy(buf[:n], readBuf[:n])

		return n, nil
	}
}

func (nr *NetworkRWer) Write(buf []byte) (int, error) {
	if nr.useTCP {
		// Use standard TCP Write
		conn := nr.conn.(net.Conn)
		n, err := conn.Write(buf)
		slog.Info("Wrote to TCP connection", "n", n)
		return n, err
	} else {
		// Use io_uring Write (original behavior)
		fd := nr.conn.(int)
		request := iouring.Write(fd, buf)
		if _, err := nr.ring.SubmitRequest(request, nr.resultChan); err != nil {
			return -1, err
		}

		result := <-nr.resultChan
		if result.Err() != nil {
			return -1, result.Err()
		}

		n := result.ReturnValue0().(int)
		slog.Info("Wrote to file descriptor", "fd", fd, "n", n)

		return n, nil
	}
}

func (cp *CommandPuller) close(conn interface{}) error {
	if cp.cfg.UseTCPNetwork {
		// Use standard TCP Close
		tcpConn := conn.(net.Conn)
		err := tcpConn.Close()
		if err == nil {
			slog.Info("Closed TCP connection")
		}
		return err
	} else {
		// Use io_uring Close (original behavior)
		fd := conn.(int)
		request := iouring.Close(fd)
		if _, err := cp.ring.SubmitRequest(request, cp.resultChan); err != nil {
			return err
		}

		result := <-cp.resultChan
		if result.Err() != nil {
			return result.Err()
		}

		slog.Info("Closed file descriptor", "fd", fd)
		return nil
	}
}

func (cp *CommandPuller) Close() {
	cp.closeOnce.Do(func() {
		slog.Info("Closing CommandPuller")
		cp.cancelFunc()

		// Add a small delay to allow pending operations to complete
		time.Sleep(50 * time.Millisecond)

		if cp.ring != nil {
			_ = cp.ring.Close()
		}

		close(cp.resultChan)
		slog.Info("CommandPuller closed")
	})
}
