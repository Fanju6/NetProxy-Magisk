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
    CatalogCreate,
    CatalogRename,
    CatalogRemove,
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
    if !write && matches!(value, "runtime/inbound.json" | "runtime/providers.json") {
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
            Operation::CatalogCreate | Operation::CatalogRename => (&["id", "name"], &[]),
            Operation::CatalogRemove => (&["id"], &[]),
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
                    "id" => {
                        valid_group_id(text)
                            && !(matches!(request.operation, Operation::CatalogRemove)
                                && text == "default")
                    }
                    "name" => {
                        !text.trim().is_empty()
                            && text.len() <= 256
                            && !text.chars().any(char::is_control)
                    }
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
        for value in ["runtime/inbound.json", "runtime/providers.json"] {
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

    #[test]
    fn management_requests_enforce_the_complete_utf8_body_limit() {
        let mut value = json!({"requestId":"r","operation":"node_import","input":{"content":""}});
        let overhead = serde_json::to_vec(&value).unwrap().len();
        value["input"]["content"] = json!("x".repeat(BODY_LIMIT - overhead));
        assert_eq!(serde_json::to_vec(&value).unwrap().len(), BODY_LIMIT);
        Request::parse(&tauri::ipc::InvokeBody::Json(value.clone())).unwrap();
        value["input"]["content"] = json!("x".repeat(BODY_LIMIT - overhead + 1));
        let error = Request::parse(&tauri::ipc::InvokeBody::Json(value))
            .err()
            .unwrap();
        assert_eq!(error.status, 413);
        assert_eq!(error.code, "api.body_too_large");
        for escaped in [r"\ud800", r"\udc00"] {
            for (operation, input) in [
                (
                    "node_read",
                    format!(r#"{{"group_id":"default","node_tag":"{escaped}"}}"#),
                ),
                ("node_add", format!(r#"{{"content":"{escaped}"}}"#)),
                (
                    "catalog_create",
                    format!(r#"{{"id":"local-1","name":"{escaped}"}}"#),
                ),
            ] {
                let raw =
                    format!(r#"{{"requestId":"r","operation":"{operation}","input":{input}}}"#);
                assert!(serde_json::from_str::<Value>(&raw).is_err());
            }
        }
    }

    #[test]
    fn catalog_operations_are_fixed_strict_and_validate_utf8_byte_lengths() {
        for operation in ["catalog_create", "catalog_rename", "catalog_remove"] {
            let mut input = json!({"id":"local-1"});
            if operation != "catalog_remove" {
                input["name"] = json!(" Local ");
            }
            let accepted =
                |input| parse(json!({"requestId":"r","operation":operation,"input":input}));
            assert!(accepted(input.clone()));
            for key in input.as_object().unwrap().keys() {
                let mut missing = input.clone();
                missing.as_object_mut().unwrap().remove(key);
                assert!(!accepted(missing));
                for item in [json!(null), json!(1), json!(true), json!([]), json!({})] {
                    let mut invalid = input.clone();
                    invalid[key] = item;
                    assert!(!accepted(invalid));
                }
            }
            for key in [
                "method",
                "path",
                "url",
                "query",
                "body",
                "headers",
                "type",
                "provider_tag",
            ] {
                let mut invalid = input.clone();
                invalid[key] = json!("x");
                assert!(!accepted(invalid));
            }
            for id in [
                "",
                "staging",
                "a..b",
                "a/b",
                "a%2fb",
                "a?x=1",
                "_a",
                "\u{4e2d}\u{6587}",
            ] {
                let mut invalid = input.clone();
                invalid["id"] = json!(id);
                assert!(!accepted(invalid));
            }
            if operation != "catalog_remove" {
                for name in [
                    String::new(),
                    " ".into(),
                    "a\n".into(),
                    "\u{7f}".into(),
                    "\u{85}".into(),
                    "x".repeat(257),
                    "\u{9f99}".repeat(86),
                ] {
                    assert!(!accepted(json!({"id":"local-1","name":name})));
                }
                for name in [
                    "x".repeat(256),
                    "\u{9f99}".repeat(85) + "a",
                    " Local ".into(),
                ] {
                    let value = json!({"requestId":"r","operation":operation,"input":{"id":"local-1","name":name}});
                    let request =
                        Request::parse(&tauri::ipc::InvokeBody::Json(value.clone())).unwrap();
                    assert_eq!(serde_json::to_value(request).unwrap(), value);
                }
            }
        }
        assert!(!parse(
            json!({"requestId":"r","operation":"catalog_remove","input":{"id":"default"}})
        ));
        assert!(!parse(
            json!({"requestId":"r","operation":"catalog_remove","input":{"id":"local-1","name":"extra"}})
        ));
        assert!(!parse(
            json!({"requestId":"r","operation":"catalog_delete","input":{"id":"local-1"}})
        ));
        assert!(serde_json::from_str::<Request>(r#"{"requestId":"r","operation":"catalog_create","input":{"id":"local-1","name":"\ud800"}}"#).is_err());
        assert!(include_str!("../permissions/default.toml").contains("allow-management"));
        assert!(!include_str!("../build.rs").contains("\"catalog_create\""));
    }
}
