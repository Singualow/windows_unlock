#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    proximity_unlock_desktop_lib::run();
}
