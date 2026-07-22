use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use std::io::{Read, Write};
use std::time::{Duration, Instant};
use tauri::menu::{Menu, MenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{Manager, Runtime};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

const CONTROL_PIPE: &str = r"\\.\pipe\ProximityUnlock.Control.v1";
const MAX_MESSAGE_SIZE: usize = 64 * 1024;
const CREDENTIAL_PROVIDER_KEY: &str = r"SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\Credential Providers\{C81FCF2E-B9D0-4EAF-8D35-55F750D2561B}";

#[derive(Serialize)]
struct ControlRequest<'a> {
    version: u8,
    op: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    payload: Option<Value>,
}

#[derive(Deserialize)]
struct ControlResponse {
    ok: bool,
    #[serde(default)]
    error: String,
    payload: Option<Value>,
}

#[derive(Default, Deserialize)]
struct RuntimeStatus {
    #[serde(default)]
    session_active: bool,
    #[serde(default)]
    locked: bool,
    #[serde(default)]
    should_lock: bool,
    #[serde(default)]
    auto_lock: bool,
    paused_until: Option<String>,
}

#[derive(Default, Deserialize)]
struct RuntimeDefaults {
    #[serde(default)]
    auto_lock: bool,
    paused_until: Option<String>,
}

#[derive(Serialize)]
struct SystemIntegration {
    credential_provider_registered: bool,
}

#[cfg(windows)]
fn call_control(op: &str, payload: Option<Value>) -> Result<Value, String> {
    use std::fs::OpenOptions;

    let mut last_error = None;
    let mut pipe = None;
    for _ in 0..12 {
        match OpenOptions::new().read(true).write(true).open(CONTROL_PIPE) {
            Ok(connection) => {
                pipe = Some(connection);
                break;
            }
            Err(error) => {
                last_error = Some(error);
                std::thread::sleep(Duration::from_millis(100));
            }
        }
    }
    let mut pipe = pipe.ok_or_else(|| {
        format!(
            "无法连接蓝牙解锁服务：{}",
            last_error
                .map(|error| error.to_string())
                .unwrap_or_else(|| "未知错误".to_owned())
        )
    })?;

    let request = serde_json::to_vec(&ControlRequest {
        version: 1,
        op,
        payload,
    })
    .map_err(|error| format!("无法编码服务请求：{error}"))?;
    if request.is_empty() || request.len() > MAX_MESSAGE_SIZE {
        return Err("服务请求大小无效".to_owned());
    }

    pipe.write_all(&(request.len() as u32).to_le_bytes())
        .and_then(|_| pipe.write_all(&request))
        .and_then(|_| pipe.flush())
        .map_err(|error| format!("无法向蓝牙解锁服务发送请求：{error}"))?;

    let mut header = [0_u8; 4];
    pipe.read_exact(&mut header)
        .map_err(|error| format!("无法读取蓝牙解锁服务响应：{error}"))?;
    let length = u32::from_le_bytes(header) as usize;
    if length == 0 || length > MAX_MESSAGE_SIZE {
        return Err("蓝牙解锁服务返回了无效数据".to_owned());
    }
    let mut body = vec![0_u8; length];
    pipe.read_exact(&mut body)
        .map_err(|error| format!("蓝牙解锁服务响应不完整：{error}"))?;
    let response: ControlResponse = serde_json::from_slice(&body)
        .map_err(|error| format!("无法解析蓝牙解锁服务响应：{error}"))?;
    if !response.ok {
        return Err(if response.error.is_empty() {
            "蓝牙解锁服务拒绝了操作".to_owned()
        } else {
            response.error
        });
    }
    Ok(response.payload.unwrap_or(Value::Null))
}

#[cfg(not(windows))]
fn call_control(_op: &str, _payload: Option<Value>) -> Result<Value, String> {
    Err("蓝牙解锁桌面端仅支持 Windows".to_owned())
}

async fn call_control_async(op: &'static str, payload: Option<Value>) -> Result<Value, String> {
    tauri::async_runtime::spawn_blocking(move || call_control(op, payload))
        .await
        .map_err(|error| format!("桌面端后台任务失败：{error}"))?
}

