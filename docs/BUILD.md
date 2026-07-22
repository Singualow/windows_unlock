# 构建与安全安装

## 开发工具链

- Windows 11 x64
- Go `1.26.4`，通过 `GOROOT`、`PATH` 或 `%USERPROFILE%\Env\GOROOT` 提供
- Android SDK 35、JDK 17 和 Gradle 8.9，通过 `ANDROID_HOME`、`JAVA_HOME`、
  `PATH` 或 `%USERPROFILE%\Env\ANDROID` 下的对应目录提供
- Visual Studio 2022 Build Tools，并安装“使用 C++ 的桌面开发”和 Windows SDK

在普通 PowerShell 窗口中执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

构建过程会运行 Go 测试和静态检查、编译真实的 WinRT BLE 后端、以 `/W4 /WX`
编译 x64 凭据提供程序、构建 Android 调试 APK，并把全部 Windows 组件打包为
单文件图形安装器 `bin\ProximityUnlockInstaller.exe`。各产物的 SHA-256 会写入
`bin\SHA256SUMS.txt`。

如需生成可连续升级的个人签名 APK，再执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-personal-apk.ps1
```

## 普通用户安装

普通用户不需要执行 PowerShell、构建脚本、`proximityctl` 或安装命令。

1. 保留 Windows PIN、密码或 Windows Hello 作为恢复方式。
2. 双击 `ProximityUnlockInstaller.exe`，批准 UAC，并在 Windows 安全凭据对话框中
   输入 Microsoft 账户的真实密码，不能使用 PIN。安装器会自动启动托盘。
3. 在 Android 15+ 手机上安装 `ProximityUnlock-Android.apk`，并授予附近设备和
   通知权限。
4. 从托盘选择“配对手机…”，扫描两分钟内有效的二维码，等待托盘显示手机信号。
5. 在安全模式下先解锁手机，然后勾选“启用 Windows 锁屏自动解锁”并批准 UAC。
   如果服务尚未获得手机的新鲜认证证明，注册会被拒绝。
6. 如需在手机已经靠近时也立即撤销 `Win+L`，可勾选“锁屏后立即解锁
   （降低安全性）”。此选项只跳过默认十秒离开再返回规则，RSSI 阈值和新鲜签名
   仍然必须通过。

## Android 个人签名

`scripts\build-personal-apk.ps1` 会在 Git 忽略的 `.local` 目录中创建个人 P-256
签名证书，并使用当前 Windows 用户的 DPAPI 保护随机密码。需要保持 Android
应用升级连续性时，请将 `.local` 目录连同 Windows 用户资料一起安全备份。
调试 APK 仅供开发测试，不适合作为稳定更新渠道。

## 密码更新与恢复

更改 Microsoft 账户密码后，请从托盘选择“更新 Windows 密码…”，并在系统安全
对话框中重新录入。

凭据提供程序从不屏蔽系统内置登录方式。蓝牙或服务故障时，可以正常选择 PIN、
密码或 Windows Hello。取消勾选“启用 Windows 锁屏自动解锁”只会注销自定义
凭据提供程序；选择“卸载软件…”会删除 LSA 私密数据、CNG 身份密钥、配对记录、
服务、启动项、System32 中的凭据提供程序 DLL 和安装文件。
