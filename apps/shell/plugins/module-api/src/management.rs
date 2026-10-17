use super::{BODY_LIMIT, BridgeError, valid_group_id, valid_id, validate_body};
use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
enum Operation {
    ConfigList,
    ConfigRead,
    ConfigApply,
    ConfigValidate,
    Apps,
    Icon,
    Wifi,
    Logs,
    LogsClear,
    Ebpf,
    Diagnostics,
    NodeRead,
    NodeExport,
    NodeAdd,
    NodeEdit,
    NodeRemove,
    NodeImport,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct Request {
    request_id: String,
    operation: Operation,
    #[serde(skip_serializing_if = "Option::is_none")]
    input: Option<Map<String, Value>>,
}

pub(super) fn config_target(value: &str, write: bool) -> bool {
    if matches!(
        value,
        "module"
            | "module/wifi"
            | "module/auto_start"
            | "inbound"
            | "inbound/backend"
            | "inbound/root_policy"
            | "inbound/app"
            | "inbound/ebpf"
            | "inbound/tun"
            | "singbox/config.json"
    ) {
        return true;
    }
    if let Some(section) = value.strip_prefix("singbox/") {
        if matches!(
            section,
            "log"
                | "dns"
                | "ntp"
                | "certificate"
                | "certificate_providers"
                | "http_clients"
                | "network_namespaces"
                | "endpoints"
                | "inbounds"
                | "outbounds"
                | "providers"
                | "route"
                | "services"
                | "experimental"
        ) {
            return true;
        }
    }
    if !write
        && matches!(
            value,
            "runtime/inbound.json" | "runtime/providers.json" | "runtime/outbounds.json"
        )
    {
        return true;
    }
    value
        .strip_prefix("singbox/rules/local/")
        .is_some_and(|name| {
            !name.starts_with('.')
                && name.ends_with(".json")
                && name.len() <= 255
                && !name.contains("..")
                && name
                    .bytes()
                    .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'.' | b'_' | b'-'))
        })
}