#[tauri::command]
async fn get_status() -> Result<Value, String> {
    call_control_async("status", None).await
}

#[tauri::command]
async fn set_mode(mode: String) -> Result<(), String> {
    if mode != "strict" && mode != "convenience" {
        return Err("不支持的解锁模式".to_owned());
    }
    call_control_async("set_mode", Some(json!({ "mode": mode })))
        .await
        .map(|_| ())
}

#[tauri::command]
async fn set_auto_lock(enabled: bool) -> Result<(), String> {
    call_control_async("set_auto_lock", Some(json!({ "enabled": enabled })))
        .await
        .map(|_| ())
}

#[tauri::command]
async fn set_immediate_unlock(enabled: bool) -> Result<(), String> {
    call_control_async("set_immediate_unlock", Some(json!({ "enabled": enabled })))
        .await
        .map(|_| ())
}

#[tauri::command]
async fn set_failure_cooldown(enabled: bool) -> Result<(), String> {
    call_control_async(
        "set_failure_cooldown",
        Some(json!({ "enabled": enabled })),
    )
    .await
    .map(|_| ())
}

#[tauri::command]
async fn set_thresholds(unlock_rssi: i32, lock_rssi: i32) -> Result<(), String> {
    if unlock_rssi <= lock_rssi
        || !(-90..=-20).contains(&unlock_rssi)
        || !(-120..=-28).contains(&lock_rssi)
        || unlock_rssi - lock_rssi < 8
    {
        return Err("距离阈值无效或安全滞回不足".to_owned());
    }
    call_control_async(
        "set_thresholds",
        Some(json!({ "unlock_rssi": unlock_rssi, "lock_rssi": lock_rssi })),
    )
    .await
    .map(|_| ())
}

#[tauri::command]
async fn pause(seconds: u32) -> Result<Value, String> {
    if seconds > 86_400 {
        return Err("暂停时间不能超过一天".to_owned());
    }
    call_control_async("pause", Some(json!({ "seconds": seconds }))).await
}

#[tauri::command]
async fn start_pairing() -> Result<Value, String> {
    call_control_async("pair_start", None).await
}

#[tauri::command]
fn get_system_integration() -> SystemIntegration {
    SystemIntegration {
        credential_provider_registered: credential_provider_registered(),
    }
}

#[tauri::command]
async fn run_setup_action(app: tauri::AppHandle, action: String) -> Result<(), String> {
    let uninstall = action == "uninstall";
    if !matches!(
        action.as_str(),
        "enable-credential-provider" | "disable-credential-provider" | "set-password" | "uninstall"
    ) {
        return Err("不支持的系统维护操作".to_owned());
    }
    tauri::async_runtime::spawn_blocking(move || run_setup_elevated(&action))
        .await
        .map_err(|error| format!("系统维护任务失败：{error}"))??;
    if uninstall {
        app.exit(0);
    }
    Ok(())
}

#[tauri::command]
async fn revoke_pairing() -> Result<(), String> {
    call_control_async("revoke", None).await.map(|_| ())
}

#[tauri::command]
fn lock_workstation() -> Result<(), String> {
    lock_workstation_impl()
}

#[cfg(windows)]
fn lock_workstation_impl() -> Result<(), String> {
    let locked = unsafe { windows_sys::Win32::System::Shutdown::LockWorkStation() };
    if locked == 0 {
        return Err(format!("无法锁定电脑：{}", std::io::Error::last_os_error()));
    }
    Ok(())
}

fn pause_is_active(paused_until: Option<&str>) -> bool {
    paused_until
        .and_then(|value| OffsetDateTime::parse(value, &Rfc3339).ok())
        .is_some_and(|until| until > OffsetDateTime::now_utc())
}

#[cfg(windows)]
fn load_runtime_defaults() -> RuntimeDefaults {
    let program_data = std::env::var_os("ProgramData")
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|| std::path::PathBuf::from(r"C:\ProgramData"));
    std::fs::read(program_data.join("ProximityUnlock").join("config.json"))
        .ok()
        .and_then(|data| serde_json::from_slice(&data).ok())
        .unwrap_or_default()
}

