use crate::{BridgeError, validate_body};
use serde::{Deserialize, Serialize};

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub(crate) enum DocumentKind {
    Config,
    Nodes,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct PickDocument {
    kind: DocumentKind,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct TakeImport {
    #[serde(skip_serializing_if = "Option::is_none")]
    completed_id: Option<String>,
}

impl TakeImport {
    pub(crate) fn parse(body: &tauri::ipc::InvokeBody) -> Result<Self, BridgeError> {
        let request: Self = parse(body)?;
        if request
            .completed_id
            .as_deref()
            .is_some_and(|id| !crate::valid_id(id))
        {
            return Err(BridgeError::invalid());
        }
        if let tauri::ipc::InvokeBody::Json(value) = body {
            if value
                .get("completedId")
                .is_some_and(serde_json::Value::is_null)
            {
                return Err(BridgeError::invalid());
            }
        }
        Ok(request)
    }
}

pub(crate) fn parse<T: serde::de::DeserializeOwned>(
    body: &tauri::ipc::InvokeBody,
) -> Result<T, BridgeError> {
    let tauri::ipc::InvokeBody::Json(value) = body else {
        return Err(BridgeError::invalid());
    };
    validate_body(value)?;
    serde_json::from_value(value.clone()).map_err(|_| BridgeError::invalid())
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use tauri::ipc::InvokeBody;

    #[test]
    fn picker_accepts_only_two_fixed_purposes_and_no_file_or_uri() {
        for kind in ["config", "nodes"] {
            assert!(parse::<PickDocument>(&InvokeBody::Json(json!({"kind": kind}))).is_ok());
        }
        for value in [
            json!({}),
            json!({"kind":"file"}),
            json!({"kind":"config","uri":"content://private"}),
            json!({"kind":"nodes","path":"/data/adb"}),
            json!({"kind":"nodes","url":"https://example.org"}),
            json!({"kind":"nodes","method":"GET"}),
            json!({"kind":null}),
        ] {
            assert!(parse::<PickDocument>(&InvokeBody::Json(value)).is_err());
        }
    }

    #[test]
    fn pending_import_cannot_target_another_document_or_pass_a_path() {
        assert!(TakeImport::parse(&InvokeBody::Json(json!({}))).is_ok());
        assert!(TakeImport::parse(&InvokeBody::Json(json!({"completedId":"fixture-id"}))).is_ok());
        for value in [
            json!({"uri":"content://private"}),
            json!({"completedId":null}),
            json!({"completedId":"../file"}),
            json!({"completedId":false}),
        ] {
            assert!(TakeImport::parse(&InvokeBody::Json(value)).is_err());
        }
    }
}
