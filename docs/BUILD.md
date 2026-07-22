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

如需验证 Android 正式构建，再执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-android-unsigned.ps1
```

此脚本只生成未签名的 Release APK，并运行 Release 单元测试和 Lint；未签名产物不能
分发。正式 Release 必须在受保护的发布环境中签名，并使用 Android SDK `apksigner`
独立验证证书和签名方案。

## 普通用户安装

普通用户不需要执行 PowerShell、构建脚本、`proximityctl` 或安装命令。

1. 保留 Windows PIN、密码或 Windows Hello 作为恢复方式。
2. 双击 `ProximityUnlockInstaller.exe`，批准 UAC，并在 Windows 安全凭据对话框中
   输入 Microsoft 账户的真实密码，不能使用 PIN。安装器会自动启动托盘。
3. 在 Android 15+ 手机上安装 `ProximityUnlock-Android.apk`，并授予附近设备和
   通知权限。
4. 打开托盘中的“蓝牙解锁”，在“设备”页生成二维码，扫描两分钟内有效的二维码，
   等待控制中心显示手机信号。
5. 在安全模式下先解锁手机，然后在“设置”的“系统维护”区域启用 Windows 锁屏
   自动解锁并批准 UAC。
   如果服务尚未获得手机的新鲜认证证明，注册会被拒绝。
6. 如需在手机已经靠近时也立即撤销 `Win+L`，可勾选“锁屏后立即解锁
   （降低安全性）”。此选项只跳过默认十秒离开再返回规则，RSSI 阈值和新鲜签名
   仍然必须通过。
7. “设备”页只设置普通模式的解锁/锁定 RSSI 阈值；高灵敏触发值和 RSSI 趋势预测
   灵敏度分别在“设置”页的独立参数弹窗中调整。高灵敏模式锁屏时约 `0.2` 秒内开始
   认证；快速模式连续 `10` 秒没有恢复有效证明才锁定。
8. 概览页距离图应使用真实滚动 10 分钟时间轴；日志页最多展示本次服务运行的最近
   100 条脱敏记录，连续相同认证结果不得重复增长。

## Android 正式签名

签名密钥和密码不存放在仓库、环境变量、构建脚本或日志中。`0.2.1` 及后续正式
APK 使用固定证书，SHA-256 为
`E4156161C60F5234821282687EAD2A43616925BBB5AEBA81D35845CA157C7661`。
发布前必须确认至少启用了 APK Signature Scheme v2，并核对该证书指纹。
调试 APK 和未签名 APK 仅供开发测试，不适合作为安装或更新渠道。

## 密码更新与恢复

更改 Microsoft 账户密码后，请打开“设置”的“系统维护”区域，选择“更新密码”，
并在系统安全对话框中重新录入。

凭据提供程序从不屏蔽系统内置登录方式。蓝牙或服务故障时，可以正常选择 PIN、
密码或 Windows Hello。“停用组件”只会注销自定义凭据提供程序；选择“卸载软件”
会删除 LSA 私密数据、CNG 身份密钥、配对记录、
服务、启动项、System32 中的凭据提供程序 DLL 和安装文件。
