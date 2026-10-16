const COMMANDS: &[&str] = &[
    "read",
    "cancel",
    "open_events",
    "next_event",
    "close_events",
    "visibility",
    "register_listener",
    "remove_listener",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS)
        .android_path("android")
        .build();
}
