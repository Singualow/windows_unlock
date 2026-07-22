//go:build windows

package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/singu/proximity-unlock/internal/config"
	"github.com/singu/proximity-unlock/internal/coordinator"
	"github.com/singu/proximity-unlock/internal/ipc"
	"github.com/singu/proximity-unlock/internal/protocol"
	"github.com/singu/proximity-unlock/internal/secret"
	"github.com/singu/proximity-unlock/internal/wincred"
	"github.com/singu/proximity-unlock/internal/windowskey"
	"github.com/singu/proximity-unlock/internal/winshell"
	qrcode "github.com/skip2/go-qrcode"
)

const pcKeyName = "ProximityUnlock.PCIdentity.v1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "initialize":
		err = initialize()
	case "self-test":
		err = selfTest()
	case "set-password":
		err = setPassword()
	case "status":
		err = control("status", nil)
	case "pair":
		err = pair()
	case "calibrate":
		err = calibrate()
	case "strict", "convenience":
		err = control("set_mode", map[string]any{"mode": os.Args[1]})
	case "pause":
		seconds := 300
		if len(os.Args) > 2 {
			_, err = fmt.Sscanf(os.Args[2], "%d", &seconds)
		}
		if err == nil {
			err = control("pause", map[string]any{"seconds": seconds})
		}
	case "resume":
		err = control("pause", map[string]any{"seconds": 0})
	case "revoke":
		err = control("revoke", nil)
	default:
		usage()
		err = errors.New("unknown command")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "proximityctl initialize|self-test|set-password|status|pair|calibrate|strict|convenience|pause [seconds]|resume|revoke")
}

func calibrate() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("距离校准不会把 RSSI 当作安全距离证明；它只调整便利性阈值。")
	fmt.Println("把手机放在你希望电脑能够解锁的最远位置，然后按 Enter。")
	_, _ = reader.ReadString('\n')
	nearRSSI, err := collectRSSI(8 * time.Second)
	if err != nil {
		return err
	}
	fmt.Printf("近距离样本中位数：%d dBm\n", nearRSSI)
	fmt.Println("现在把手机移到应触发自动锁定的位置，等待几秒后按 Enter。")
	_, _ = reader.ReadString('\n')
	farRSSI, err := collectRSSI(8 * time.Second)
	if err != nil {
		return err
	}
	if nearRSSI-farRSSI < 8 {
		return fmt.Errorf("near/far samples differ by only %d dB; move the phone farther away and retry", nearRSSI-farRSSI)
	}
	unlock := clamp(nearRSSI-2, -90, -35)
	lock := clamp(farRSSI+2, -110, unlock-8)
	fmt.Printf("建议阈值：解锁 %d dBm，锁定 %d dBm。应用？[y/N] ", unlock, lock)
	answer, _ := reader.ReadString('\n')
	if answer != "y\n" && answer != "Y\n" && answer != "y\r\n" && answer != "Y\r\n" {
		return errors.New("calibration cancelled")
	}
	return control("set_thresholds", map[string]any{"unlock_rssi": unlock, "lock_rssi": lock})
}

