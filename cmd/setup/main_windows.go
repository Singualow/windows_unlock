//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/singu/proximity-unlock/internal/config"
	"github.com/singu/proximity-unlock/internal/coordinator"
	"github.com/singu/proximity-unlock/internal/ipc"
	"github.com/singu/proximity-unlock/internal/secret"
	"github.com/singu/proximity-unlock/internal/wincred"
	"github.com/singu/proximity-unlock/internal/windowskey"
	"github.com/singu/proximity-unlock/internal/winshell"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName               = "ProximityUnlockSvc"
	pcKeyName                 = "ProximityUnlock.PCIdentity.v1"
	credentialProviderDLLName = "ProximityUnlockCredentialProvider.dll"
)

var files = []string{
	"ProximityUnlockSvc.exe",
	"ProximityUnlock.exe",
	"proximityctl.exe",
	"ProximityUnlockCredentialProvider.dll",
	"setup.exe",
	"uninstall.exe",
}

func main() {
	if !wincred.IsElevated() {
		if strings.EqualFold(filepath.Base(os.Args[0]), "uninstall.exe") && len(os.Args) == 1 {
			if !winshell.Confirm("卸载 Proximity Unlock", "确定卸载服务、托盘、配对记录、LSA 密码和电脑身份密钥吗？\n\nWindows PIN、密码和 Windows Hello 不受影响。") {
				return
			}
			executable, err := os.Executable()
			if err != nil {
				fatal(err)
			}
			if err := winshell.Execute("runas", executable, "uninstall"); err != nil {
				fatal(err)
			}
			return
		}
		fatal(errors.New("setup must run elevated as Administrator"))
	}
	operation := ""
	if len(os.Args) >= 2 {
		operation = os.Args[1]
	} else if strings.EqualFold(filepath.Base(os.Args[0]), "uninstall.exe") {
		operation = "uninstall"
	} else {
		fatal(errors.New("请从 Proximity Unlock 托盘或安装程序执行此操作"))
	}
	var err error
	switch operation {
	case "install":
		err = installStageOne()
	case "repair-management":
		err = repairManagementBinaries()
	case "upgrade-ui":
		err = upgradeUI()
	case "enable-credential-provider":
		err = enableCredentialProvider()
	case "disable-credential-provider":
		err = unregisterCredentialProvider()
	case "set-password":
		err = updatePassword()
	case "recover":
		err = recoverOnly()
	case "uninstall":
		err = uninstall()
	default:
		err = errors.New("unknown setup operation")
	}
	if err != nil {
		fatal(err)
	}
	switch operation {
	case "enable-credential-provider":
		winshell.Info("Proximity Unlock", "Windows 锁屏自动解锁组件已启用。PIN、密码和 Windows Hello 仍然保留。")
	case "disable-credential-provider":
		winshell.Info("Proximity Unlock", "Windows 锁屏自动解锁组件已停用。")
	case "set-password":
		winshell.Info("Proximity Unlock", "Windows 密码已安全更新。")
	case "repair-management":
		winshell.Info("Proximity Unlock", "服务诊断与安装卸载组件已更新。")
	case "upgrade-ui":
		// The non-elevated installer process starts the new interface after UAC exits.
	case "uninstall":
		winshell.Info("Proximity Unlock", "软件已卸载。仍被系统占用的文件将在重启后删除。")
	}
}

