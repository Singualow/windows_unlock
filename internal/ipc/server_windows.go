//go:build windows

package ipc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
)

type ControlHandler func(context.Context, ControlRequest) ControlResponse
type AuthHandler func(context.Context, string) AuthResponse

type Servers struct {
	control net.Listener
	auth    net.Listener
	wg      sync.WaitGroup
}

func Listen(targetSID string, controlHandler ControlHandler, authHandler AuthHandler) (*Servers, error) {
	if targetSID == "" {
		return nil, errors.New("target SID is required for control pipe ACL")
	}
	controlACL := fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;%s)", targetSID)
	control, err := winio.ListenPipe(ControlPipe, &winio.PipeConfig{SecurityDescriptor: controlACL, MessageMode: true, InputBufferSize: 65536, OutputBufferSize: 65536})
	if err != nil {
		return nil, err
	}
	auth, err := winio.ListenPipe(AuthPipe, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)", MessageMode: true, InputBufferSize: 8192, OutputBufferSize: 8192})
	if err != nil {
		control.Close()
		return nil, err
	}
	s := &Servers{control: control, auth: auth}
	s.wg.Add(2)
	go s.serveControl(controlHandler)
	go s.serveAuth(authHandler)
	return s, nil
}

func (s *Servers) Close() error {
	var result error
	if s.control != nil {
		result = s.control.Close()
	}
	if s.auth != nil {
		result = errors.Join(result, s.auth.Close())
	}
	s.wg.Wait()
	return result
}

func (s *Servers) serveControl(handler ControlHandler) {
	defer s.wg.Done()
	for {
		conn, err := s.control.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			var request ControlRequest
			if err := ReadJSON(conn, &request); err != nil {
				_ = WriteJSON(conn, ControlResponse{OK: false, Error: "invalid request"})
				return
			}
			if request.Version != 1 {
				_ = WriteJSON(conn, ControlResponse{OK: false, Error: "unsupported IPC version"})
				return
			}
			_ = WriteJSON(conn, handler(context.Background(), request))
		}()
	}
}

func (s *Servers) serveAuth(handler AuthHandler) {
	defer s.wg.Done()
	for {
		conn, err := s.auth.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			command, err := bufio.NewReader(io.LimitReader(conn, 64)).ReadString('\n')
			if err != nil {
				_ = WriteAuthResponse(conn, AuthResponse{Status: AuthError})
				return
			}
			response := handler(context.Background(), strings.TrimSpace(strings.ToUpper(command)))
			_ = WriteAuthResponse(conn, response)
		}()
	}
}

func CallControl(ctx context.Context, request ControlRequest) (ControlResponse, error) {
	conn, err := winio.DialPipeContext(ctx, ControlPipe)
	if err != nil {
		return ControlResponse{}, err
	}
	defer conn.Close()
	if err := WriteJSON(conn, request); err != nil {
		return ControlResponse{}, err
	}
	var response ControlResponse
	if err := ReadJSON(conn, &response); err != nil {
		return ControlResponse{}, err
	}
	return response, nil
}
