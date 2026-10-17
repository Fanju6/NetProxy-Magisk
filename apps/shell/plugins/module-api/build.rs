const COMMANDS: &[&str] = &[
    "read",
    "service_action",
    "select_node",
    "change_mode",
    "test_delay",
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
