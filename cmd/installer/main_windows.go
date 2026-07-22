//go:build windows

package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/singu/proximity-unlock/internal/config"
	"github.com/singu/proximity-unlock/internal/wincred"
	"github.com/singu/proximity-unlock/internal/winshell"
)

// payload is populated by scripts/build.ps1 before the final installer build.
//
//go:embed payload/*
var payload embed.FS

var payloadFiles = []string{
	"ProximityUnlockSvc.exe",
	"ProximityUnlock.exe",
	"proximityctl.exe",
	"ProximityUnlockCredentialProvider.dll",
	"setup.exe",
	"uninstall.exe",
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--verify-payload" {
		if err := verifyPayload(); err != nil {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--elevated" {
		if !wincred.IsElevated() {
			os.Exit(2)
		}
		if err := install(); err != nil {
			winshell.Error("Proximity Unlock 安装失败", err.Error())
			os.Exit(1)
		}
		return
	}

	if !winshell.Confirm(
		"安装 Proximity Unlock",
		"将安装或升级 Windows 蓝牙解锁服务与中文可视化托盘。\n\n首次安装会出现管理员确认，并要求输入当前 Windows/Microsoft 账户密码（不能使用 PIN）。升级会保留手机配对、密码和现有解锁组件。系统 PIN、密码和 Windows Hello 不会被隐藏。\n\n点击“确定”继续。",
	) {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		winshell.Error("Proximity Unlock 安装失败", err.Error())
		return
	}
	exitCode, err := winshell.ExecuteWait("runas", executable, "--elevated")
	if err != nil {
		winshell.Error("Proximity Unlock 安装失败", "管理员确认被取消或无法启动：\n"+err.Error())
		return
	}
	if exitCode != 0 {
		winshell.Error("Proximity Unlock 安装失败", fmt.Sprintf("安装进程返回错误代码 %d。", exitCode))
		return
	}
	interfaceBinary := filepath.Join(programFiles(), "ProximityUnlock", "ProximityUnlock.exe")
	if err := exec.Command(interfaceBinary).Start(); err != nil {
		winshell.Error("Proximity Unlock", "安装已完成，但托盘启动失败。下次登录会自动启动。\n"+err.Error())
		return
	}
	winshell.Info("Proximity Unlock", "安装或升级完成，中文控制中心已经启动。\n\n手机配对和现有自动解锁配置已保留；首次安装请在“设备”页面完成配对。")
}

func install() error {
	if err := verifyPayload(); err != nil {
		return fmt.Errorf("安装包完整性检查失败：%w", err)
	}
	installDir := filepath.Join(programFiles(), "ProximityUnlock")
	entries, readInstallErr := os.ReadDir(installDir)
	upgrade := readInstallErr == nil && len(entries) != 0
	tempDir, err := os.MkdirTemp("", "ProximityUnlockInstaller-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	if err := extractPayload(tempDir); err != nil {
		return err
	}
	ctl := filepath.Join(tempDir, "proximityctl.exe")
	if _, err := os.Stat(config.ProgramDataPath()); errors.Is(err, os.ErrNotExist) {
		if err := runHidden(ctl, "initialize"); err != nil {
			return fmt.Errorf("账户初始化失败：%w", err)
		}
	}
	if err := runHidden(ctl, "self-test"); err != nil {
		return fmt.Errorf("安全自检失败：%w", err)
	}
	if upgrade {
		if err := runHidden(filepath.Join(tempDir, "setup.exe"), "upgrade-ui"); err != nil {
			return fmt.Errorf("软件就地升级失败：%w", err)
		}
		return nil
	}
	if err := runHidden(filepath.Join(tempDir, "setup.exe"), "install"); err != nil {
		return fmt.Errorf("Windows 服务安装失败：%w", err)
	}
	return nil
}

func extractPayload(destination string) error {
	for _, name := range payloadFiles {
		data, err := fs.ReadFile(payload, "payload/"+name)
		if err != nil {
			return fmt.Errorf("安装包缺少 %s", name)
		}
		path := filepath.Join(destination, name)
		if err := os.WriteFile(path, data, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func verifyPayload() error {
	for _, name := range payloadFiles {
		data, err := fs.ReadFile(payload, "payload/"+name)
		if err != nil {
			return err
		}
		if len(data) < 2 || data[0] != 'M' || data[1] != 'Z' {
			return fmt.Errorf("%s is not a Windows PE image", name)
		}
	}
	return nil
}

func runHidden(path string, arguments ...string) error {
	command := exec.Command(path, arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	return errors.New(message)
}

func programFiles() string {
	if value := os.Getenv("ProgramFiles"); value != "" {
		return value
	}
	return `C:\Program Files`
}