func collectRSSI(duration time.Duration) (int, error) {
	deadline := time.Now().Add(duration)
	values := make([]int, 0, int(duration/(500*time.Millisecond)))
	for time.Now().Before(deadline) {
		response, err := call("status", nil)
		if err != nil {
			return 0, err
		}
		raw, _ := json.Marshal(response.Payload)
		var status coordinator.Status
		if json.Unmarshal(raw, &status) == nil && status.HasRSSI {
			values = append(values, status.MedianRSSI)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(values) < 5 {
		return 0, errors.New("not enough authenticated phone advertisements; keep the phone service running and retry")
	}
	sort.Ints(values)
	return values[len(values)/2], nil
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func initialize() error {
	if !wincred.IsElevated() {
		return errors.New("initialize must run from an elevated Administrator terminal")
	}
	path := config.ProgramDataPath()
	if _, err := os.Stat(path); err == nil {
		return errors.New("configuration already exists; uninstall or back it up before reinitializing")
	}
	sid, err := wincred.CurrentSID()
	if err != nil {
		return err
	}
	key, err := windowskey.OpenOrCreate(pcKeyName, true)
	if err != nil {
		return err
	}
	defer key.Close()
	credential, err := wincred.PromptAndValidate()
	if err != nil {
		return err
	}
	defer credential.Clear()
	if credential.SID != sid {
		return errors.New("the entered password belongs to a different Windows account")
	}
	pcIDBytes, err := protocol.RandomBytes(protocol.DeviceIDSize)
	if err != nil {
		return err
	}
	defer protocol.Zero(pcIDBytes)
	cfg := config.Default()
	cfg.TargetSID = sid
	cfg.CanonicalUser = credential.CanonicalUser
	cfg.PCID = base64.RawURLEncoding.EncodeToString(pcIDBytes)
	cfg.CredentialValid = true
	store := secret.NewLSAStore()
	if err := store.Put(secret.CredentialName(sid), credential.Password); err != nil {
		return fmt.Errorf("store password in LSA: %w", err)
	}
	if err := config.Save(path, cfg); err != nil {
		_ = store.Delete(secret.CredentialName(sid))
		return err
	}
	fmt.Println("Initialized", path)
	fmt.Println("CNG provider:", key.Backend())
	return nil
}

func selfTest() error {
	if !wincred.IsElevated() {
		return errors.New("self-test must run elevated")
	}
	cfg, err := config.Load(config.ProgramDataPath())
	if err != nil {
		return err
	}
	if cfg.TargetSID == "" {
		return errors.New("not initialized")
	}
	key, err := windowskey.OpenOrCreate(pcKeyName, true)
	if err != nil {
		return err
	}
	defer key.Close()
	message := []byte("ProximityUnlock self-test")
	signature, err := key.Sign(message)
	if err != nil {
		return err
	}
	publicKey, err := key.PublicKey()
	if err != nil || !protocol.VerifyP256(publicKey, message, signature[:]) {
		return errors.New("CNG signature self-test failed")
	}
	store := secret.NewLSAStore()
	testName := "L$ProximityUnlock/SelfTest"
	testValue, _ := protocol.RandomBytes(32)
	defer protocol.Zero(testValue)
	if err := store.Put(testName, testValue); err != nil {
		return err
	}
	defer store.Delete(testName)
	got, err := store.Get(testName)
	if err != nil || !protocol.Equal(got, testValue) {
		return errors.New("LSA private data self-test failed")
	}
	protocol.Zero(got)
	digest := sha256.Sum256([]byte(cfg.TargetSID))
	fmt.Printf("OK: config, LSA, CNG (%s), target SID hash %.8x\n", key.Backend(), digest[:4])
	return nil
}

func setPassword() error {
	if !wincred.IsElevated() {
		return errors.New("set-password must run elevated")
	}
	path := config.ProgramDataPath()
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	credential, err := wincred.PromptAndValidate()
	if err != nil {
		return err
	}
	defer credential.Clear()
	if credential.SID != cfg.TargetSID {
		return errors.New("the entered password belongs to a different Windows account")
	}
	if err := secret.NewLSAStore().Put(secret.CredentialName(cfg.TargetSID), credential.Password); err != nil {
		return err
	}
	cfg.CanonicalUser = credential.CanonicalUser
	cfg.CredentialValid = true
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	// The running service may not be installed yet; a failed reload request is
	// harmless because the next service start reads the updated config.
	_ = control("reload", nil)
	return nil
}

func pair() error {
	response, err := call("pair_start", nil)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(response.Payload)
	var payload struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil || payload.URI == "" {
		return errors.New("service returned an invalid pairing URI")
	}
	fmt.Println("Pairing URI (expires in two minutes):")
	fmt.Println(payload.URI)
	qrPath := filepath.Join(os.TempDir(), "ProximityUnlock-pair.png")
	if err := qrcode.WriteFile(payload.URI, qrcode.Medium, 512, qrPath); err != nil {
		return err
	}
	defer os.Remove(qrPath)
	fmt.Println("QR code:", qrPath)
	_ = winshell.Open(qrPath)
	fmt.Println("Scan the QR code or paste the URI into Android. Press Enter after pairing; the QR file will be deleted.")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	for i := 0; i < 50; i++ {
		fmt.Println()
	}
	return nil
}

func control(op string, payload any) error {
	response, err := call(op, payload)
	if err != nil {
		return err
	}
	formatted, _ := json.MarshalIndent(response.Payload, "", "  ")
	if len(formatted) > 0 && string(formatted) != "null" {
		fmt.Println(string(formatted))
	}
	return nil
}

func call(op string, payload any) (ipc.ControlResponse, error) {
	var raw json.RawMessage
	if payload != nil {
		raw, _ = json.Marshal(payload)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := ipc.CallControl(ctx, ipc.ControlRequest{Version: 1, Op: op, Payload: raw})
	if err != nil {
		return response, err
	}
	if !response.OK {
		return response, errors.New(response.Error)
	}
	return response, nil
}
