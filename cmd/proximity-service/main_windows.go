//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"
	"unsafe"

	"github.com/singu/proximity-unlock/internal/authorize"
	"github.com/singu/proximity-unlock/internal/ble"
	"github.com/singu/proximity-unlock/internal/config"
	"github.com/singu/proximity-unlock/internal/coordinator"
	"github.com/singu/proximity-unlock/internal/ipc"
	"github.com/singu/proximity-unlock/internal/secret"
	"github.com/singu/proximity-unlock/internal/windowskey"
	"github.com/singu/proximity-unlock/internal/winsession"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

const (
	serviceName           = "ProximityUnlockSvc"
	pcKeyName             = "ProximityUnlock.PCIdentity.v1"
	pbtAPMResumeAutomatic = 0x0012
	pbtAPMResumeSuspend   = 0x0007
)

type serviceHandler struct{}

func main() {
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatal(err)
	}
	if !isService || (len(os.Args) > 1 && os.Args[1] == "--console") {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		if err := runRuntime(ctx, nil); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := svc.Run(serviceName, serviceHandler{}); err != nil {
		log.Fatal(err)
	}
}

func (serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	logger, _ := eventlog.Open(serviceName)
	if logger != nil {
		defer logger.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan error, 1)
	var coord *coordinator.Coordinator
	go func() {
		var err error
		coord, err = startRuntime(ctx, func(entry coordinator.AuthenticationLogEntry) {
			if logger == nil {
				return
			}
			message := fmt.Sprintf("authentication [%s]: %s", entry.Code, entry.Message)
			if entry.Warning {
				_ = logger.Warning(2, message)
				return
			}
			_ = logger.Info(3, message)
		})
		ready <- err
	}()
	if err := <-ready; err != nil {
		if logger != nil {
			_ = logger.Error(1, "service startup failed: "+err.Error())
		}
		return true, 1
	}
	statuses <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptSessionChange | svc.AcceptPowerEvent}
	for request := range requests {
		switch request.Cmd {
		case svc.Stop, svc.Shutdown:
			statuses <- svc.Status{State: svc.StopPending}
			cancel()
			return false, 0
		case svc.Interrogate:
			statuses <- request.CurrentStatus
		case svc.SessionChange:
			if request.EventData == 0 {
				continue
			}
			notification := (*windows.WTSSESSION_NOTIFICATION)(unsafe.Pointer(request.EventData))
			switch request.EventType {
			case windows.WTS_SESSION_LOGON, windows.WTS_SESSION_UNLOCK:
				if winsession.IsActiveConsole(notification.SessionID) {
					coord.MarkUnlocked(notification.SessionID)
				}
			case windows.WTS_SESSION_LOCK:
				if winsession.IsActiveConsole(notification.SessionID) {
					coord.MarkLocked(notification.SessionID, time.Now())
				}
			case windows.WTS_SESSION_LOGOFF:
				coord.MarkLoggedOff(notification.SessionID)
			}
		case svc.PowerEvent:
			if request.EventType == pbtAPMResumeAutomatic || request.EventType == pbtAPMResumeSuspend {
				coord.MarkResume(time.Now())
			}
		}
	}
	return false, 0
}

func runRuntime(ctx context.Context, result chan<- *coordinator.Coordinator) error {
	coord, err := startRuntime(ctx, func(entry coordinator.AuthenticationLogEntry) {
		log.Printf("authentication [%s]: %s", entry.Code, entry.Message)
	})
	if err != nil {
		return err
	}
	if result != nil {
		result <- coord
	}
	<-ctx.Done()
	return coord.Close()
}

func startRuntime(ctx context.Context, authLogger func(coordinator.AuthenticationLogEntry)) (*coordinator.Coordinator, error) {
	path := config.ProgramDataPath()
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if cfg.TargetSID == "" || cfg.PCID == "" {
		return nil, errors.New("service is not enrolled; run proximityctl initialize first")
	}
	key, err := windowskey.OpenOrCreate(pcKeyName, true)
	if err != nil {
		return nil, fmt.Errorf("open PC identity key: %w", err)
	}
	transport := ble.New()
	authorizer := authorize.New(ipc.SignalCredentialReady)
	if !cfg.CredentialValid {
		authorizer.Disable()
	}
	coord := coordinator.New(path, cfg, secret.NewLSAStore(), key, transport, authorizer)
	coord.SetSessionValidator(winsession.IsActiveConsole)
	coord.SetAuthenticationLogger(authLogger)
	servers, err := ipc.Listen(cfg.TargetSID, coord.HandleControl, coord.HandleAuth)
	if err != nil {
		key.Close()
		return nil, fmt.Errorf("start IPC: %w", err)
	}
	if err := coord.Start(ctx); err != nil {
		servers.Close()
		key.Close()
		return nil, fmt.Errorf("start BLE: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = servers.Close()
		_ = coord.Close()
		_ = key.Close()
	}()
	return coord, nil
}