impl Request {
    pub(super) fn parse(body: &tauri::ipc::InvokeBody) -> Result<Self, BridgeError> {
        let tauri::ipc::InvokeBody::Json(value) = body else {
            return Err(BridgeError::invalid());
        };
        validate_body(value)?;
        if value
            .as_object()
            .is_none_or(|object| object.values().any(Value::is_null))
        {
            return Err(BridgeError::invalid());
        }
        let request: Self =
            serde_json::from_value(value.clone()).map_err(|_| BridgeError::invalid())?;
        if !valid_id(&request.request_id) {
            return Err(BridgeError::invalid());
        }
        let empty = Map::new();
        let input = request.input.as_ref().unwrap_or(&empty);
        let (required, optional): (&[&str], &[&str]) = match request.operation {
            Operation::ConfigRead => (&["target"], &[]),
            Operation::ConfigApply | Operation::ConfigValidate => {
                (&["target", "revision", "content"], &[])
            }
            Operation::Icon => (&["icon_key"], &[]),
            Operation::Logs => (&[], &["kind", "lines"]),
            Operation::LogsClear => (&[], &["kind"]),
            Operation::Ebpf => (&[], &["mode", "raw"]),
            Operation::NodeRead | Operation::NodeExport | Operation::NodeRemove => {
                (&["group_id", "node_tag"], &[])
            }
            Operation::NodeAdd => (&["content"], &["group_id", "allow_insecure"]),
            Operation::NodeEdit => (&["group_id", "node_tag", "content"], &["allow_insecure"]),
            Operation::NodeImport => (&["content"], &["allow_insecure"]),
            _ => (&[], &[]),
        };
        if required.iter().any(|key| !input.contains_key(*key))
            || input
                .keys()
                .any(|key| !required.contains(&key.as_str()) && !optional.contains(&key.as_str()))
        {
            return Err(BridgeError::invalid());
        }
        for (key, value) in input {
            let valid = match key.as_str() {
                "raw" | "allow_insecure" => value.is_boolean(),
                "lines" => value.as_u64().is_some_and(|v| (1..=2000).contains(&v)),
                _ => value.as_str().is_some_and(|text| match key.as_str() {
                    "target" => {
                        config_target(text, !matches!(request.operation, Operation::ConfigRead))
                    }
                    "revision" | "icon_key" => {
                        text.len() == 64
                            && text
                                .bytes()
                                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
                    }
                    "group_id" => valid_group_id(text),
                    "node_tag" => !text.trim().is_empty() && !text.chars().any(char::is_control),
                    "kind" => matches!(text, "service" | "core"),
                    "mode" => matches!(text, "configured" | "all" | "local" | "shared"),
                    "content" => {
                        text.len() <= BODY_LIMIT
                            && (matches!(
                                request.operation,
                                Operation::ConfigApply | Operation::ConfigValidate
                            ) || !text.trim().is_empty())
                    }
                    _ => false,
                }),
            };
            if !valid {
                return Err(BridgeError::invalid());
            }
        }
        Ok(request)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    fn parse(value: Value) -> bool {
        Request::parse(&tauri::ipc::InvokeBody::Json(value)).is_ok()
    }
    #[test]
    fn targets_exclude_private_and_arbitrary_files_and_runtime_writes() {
        for value in [
            "module",
            "module/wifi",
            "module/auto_start",
            "inbound/app",
            "singbox/dns",
            "singbox/rules/local/custom-1.json",
        ] {
            assert!(config_target(value, true));
        }
        for value in [
            "api",
            "api.json",
            "telemetry",
            "module/selection",
            "/config",
            "singbox/../api.json",
            "singbox/rules/local/.a.json",
            "singbox/rules/local/a%2fb.json",
            "runtime/secret.json",
        ] {
            assert!(!config_target(value, false));
        }
        for value in [
            "runtime/inbound.json",
            "runtime/providers.json",
            "runtime/outbounds.json",
        ] {
            assert!(config_target(value, false));
            assert!(!config_target(value, true));
        }
    }
    #[test]
    fn operations_have_strict_independent_inputs() {
        for operation in ["config_list", "apps", "wifi", "diagnostics"] {
            assert!(parse(json!({"requestId":"r","operation":operation})));
            assert!(!parse(
                json!({"requestId":"r","operation":operation,"input":{"path":"/secret"}})
            ));
        }
        for (operation, input) in [
            ("config_read", json!({"target":"module/wifi"})),
            (
                "config_apply",
                json!({"target":"inbound/app","revision":"a".repeat(64),"content":"{}"}),
            ),
            (
                "config_validate",
                json!({"target":"singbox/dns","revision":"b".repeat(64),"content":"{}"}),
            ),
            ("icon", json!({"icon_key":"a".repeat(64)})),
            ("logs", json!({"kind":"core","lines":2000})),
            ("logs_clear", json!({"kind":"service"})),
            ("ebpf", json!({"mode":"all","raw":false})),
            ("node_read", json!({"group_id":"default","node_tag":"node"})),
            (
                "node_export",
                json!({"group_id":"default","node_tag":"node"}),
            ),
            (
                "node_remove",
                json!({"group_id":"default","node_tag":"node"}),
            ),
            (
                "node_add",
                json!({"content":"socks://example.invalid:1080","allow_insecure":false}),
            ),
            (
                "node_edit",
                json!({"group_id":"default","node_tag":"node","content":"{}"}),
            ),
            ("node_import", json!({"content":"{}"})),
        ] {
            assert!(parse(
                json!({"requestId":"r","operation":operation,"input":input})
            ));
            let mut extra = input.clone();
            extra["method"] = json!("GET");
            assert!(!parse(
                json!({"requestId":"r","operation":operation,"input":extra})
            ));
            for key in input.as_object().unwrap().keys() {
                let mut invalid = input.clone();
                invalid[key] = Value::Null;
                assert!(!parse(
                    json!({"requestId":"r","operation":operation,"input":invalid})
                ));
            }
        }
        for value in [
            json!({"requestId":"r","operation":"GET"}),
            json!({"requestId":"r","operation":"apps","url":"https://example.invalid"}),
            json!({"requestId":"r","operation":"apps","input":null}),
            json!({"requestId":"r","operation":"logs","input":{"lines":2001}}),
            json!({"requestId":"r","operation":"node_read","input":{"group_id":"a/b","node_tag":"x"}}),
        ] {
            assert!(!parse(value));
        }
        assert!(Request::parse(&tauri::ipc::InvokeBody::Json(json!({"requestId":"r","operation":"node_import","input":{"content":"x".repeat(BODY_LIMIT)}}))).is_err());
    }
}
