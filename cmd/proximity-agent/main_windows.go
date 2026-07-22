//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/getlantern/systray"
	"github.com/singu/proximity-unlock/internal/config"
	"github.com/singu/proximity-unlock/internal/coordinator"
	"github.com/singu/proximity-unlock/internal/ipc"
	"github.com/singu/proximity-unlock/internal/wincred"
	"github.com/singu/proximity-unlock/internal/winsession"
	"github.com/singu/proximity-unlock/internal/winshell"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/sys/windows/registry"
)

var cancel context.CancelFunc
var calibrationBusy atomic.Bool

const credentialProviderRegistryPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\Credential Providers\{C81FCF2E-B9D0-4EAF-8D35-55F750D2561B}`

func main() { systray.Run(onReady, onExit) }

func onReady() {
	systray.SetIcon(lockIcon())
	systray.SetTitle("Proximity Unlock")
	systray.SetTooltip("蓝牙自动解锁")
	statusItem := systray.AddMenuItem("正在连接服务…", "当前状态")
	statusItem.Disable()
	systray.AddSeparator()
	credentialProviderItem := systray.AddMenuItemCheckbox(
		"启用 Windows 锁屏自动解锁",
		"注册或注销 Credential Provider；PIN、密码和 Windows Hello 始终保留",
		credentialProviderRegistered(),
	)
	immediateUnlockItem := systray.AddMenuItemCheckbox(
		"锁屏后立即解锁（降低安全性）",
		"取消 Win+L 后必须离开 10 秒的保护；仍要求近距离广播和手机签名",
		false,
	)
	systray.AddSeparator()
	strictItem := systray.AddMenuItemCheckbox("安全模式（手机须解锁）", "严格密钥", true)
	convenienceItem := systray.AddMenuItemCheckbox("便捷模式（锁屏可用）", "便捷密钥", false)
	autoLockItem := systray.AddMenuItemCheckbox("自动锁定", "手机离开后锁定电脑", true)
	pauseItem := systray.AddMenuItem("暂停 5 分钟", "临时关闭自动锁定与解锁")
	resumeItem := systray.AddMenuItem("立即恢复", "取消暂停")
	systray.AddSeparator()
	pairItem := systray.AddMenuItem("配对手机…", "生成两分钟配对二维码")
	revokeItem := systray.AddMenuItem("撤销已配对手机…", "清除电脑端的手机公钥和 presence key")
	calibrateItem := systray.AddMenuItem("距离校准…", "采集近距离和远距离 RSSI")
	passwordItem := systray.AddMenuItem("更新 Windows 密码…", "需要管理员权限")
	systray.AddSeparator()
	uninstallItem := systray.AddMenuItem("卸载软件…", "删除服务、托盘、配对记录和私密数据")
	quitItem := systray.AddMenuItem("退出托盘", "服务继续以失败关闭方式运行")

	ctx, cancelFunc := context.WithCancel(context.Background())
	cancel = cancelFunc
	sessionID, sessionErr := winsession.CurrentID()
	if activeID, ok := winsession.ActiveConsoleID(); sessionErr != nil || !ok || sessionID != activeID {
		statusItem.SetTitle("RDP/非控制台会话不支持自动解锁")
		return
	}
	cfg, cfgErr := config.Load(config.ProgramDataPath())
	currentSID, sidErr := wincred.CurrentSID()
	if cfgErr != nil || sidErr != nil || cfg.TargetSID == "" || currentSID != cfg.TargetSID {
		statusItem.SetTitle("当前用户不是已登记账户；代理未启动")
		return
	}
	serviceUnavailableSince := time.Time{}
	autoLockEnabled := cfg.AutoLock
	immediateUnlockEnabled := cfg.ImmediateUnlock
	pausedUntil := time.Time{}
	if cfg.PausedUntil != nil {
		pausedUntil = *cfg.PausedUntil
	}
	if sessionID != 0 {
		payload, _ := json.Marshal(map[string]any{"session_id": sessionID})
		_, _ = call(ctx, "session_active", payload)
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				response, err := call(ctx, "status", nil)
				if err != nil || !response.OK {
					statusItem.SetTitle("服务不可用（20 秒后安全锁定）")
					now := time.Now()
					if serviceUnavailableSince.IsZero() {
						serviceUnavailableSince = now
					}
					if autoLockEnabled && !pausedUntil.After(now) && now.Sub(serviceUnavailableSince) >= 20*time.Second {
						_ = winsession.Lock()
					}
					continue
				}
				serviceUnavailableSince = time.Time{}
				encoded, _ := json.Marshal(response.Payload)
				var status coordinator.Status
				if json.Unmarshal(encoded, &status) != nil {
					continue
				}
				if !status.SessionActive && sessionID != 0 {
					payload, _ := json.Marshal(map[string]any{"session_id": sessionID})
					_, _ = call(ctx, "session_active", payload)
					continue
				}
				statusItem.SetTitle(formatStatus(status))
				autoLockEnabled = status.AutoLock
				pausedUntil = status.PausedUntil
				if status.Mode == "convenience" {
					convenienceItem.Check()
					strictItem.Uncheck()
				} else {
					strictItem.Check()
					convenienceItem.Uncheck()
				}
				if status.AutoLock {
					autoLockItem.Check()
				} else {
					autoLockItem.Uncheck()
				}
				immediateUnlockEnabled = status.ImmediateUnlock
				if status.ImmediateUnlock {
					immediateUnlockItem.Check()
				} else {
					immediateUnlockItem.Uncheck()
				}
				if credentialProviderRegistered() {
					credentialProviderItem.Check()
				} else {
					credentialProviderItem.Uncheck()
				}
				if status.ShouldLock {
					_ = winsession.Lock()
				}
			case <-strictItem.ClickedCh:
				_, _ = callPayload(ctx, "set_mode", map[string]any{"mode": "strict"})
			case <-convenienceItem.ClickedCh:
				_, _ = callPayload(ctx, "set_mode", map[string]any{"mode": "convenience"})
			case <-autoLockItem.ClickedCh:
				_, _ = callPayload(ctx, "set_auto_lock", map[string]any{"enabled": !autoLockEnabled})
			case <-immediateUnlockItem.ClickedCh:
				enabled := !immediateUnlockEnabled
				if enabled && !winshell.Confirm(
					"开启锁屏后立即解锁",
					"开启后，只要手机仍在附近并能完成签名，主动按 Win+L 也可能在几秒内自动解锁。\n\n这会降低手动锁屏的保护强度。确定开启吗？",
				) {
					continue
				}
				_, _ = callPayload(ctx, "set_immediate_unlock", map[string]any{"enabled": enabled})
			case <-credentialProviderItem.ClickedCh:
				operation := "enable-credential-provider"
				if credentialProviderRegistered() {
					operation = "disable-credential-provider"
				}
				_ = launchSibling("runas", "setup.exe", operation)
			case <-pauseItem.ClickedCh:
				_, _ = callPayload(ctx, "pause", map[string]any{"seconds": 300})
			case <-resumeItem.ClickedCh:
				_, _ = callPayload(ctx, "pause", map[string]any{"seconds": 0})
			case <-pairItem.ClickedCh:
				go func() {
					if err := startPairing(ctx); err != nil {
						winshell.Error("Proximity Unlock", "无法开始配对：\n"+err.Error())
					}
				}()
			case <-revokeItem.ClickedCh:
				if winshell.Confirm("撤销已配对手机", "确定清除电脑端的手机配对记录吗？撤销后必须重新配对才能解锁。") {
					response, err := call(ctx, "revoke", nil)
					if err != nil {
						winshell.Error("Proximity Unlock", err.Error())
					} else if !response.OK {
						winshell.Error("Proximity Unlock", response.Error)
					}
				}
			case <-calibrateItem.ClickedCh:
				go runCalibration(ctx)
			case <-passwordItem.ClickedCh:
				_ = launchSibling("runas", "setup.exe", "set-password")
			case <-uninstallItem.ClickedCh:
				if winshell.Confirm("卸载 Proximity Unlock", "确定卸载服务、托盘、配对记录、LSA 密码和电脑身份密钥吗？\n\nWindows PIN、密码和 Windows Hello 不受影响。") {
					if launchSibling("runas", "uninstall.exe", "uninstall") == nil {
						systray.Quit()
					}
				}
			case <-quitItem.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	if cancel != nil {
		cancel()
	}
}

func formatStatus(status coordinator.Status) string {
	if !status.Paired {
		return "未配对手机"
	}
	if !status.CredentialValid {
		return "Windows 密码需要更新"
	}
	if !status.PausedUntil.IsZero() && status.PausedUntil.After(time.Now()) {
		return "已暂停至 " + status.PausedUntil.Format("15:04")
	}
	if status.HasRSSI {
		if status.ImmediateUnlock {
			return fmt.Sprintf("%s · 即时解锁 · RSSI %d dBm", status.Mode, status.MedianRSSI)
		}
		return fmt.Sprintf("%s · 离开后解锁 · RSSI %d dBm", status.Mode, status.MedianRSSI)
	}
	return status.Mode + " · 等待手机"
}

func callPayload(ctx context.Context, op string, payload any) (ipc.ControlResponse, error) {
	raw, _ := json.Marshal(payload)
	return call(ctx, op, raw)
}

func call(parent context.Context, op string, payload json.RawMessage) (ipc.ControlResponse, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	return ipc.CallControl(ctx, ipc.ControlRequest{Version: 1, Op: op, Payload: payload})
}

func startPairing(ctx context.Context) error {
	response, err := call(ctx, "pair_start", nil)
	if err != nil || !response.OK {
		if err != nil {
			return err
		}
		return errors.New(response.Error)
	}
	encoded, _ := json.Marshal(response.Payload)
	var payload struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(encoded, &payload) != nil || payload.URI == "" {
		return errors.New("服务返回了无效的配对信息")
	}
	file, err := os.CreateTemp("", "ProximityUnlock-pair-*.png")
	if err != nil {
		return err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	if err := qrcode.WriteFile(payload.URI, qrcode.Medium, 512, path); err != nil {
		_ = os.Remove(path)
		return err
	}
	if err := winshell.Open(path); err != nil {
		_ = os.Remove(path)
		return err
	}
	time.AfterFunc(125*time.Second, func() { _ = os.Remove(path) })
	return nil
}

func runCalibration(ctx context.Context) {
	if !calibrationBusy.CompareAndSwap(false, true) {
		winshell.Info("Proximity Unlock", "距离校准已经在进行中。")
		return
	}
	defer calibrationBusy.Store(false)
	if !winshell.Confirm("距离校准", "把手机放在希望能够解锁电脑的最远位置，然后点击“确定”。程序将采集 8 秒信号。\n\nRSSI 只能改善便利性，不能防止蓝牙中继攻击。") {
		return
	}
	nearRSSI, err := collectRSSI(ctx, 8*time.Second)
	if err != nil {
		winshell.Error("距离校准", err.Error())
		return
	}
	if !winshell.Confirm("距离校准", fmt.Sprintf("近距离样本：%d dBm。\n\n现在把手机移到应触发自动锁定的位置，等待稳定后点击“确定”。程序将再采集 8 秒。", nearRSSI)) {
		return
	}
	farRSSI, err := collectRSSI(ctx, 8*time.Second)
	if err != nil {
		winshell.Error("距离校准", err.Error())
		return
	}
	if nearRSSI-farRSSI < 8 {
		winshell.Error("距离校准", fmt.Sprintf("近、远样本只相差 %d dB。请把手机移得更远后重试。", nearRSSI-farRSSI))
		return
	}
	unlock := clamp(nearRSSI-2, -90, -35)
	lock := clamp(farRSSI+2, -110, unlock-8)
	if !winshell.Confirm("距离校准", fmt.Sprintf("建议阈值：\n解锁 %d dBm\n锁定 %d dBm\n\n点击“确定”应用。", unlock, lock)) {
		return
	}
	response, err := callPayload(ctx, "set_thresholds", map[string]any{"unlock_rssi": unlock, "lock_rssi": lock})
	if err != nil || !response.OK {
		if err != nil {
			winshell.Error("距离校准", err.Error())
		} else {
			winshell.Error("距离校准", response.Error)
		}
		return
	}
	winshell.Info("距离校准", "新阈值已应用。")
}

func collectRSSI(ctx context.Context, duration time.Duration) (int, error) {
	deadline := time.Now().Add(duration)
	values := make([]int, 0, int(duration/(500*time.Millisecond)))
	for time.Now().Before(deadline) {
		response, err := call(ctx, "status", nil)
		if err != nil {
			return 0, err
		}
		encoded, _ := json.Marshal(response.Payload)
		var status coordinator.Status
		if response.OK && json.Unmarshal(encoded, &status) == nil && status.HasRSSI {
			values = append(values, status.MedianRSSI)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(values) < 5 {
		return 0, errors.New("有效蓝牙样本不足；请保持手机蓝牙钥匙运行后重试")
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

func launchControl(verb, argument string) error {
	return launchSibling(verb, "proximityctl.exe", argument)
}

func launchSibling(verb, name, argument string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(executable), name)
	return winshell.Execute(verb, path, argument)
}

func credentialProviderRegistered() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, credentialProviderRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	key.Close()
	return true
}

// lockIcon creates a self-contained 16x16 32-bit ICO, avoiding a mutable
// external icon beside the security-sensitive agent binary.
func lockIcon() []byte {
	const width, height = 16, 16
	const bitmapBytes = width * height * 4
	const maskBytes = height * 4
	const imageBytes = 40 + bitmapBytes + maskBytes
	buffer := new(bytes.Buffer)
	write := func(value any) { _ = binary.Write(buffer, binary.LittleEndian, value) }
	write(uint16(0))
	write(uint16(1))
	write(uint16(1))
	buffer.WriteByte(width)
	buffer.WriteByte(height)
	buffer.WriteByte(0)
	buffer.WriteByte(0)
	write(uint16(1))
	write(uint16(32))
	write(uint32(imageBytes))
	write(uint32(22))
	write(uint32(40))
	write(int32(width))
	write(int32(height * 2))
	write(uint16(1))
	write(uint16(32))
	write(uint32(0))
	write(uint32(bitmapBytes))
	write(int32(0))
	write(int32(0))
	write(uint32(0))
	write(uint32(0))
	for y := height - 1; y >= 0; y-- {
		for x := 0; x < width; x++ {
			blue, green, red, alpha := byte(235), byte(99), byte(37), byte(255)
			body := x >= 4 && x <= 11 && y >= 7 && y <= 13
			shackle := y >= 3 && y <= 8 && ((x == 5 || x == 10) || (y == 3 && x >= 6 && x <= 9))
			if body || shackle {
				blue, green, red = 255, 255, 255
			}
			buffer.Write([]byte{blue, green, red, alpha})
		}
	}
	buffer.Write(make([]byte, maskBytes))
	return buffer.Bytes()
}