#[cfg(not(windows))]
fn load_runtime_defaults() -> RuntimeDefaults {
    RuntimeDefaults::default()
}

#[cfg(windows)]
fn current_console_session() -> Option<u32> {
    let process_id = unsafe { windows_sys::Win32::System::Threading::GetCurrentProcessId() };
    let mut session_id = 0_u32;
    let found = unsafe {
        windows_sys::Win32::System::RemoteDesktop::ProcessIdToSessionId(process_id, &mut session_id)
    };
    let active =
        unsafe { windows_sys::Win32::System::RemoteDesktop::WTSGetActiveConsoleSessionId() };
    if found != 0 && active != u32::MAX && session_id == active {
        Some(session_id)
    } else {
        None
    }
}

#[cfg(windows)]
fn credential_provider_registered() -> bool {
    use winreg::RegKey;
    use winreg::enums::HKEY_LOCAL_MACHINE;

    RegKey::predef(HKEY_LOCAL_MACHINE)
        .open_subkey(CREDENTIAL_PROVIDER_KEY)
        .is_ok()
}

#[cfg(not(windows))]
fn credential_provider_registered() -> bool {
    false
}

#[cfg(windows)]
fn run_setup_elevated(action: &str) -> Result<(), String> {
    use std::ffi::OsStr;
    use std::os::windows::ffi::OsStrExt;
    use windows_sys::Win32::Foundation::CloseHandle;
    use windows_sys::Win32::System::Threading::{GetExitCodeProcess, WaitForSingleObject};
    use windows_sys::Win32::UI::Shell::{
        SEE_MASK_NOCLOSEPROCESS, SHELLEXECUTEINFOW, ShellExecuteExW,
    };

    let setup = std::env::current_exe()
        .map_err(|error| format!("无法定位当前程序：{error}"))?
        .parent()
        .ok_or_else(|| "无法定位安装目录".to_owned())?
        .join("setup.exe");
    if !setup.is_file() {
        return Err("安装目录缺少 setup.exe".to_owned());
    }
    let verb: Vec<u16> = OsStr::new("runas").encode_wide().chain(Some(0)).collect();
    let file: Vec<u16> = setup.as_os_str().encode_wide().chain(Some(0)).collect();
    let parameters: Vec<u16> = OsStr::new(action).encode_wide().chain(Some(0)).collect();
    let mut info = SHELLEXECUTEINFOW {
        cbSize: std::mem::size_of::<SHELLEXECUTEINFOW>() as u32,
        fMask: SEE_MASK_NOCLOSEPROCESS,
        lpVerb: verb.as_ptr(),
        lpFile: file.as_ptr(),
        lpParameters: parameters.as_ptr(),
        nShow: 1,
        ..Default::default()
    };
    let launched = unsafe { ShellExecuteExW(&mut info) };
    if launched == 0 || info.hProcess.is_null() {
        return Err(format!(
            "管理员确认被取消或无法启动：{}",
            std::io::Error::last_os_error()
        ));
    }
    let wait_result = unsafe { WaitForSingleObject(info.hProcess, u32::MAX) };
    let mut exit_code = 1_u32;
    let read_exit = unsafe { GetExitCodeProcess(info.hProcess, &mut exit_code) };
    unsafe {
        CloseHandle(info.hProcess);
    }
    if wait_result != 0 {
        return Err("等待管理员操作完成时发生错误".to_owned());
    }
    if read_exit == 0 || exit_code != 0 {
        return Err(format!("系统维护程序返回错误代码 {exit_code}"));
    }
    Ok(())
}

#[cfg(not(windows))]
fn run_setup_elevated(_action: &str) -> Result<(), String> {
    Err("系统维护操作仅支持 Windows".to_owned())
}

#[cfg(not(windows))]
fn current_console_session() -> Option<u32> {
    None
}

