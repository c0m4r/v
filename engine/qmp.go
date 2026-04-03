package engine

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// qmpClient handles communication with QEMU's Machine Protocol socket.
type qmpClient struct {
	conn net.Conn
}

type qmpResponse struct {
	Return json.RawMessage `json:"return,omitempty"`
	Error  *qmpError       `json:"error,omitempty"`
}

type qmpError struct {
	Class string `json:"class"`
	Desc  string `json:"desc"`
}

// qmpConnect opens a QMP socket and performs the capabilities handshake.
func qmpConnect(socketPath string) (*qmpClient, error) {
	conn, err := net.DialTimeout("unix", socketPath, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to QMP socket: %w", err)
	}

	c := &qmpClient{conn: conn}

	// Read the greeting
	if _, err := c.readResponse(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read QMP greeting: %w", err)
	}

	// Negotiate capabilities
	if err := c.execute("qmp_capabilities", nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("QMP capabilities: %w", err)
	}

	return c, nil
}

func (c *qmpClient) close() {
	_ = c.conn.Close()
}

// execute sends a QMP command and waits for the response.
func (c *qmpClient) execute(command string, args any) error {
	msg := map[string]any{"execute": command}
	if args != nil {
		msg["arguments"] = args
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal QMP command: %w", err)
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write QMP command: %w", err)
	}

	resp, err := c.readResponse()
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("QMP error: %s: %s", resp.Error.Class, resp.Error.Desc)
	}
	return nil
}

func (c *qmpClient) readResponse() (*qmpResponse, error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	buf := make([]byte, 4096)
	var data []byte
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("read QMP response: %w", err)
		}
		data = append(data, buf[:n]...)
		// QMP sends newline-delimited JSON
		for i, b := range data {
			if b == '\n' {
				var resp qmpResponse
				if err := json.Unmarshal(data[:i], &resp); err == nil {
					return &resp, nil
				}
				// Could be an event, skip and keep reading
				data = data[i+1:]
				break
			}
		}
		if len(data) > 65536 {
			return nil, fmt.Errorf("QMP response too large")
		}
	}
}
