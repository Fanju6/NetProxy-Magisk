use crate::BridgeError;
use serde::{Deserialize, Serialize};

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
enum Theme {
    Light,
    Dark,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct Request {
    theme: Theme,
}

impl Request {
    pub(crate) fn parse(body: &tauri::ipc::InvokeBody) -> Result<Self, BridgeError> {
        let tauri::ipc::InvokeBody::Json(value) = body else {
            return Err(BridgeError::invalid());
        };
        serde_json::from_value(value.clone()).map_err(|_| BridgeError::invalid())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn appearance_accepts_only_resolved_light_or_dark_without_extra_fields() {
        for theme in ["light", "dark"] {
            let value = json!({"theme": theme});
            let request = Request::parse(&tauri::ipc::InvokeBody::Json(value.clone())).unwrap();
            assert_eq!(serde_json::to_value(request).unwrap(), value);
        }
        for value in [
            json!({}),
            json!({"theme": "system"}),
            json!({"theme": "Dark"}),
            json!({"theme": " dark"}),
            json!({"theme": ""}),
            json!({"theme": null}),
            json!({"theme": false}),
            json!({"theme": 1}),
            json!({"theme": {"dark": true}}),
            json!({"theme": "dark", "method": "POST"}),
            json!({"theme": "light", "path": "/service/start"}),
            json!({"theme": "dark", "url": "https://localhost"}),
            json!({"theme": "dark", "system": true}),
            json!({"theme": "light", "unknown": null}),
            json!({"request": {"theme": "light"}}),
            json!("dark"),
            json!([]),
            json!(null),
        ] {
            let error = Request::parse(&tauri::ipc::InvokeBody::Json(value)).unwrap_err();
            assert_eq!(error.code, "api.invalid_request");
            assert_eq!(error.status, 400);
        }
        assert!(Request::parse(&tauri::ipc::InvokeBody::Raw(vec![])).is_err());
    }
}
