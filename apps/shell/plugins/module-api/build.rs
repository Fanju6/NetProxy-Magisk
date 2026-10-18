const COMMANDS: &[&str] = &[
    "read",
    "subscription_read",
    "subscription_action",
    "subscription_write",
    "management",
    "service_action",
    "select_node",
    "change_mode",
    "test_delay",
    "cancel",
    "open_events",
    "next_event",
    "close_events",
    "visibility",
    "set_appearance",
    "register_listener",
    "remove_listener",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS)
        .android_path("android")
        .build();
}