fn start_runtime_monitor() {
    let Some(session_id) = current_console_session() else {
        return;
    };
    let defaults = load_runtime_defaults();
    let _ = std::thread::Builder::new()
        .name("proximity-runtime-monitor".to_owned())
        .spawn(move || {
            let mut last_auto_lock = defaults.auto_lock;
            let mut last_paused = pause_is_active(defaults.paused_until.as_deref());
            let mut unavailable_since: Option<Instant> = None;
            let mut fail_safe_locked = false;

            let _ = call_control("session_active", Some(json!({ "session_id": session_id })));
            loop {
                match call_control("status", None) {
                    Ok(payload) => {
                        unavailable_since = None;
                        fail_safe_locked = false;
                        if let Ok(status) = serde_json::from_value::<RuntimeStatus>(payload) {
                            last_auto_lock = status.auto_lock;
                            last_paused = pause_is_active(status.paused_until.as_deref());
                            if !status.session_active {
                                let _ = call_control(
                                    "session_active",
                                    Some(json!({ "session_id": session_id })),
                                );
                            } else if status.should_lock && !status.locked {
                                let _ = lock_workstation_impl();
                            }
                        }
                    }
                    Err(_) => {
                        let started = unavailable_since.get_or_insert_with(Instant::now);
                        if !fail_safe_locked
                            && last_auto_lock
                            && !last_paused
                            && started.elapsed() >= Duration::from_secs(20)
                        {
                            let _ = lock_workstation_impl();
                            fail_safe_locked = true;
                        }
                    }
                }
                std::thread::sleep(Duration::from_secs(2));
            }
        });
}

#[cfg(not(windows))]
fn lock_workstation_impl() -> Result<(), String> {
    Err("锁定电脑仅支持 Windows".to_owned())
}

fn show_main_window<R: Runtime>(app: &tauri::AppHandle<R>) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(
            |app, _arguments, _cwd| {
                show_main_window(app);
            },
        ))
        .setup(|app| {
            let background = std::env::args_os().any(|value| value == "--background");
            let show = MenuItem::with_id(app, "show", "显示蓝牙解锁", true, None::<&str>)?;
            let lock = MenuItem::with_id(app, "lock", "立即锁定电脑", true, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "退出界面", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&show, &lock, &quit])?;
            let mut tray = TrayIconBuilder::new()
                .tooltip("蓝牙解锁")
                .menu(&menu)
                .show_menu_on_left_click(false)
                .on_menu_event(|app, event| match event.id().as_ref() {
                    "show" => show_main_window(app),
                    "lock" => {
                        let _ = lock_workstation_impl();
                    }
                    "quit" => app.exit(0),
                    _ => {}
                })
                .on_tray_icon_event(|tray, event| {
                    if let TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } = event
                    {
                        show_main_window(tray.app_handle());
                    }
                });
            if let Some(icon) = app.default_window_icon() {
                tray = tray.icon(icon.clone());
            }
            tray.build(app)?;
            start_runtime_monitor();
            if background {
                let app_handle = app.handle().clone();
                let _ = std::thread::Builder::new()
                    .name("proximity-background-hide".to_owned())
                    .spawn(move || {
                        std::thread::sleep(Duration::from_millis(750));
                        if let Some(window) = app_handle.get_webview_window("main") {
                            let _ = window.hide();
                        }
                    });
            } else {
                show_main_window(app.handle());
            }
            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                api.prevent_close();
                let _ = window.hide();
            }
        })
        .invoke_handler(tauri::generate_handler![
            get_status,
            set_mode,
            set_auto_lock,
            set_immediate_unlock,
            set_failure_cooldown,
            set_thresholds,
            pause,
            start_pairing,
            revoke_pairing,
            get_system_integration,
            run_setup_action,
            lock_workstation
        ])
        .run(tauri::generate_context!())
        .expect("蓝牙解锁桌面端启动失败");
}

#[cfg(all(test, windows))]
mod integration_tests {
    use super::*;

    #[test]
    #[ignore = "需要本机已安装并运行 ProximityUnlockSvc"]
    fn reads_status_from_installed_service() {
        let payload = call_control("status", None).expect("无法读取已安装服务状态");
        assert!(payload.get("configured").and_then(Value::as_bool).is_some());
        assert!(payload.get("mode").and_then(Value::as_str).is_some());
        assert!(
            payload
                .get("authorization")
                .and_then(Value::as_object)
                .is_some()
        );
    }
}