func repairManagementBinaries() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	sourceDir := filepath.Dir(executable)
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer service.Close()
	_, stopErr := service.Control(svc.Stop)
	if stopErr != nil && !errors.Is(stopErr, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return stopErr
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, queryErr := service.Query()
		if queryErr != nil {
			return queryErr
		}
		if status.State == svc.Stopped {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	status, err := service.Query()
	if err != nil || status.State != svc.Stopped {
		return errors.New("service did not stop for update")
	}
	defer func() { _ = service.Start() }()
	for _, name := range []string{"ProximityUnlockSvc.exe", credentialProviderDLLName, "setup.exe", "uninstall.exe"} {
		from := filepath.Join(sourceDir, name)
		to := filepath.Join(installDir(), name)
		if err := copyFile(from, to); err != nil {
			return fmt.Errorf("update %s: %w", name, err)
		}
	}
	systemDLL := systemCredentialProviderPath()
	if err := copyFile(filepath.Join(sourceDir, credentialProviderDLLName), systemDLL); err != nil {
		return fmt.Errorf("update System32 Credential Provider: %w", err)
	}
	if err := callDLL(systemDLL, "DllRegisterServer"); err != nil {
		return fmt.Errorf("register updated Credential Provider: %w", err)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	winshell.Error("Proximity Unlock", err.Error())
	os.Exit(1)
}

func installDir() string {
	base := os.Getenv("ProgramFiles")
	if base == "" {
		base = `C:\Program Files`
	}
	return filepath.Join(base, "ProximityUnlock")
}

func installStageOne() error {
	cfg, err := config.Load(config.ProgramDataPath())
	if err != nil || cfg.TargetSID == "" || !cfg.CredentialValid {
		return errors.New("run elevated 'proximityctl initialize' successfully before installing the service")
	}
	if _, err := secret.NewLSAStore().Get(secret.CredentialName(cfg.TargetSID)); err != nil {
		return fmt.Errorf("credential secret self-check failed: %w", err)
	}
	source, err := os.Executable()
	if err != nil {
		return err
	}
	sourceDir := filepath.Dir(source)
	destination := installDir()
	if entries, readErr := os.ReadDir(destination); readErr == nil && len(entries) != 0 {
		return errors.New("Proximity Unlock is already installed; uninstall it before reinstalling this personal build")
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	copied := make([]string, 0, len(files))
	for _, name := range files {
		from := filepath.Join(sourceDir, name)
		to := filepath.Join(destination, name)
		if err := copyFile(from, to); err != nil {
			rollbackStageOne(cfg.TargetSID, destination, copied)
			return err
		}
		copied = append(copied, to)
	}
	if err := installService(filepath.Join(destination, "ProximityUnlockSvc.exe")); err != nil {
		rollbackStageOne(cfg.TargetSID, destination, copied)
		return err
	}
	if err := setAgentRun(cfg.TargetSID, filepath.Join(destination, "ProximityUnlock.exe")); err != nil {
		rollbackStageOne(cfg.TargetSID, destination, copied)
		return err
	}
	fmt.Println("Stage 1 installed. Pair the Android phone, verify 'proximityctl status', then run:")
	fmt.Println(`  setup.exe enable-credential-provider`)
	return nil
}

func rollbackStageOne(targetSID, destination string, copied []string) {
	removeAgentRun(targetSID)
	if manager, err := mgr.Connect(); err == nil {
		if service, openErr := manager.OpenService(serviceName); openErr == nil {
			_, _ = service.Control(svc.Stop)
			_ = service.Delete()
			service.Close()
		}
		manager.Disconnect()
	}
	_ = eventlog.Remove(serviceName)
	for _, path := range copied {
		_ = os.Remove(path)
	}
	_ = os.Remove(destination)
}

func copyFile(from, to string) error {
	input, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("open %s: %w", from, err)
	}
	defer input.Close()
	temp := to + ".new"
	output, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err = io.Copy(output, input); err == nil {
		err = output.Sync()
	}
	closeErr := output.Close()
	if err != nil {
		_ = os.Remove(temp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(temp)
		return closeErr
	}
	var renameErr error
	deadline := time.Now().Add(10 * time.Second)
	for {
		renameErr = os.Rename(temp, to)
		if renameErr == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = os.Remove(temp)
			return renameErr
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func installService(binary string) error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if err == nil {
		defer service.Close()
		_, _ = service.Control(svc.Stop)
		current, configErr := service.Config()
		if configErr != nil {
			return configErr
		}
		current.BinaryPathName = binary
		current.DisplayName = "Proximity Unlock Service"
		current.Description = "Authenticated BLE proximity coordinator"
		current.StartType = mgr.StartAutomatic
		if err := service.UpdateConfig(current); err != nil {
			return err
		}
	} else {
		service, err = manager.CreateService(serviceName, binary, mgr.Config{
			DisplayName: "Proximity Unlock Service",
			Description: "Authenticated BLE proximity coordinator",
			StartType:   mgr.StartAutomatic,
		})
		if err != nil {
			return err
		}
		defer service.Close()
		_ = eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	}
	if err := service.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return err
	}
	return nil
}

func setAgentRun(targetSID, binary string) error {
	key, _, err := registry.CreateKey(registry.USERS, targetSID+`\Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue("ProximityUnlock", `"`+binary+`" --background`); err != nil {
		return err
	}
	_ = key.DeleteValue("ProximityUnlockAgent")
	return nil
}

func removeAgentRun(targetSID string) {
	if targetSID == "" {
		return
	}
	if key, err := registry.OpenKey(registry.USERS, targetSID+`\Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE); err == nil {
		_ = key.DeleteValue("ProximityUnlockAgent")
		_ = key.DeleteValue("ProximityUnlock")
		key.Close()
	}
}

func upgradeUI() error {
	cfg, err := config.Load(config.ProgramDataPath())
	if err != nil || cfg.TargetSID == "" {
		return errors.New("无法读取现有安装账户")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	sourceDir := filepath.Dir(executable)
	names := []string{"ProximityUnlockSvc.exe", "ProximityUnlock.exe", "proximityctl.exe", "setup.exe", "uninstall.exe"}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(sourceDir, name)); err != nil {
			return fmt.Errorf("升级包缺少 %s: %w", name, err)
		}
	}
	destination := installDir()
	if _, err := os.Stat(destination); err != nil {
		return errors.New("没有找到现有安装目录")
	}
	if err := stopInterfaceProcesses(); err != nil {
		return err
	}
	serviceWasRunning, err := stopServiceForUpgrade()
	if err != nil {
		return fmt.Errorf("停止蓝牙解锁服务: %w", err)
	}
	type backup struct {
		destination string
		path        string
		existed     bool
	}
	backups := make([]backup, 0, len(names))
	rollback := func() {
		for index := len(backups) - 1; index >= 0; index-- {
			item := backups[index]
			_ = os.Remove(item.destination)
			if item.existed {
				_ = copyFile(item.path, item.destination)
			}
			_ = os.Remove(item.path)
		}
		if serviceWasRunning {
			_ = startInstalledService()
		}
	}
	for _, name := range names {
		to := filepath.Join(destination, name)
		item := backup{destination: to, path: to + ".ui-upgrade-backup"}
		if _, err := os.Stat(to); err == nil {
			item.existed = true
			if err := copyFile(to, item.path); err != nil {
				rollback()
				return fmt.Errorf("备份 %s: %w", name, err)
			}
		}
		backups = append(backups, item)
		if err := copyFile(filepath.Join(sourceDir, name), to); err != nil {
			rollback()
			return fmt.Errorf("更新 %s: %w", name, err)
		}
	}
	if err := setAgentRun(cfg.TargetSID, filepath.Join(destination, "ProximityUnlock.exe")); err != nil {
		rollback()
		return fmt.Errorf("更新登录启动项: %w", err)
	}
	if serviceWasRunning {
		if err := startInstalledService(); err != nil {
			rollback()
			return fmt.Errorf("启动更新后的蓝牙解锁服务: %w", err)
		}
	}
	removeOrSchedule(filepath.Join(destination, "ProximityUnlockAgent.exe"))
	for _, item := range backups {
		_ = os.Remove(item.path)
	}
	return nil
}

func stopServiceForUpgrade() (bool, error) {
	manager, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if err != nil {
		return false, err
	}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return false, err
	}
	if status.State == svc.Stopped {
		return false, nil
	}
	if status.State != svc.StopPending {
		if _, err := service.Control(svc.Stop); err != nil {
			return true, err
		}
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		status, err = service.Query()
		if err != nil {
			return true, err
		}
		if status.State == svc.Stopped {
			return true, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return true, errors.New("等待服务停止超时")
}

func startInstalledService() error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer service.Close()
	if err := service.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return err
	}
	return nil
}

func stopInterfaceProcesses() error {
	images := []string{"ProximityUnlockAgent.exe", "ProximityUnlock.exe"}
	deadline := time.Now().Add(10 * time.Second)
	for {
		running := false
		for _, image := range images {
			command := exec.Command("taskkill.exe", "/IM", image, "/T", "/F")
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			_ = command.Run()
			if interfaceProcessRunning(image) {
				running = true
			}
		}
		if !running {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("无法结束正在运行的旧托盘，请关闭蓝牙解锁窗口后重试")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func interfaceProcessRunning(image string) bool {
	command := exec.Command("tasklist.exe", "/FI", "IMAGENAME eq "+image, "/FO", "CSV", "/NH")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.Output()
	if err != nil {
		return true
	}
	return strings.Contains(strings.ToLower(string(output)), strings.ToLower(image))
}

func enableCredentialProvider() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := ipc.CallControl(ctx, ipc.ControlRequest{Version: 1, Op: "status"})
	if err != nil || !response.OK {
		return errors.New("无法读取蓝牙解锁服务状态")
	}
	data, _ := json.Marshal(response.Payload)
	var status coordinator.Status
	if json.Unmarshal(data, &status) != nil || !status.Configured || !status.Paired || !status.CredentialValid {
		return errors.New("拒绝注册：电脑必须已初始化、手机已配对，并保存有效的 Windows 密码")
	}
	if strings.Contains(status.BLEBackend, "disabled") {
		return errors.New("拒绝注册：当前服务没有启用真实蓝牙后端")
	}
	if status.LastAuthenticated.IsZero() || time.Since(status.LastAuthenticated) > 30*time.Second {
		return errors.New("拒绝注册：请解锁手机、保持靠近，并等待电脑取得 30 秒内的新鲜蓝牙签名证明")
	}
	// LogonUI loads third-party credential providers from the trusted Windows
	// system directory. Keep the Program Files copy as the install payload, but
	// register the staged System32 copy, matching Microsoft's V2 sample layout.
	sourcePath := filepath.Join(installDir(), credentialProviderDLLName)
	dllPath := systemCredentialProviderPath()
	if err := copyFile(sourcePath, dllPath); err != nil {
		return fmt.Errorf("copy Credential Provider to System32: %w", err)
	}
	if err := callDLL(dllPath, "DllRegisterServer"); err != nil {
		_ = callDLL(dllPath, "DllUnregisterServer")
		removeOrSchedule(dllPath)
		return err
	}
	return nil
}

func updatePassword() error {
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
		return errors.New("输入的密码属于另一个 Windows 账户")
	}
	if err := secret.NewLSAStore().Put(secret.CredentialName(cfg.TargetSID), credential.Password); err != nil {
		return err
	}
	cfg.CanonicalUser = credential.CanonicalUser
	cfg.CredentialValid = true
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = ipc.CallControl(ctx, ipc.ControlRequest{Version: 1, Op: "reload"})
	return nil
}

func unregisterCredentialProvider() error {
	dllPath := systemCredentialProviderPath()
	if _, err := os.Stat(dllPath); errors.Is(err, os.ErrNotExist) {
		// Upgrade/recovery path for builds that registered the Program Files copy.
		dllPath = filepath.Join(installDir(), credentialProviderDLLName)
		if _, fallbackErr := os.Stat(dllPath); errors.Is(fallbackErr, os.ErrNotExist) {
			return nil
		}
	}
	err := callDLL(dllPath, "DllUnregisterServer")
	removeOrSchedule(systemCredentialProviderPath())
	return err
}

func systemCredentialProviderPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", credentialProviderDLLName)
}

func removeOrSchedule(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		pathPtr, pointerErr := windows.UTF16PtrFromString(path)
		if pointerErr == nil {
			_ = windows.MoveFileEx(pathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
		}
	}
}

func callDLL(path, procedure string) error {
	dll, err := windows.LoadDLL(path)
	if err != nil {
		return err
	}
	defer dll.Release()
	proc, err := dll.FindProc(procedure)
	if err != nil {
		return err
	}
	result, _, _ := proc.Call()
	if int32(result) < 0 {
		return fmt.Errorf("%s failed with HRESULT 0x%08x", procedure, uint32(result))
	}
	return nil
}

func recoverOnly() error {
	_ = unregisterCredentialProvider()
	manager, err := mgr.Connect()
	if err == nil {
		defer manager.Disconnect()
		if service, openErr := manager.OpenService(serviceName); openErr == nil {
			_, _ = service.Control(svc.Stop)
			service.Close()
		}
	}
	fmt.Println("Credential Provider disabled and service stopped. Windows PIN/password/Hello remain available.")
	return nil
}

func uninstall() error {
	_ = unregisterCredentialProvider()
	manager, err := mgr.Connect()
	if err == nil {
		if service, openErr := manager.OpenService(serviceName); openErr == nil {
			_, _ = service.Control(svc.Stop)
			_ = service.Delete()
			service.Close()
		}
		manager.Disconnect()
	}
	_ = eventlog.Remove(serviceName)
	cfg, _ := config.Load(config.ProgramDataPath())
	removeAgentRun(cfg.TargetSID)
	store := secret.NewLSAStore()
	if cfg.TargetSID != "" {
		_ = store.Delete(secret.CredentialName(cfg.TargetSID))
	}
	if cfg.PresenceSecretID != "" {
		_ = store.Delete(secret.PresenceName(cfg.PresenceSecretID))
	}
	if key, keyErr := windowskey.Open(pcKeyName, true); keyErr == nil {
		_ = key.Delete()
		_ = key.Close()
	}
	_ = os.Remove(config.ProgramDataPath())
	directory := installDir()
	entries, _ := os.ReadDir(directory)
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		if removeErr := os.Remove(path); removeErr != nil {
			pathPtr, _ := windows.UTF16PtrFromString(path)
			_ = windows.MoveFileEx(pathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
		}
	}
	_ = os.Remove(directory)
	fmt.Println("Uninstalled. In-use files, if any, are scheduled for removal after reboot.")
	return nil
}
