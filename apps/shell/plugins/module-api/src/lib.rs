use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
#[cfg(target_os = "android")]
use tauri::Manager;
use tauri::{
    AppHandle, Runtime,
    plugin::{Builder, TauriPlugin},
};

#[derive(Debug, Serialize)]
pub struct BridgeError {
    code: String,
    status: u16,
    message: &'static str,
}

impl BridgeError {
    fn invalid() -> Self {
        Self {
            code: "api.invalid_request".into(),
            status: 400,
            message: "Invalid native request",
        }
    }

    #[cfg(not(target_os = "android"))]
    fn unavailable() -> Self {
        Self {
            code: "api.connection_unavailable".into(),
            status: 503,
            message: "Native connection unavailable",
        }
    }
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum Resource {
    Info,
    Service,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum ServiceAction {
    Start,
    Stop,
}

fn valid_id(id: &str) -> bool {
    !id.is_empty()
        && id.len() <= 96
        && id
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
}

fn valid_topics(topics: &[String]) -> bool {
    !topics.is_empty()
        && topics.len() <= 3
        && topics
            .iter()
            .all(|topic| matches!(topic.as_str(), "status" | "groups" | "subscriptions"))
        && topics
            .iter()
            .enumerate()
            .all(|(i, topic)| !topics[..i].contains(topic))
}

#[cfg(target_os = "android")]
struct Native<R: Runtime>(tauri::plugin::PluginHandle<R>);

async fn dispatch<R: Runtime>(
    app: AppHandle<R>,
    command: &'static str,
    args: Value,
) -> Result<Value, BridgeError> {
    #[cfg(target_os = "android")]
    {
        app.state::<Native<R>>()
            .0
            .run_mobile_plugin_async(command, args)
            .await
            .map_err(|error| {
                // Never forward framework or HTTP exception text to JavaScript.
                let raw = match error {
                    tauri::plugin::mobile::PluginInvokeError::InvokeRejected(response) => {
                        response.code
                    }
                    _ => None,
                };
                let code = raw.as_deref().unwrap_or("api.connection_failed");
                let code = if code.starts_with("api.")
                    && code.len() <= 64
                    && code
                        .bytes()
                        .all(|b| b.is_ascii_lowercase() || b == b'.' || b == b'_')
                {
                    code.to_owned()
                } else {
                    "api.connection_failed".into()
                };
                let status = match code.as_str() {
                    "api.unauthorized" => 401,
                    "api.invalid_request" => 400,
                    "api.read_only" => 403,
                    "api.body_too_large" => 413,
                    "api.invalid_response" | "api.unsupported_schema" => 502,
                    "api.request_exists" | "api.subscription_exists" | "api.next_pending" => 409,
                    "api.busy" => 429,
                    "api.cancelled"
                    | "api.background"
                    | "api.destroyed"
                    | "api.subscription_closed" => 0,
                    _ => 503,
                };
                BridgeError {
                    code,
                    status,
                    message: "Native module request failed",
                }
            })
    }
    #[cfg(not(target_os = "android"))]
    {
        let _ = (app, command, args);
        Err(BridgeError::unavailable())
    }
}

#[tauri::command]
async fn read<R: Runtime>(
    app: AppHandle<R>,
    request_id: String,
    resource: Resource,
) -> Result<Value, BridgeError> {
    if !valid_id(&request_id) {
        return Err(BridgeError::invalid());
    }
    dispatch(
        app,
        "read",
        json!({"requestId": request_id, "resource": resource}),
    )
    .await
}

#[tauri::command]
async fn service_action<R: Runtime>(
    app: AppHandle<R>,
    request_id: String,
    action: ServiceAction,
) -> Result<Value, BridgeError> {
    if !valid_id(&request_id) {
        return Err(BridgeError::invalid());
    }
    dispatch(
        app,
        "serviceAction",
        json!({"requestId": request_id, "action": action}),
    )
    .await
}

#[tauri::command]
async fn cancel<R: Runtime>(app: AppHandle<R>, request_id: String) -> Result<Value, BridgeError> {
    if !valid_id(&request_id) {
        return Err(BridgeError::invalid());
    }
    dispatch(app, "cancel", json!({"requestId": request_id})).await
}

#[tauri::command]
async fn open_events<R: Runtime>(
    app: AppHandle<R>,
    subscription_id: String,
    topics: Vec<String>,
) -> Result<Value, BridgeError> {
    if !valid_id(&subscription_id) || !valid_topics(&topics) {
        return Err(BridgeError::invalid());
    }
    dispatch(
        app,
        "openEvents",
        json!({"subscriptionId": subscription_id, "topics": topics}),
    )
    .await
}

#[tauri::command]
async fn next_event<R: Runtime>(
    app: AppHandle<R>,
    subscription_id: String,
) -> Result<Value, BridgeError> {
    if !valid_id(&subscription_id) {
        return Err(BridgeError::invalid());
    }
    dispatch(app, "nextEvent", json!({"subscriptionId": subscription_id})).await
}

#[tauri::command]
async fn close_events<R: Runtime>(
    app: AppHandle<R>,
    subscription_id: String,
) -> Result<Value, BridgeError> {
    if !valid_id(&subscription_id) {
        return Err(BridgeError::invalid());
    }
    dispatch(
        app,
        "closeEvents",
        json!({"subscriptionId": subscription_id}),
    )
    .await
}

#[tauri::command]
async fn visibility<R: Runtime>(app: AppHandle<R>) -> Result<Value, BridgeError> {
    dispatch(app, "visibility", json!({})).await
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("module-api")
        .invoke_handler(tauri::generate_handler![
            read,
            service_action,
            cancel,
            open_events,
            next_event,
            close_events,
            visibility
        ])
        .setup(|_app, _api| {
            #[cfg(target_os = "android")]
            _app.manage(Native(_api.register_android_plugin(
                "com.fanjv.netproxy.bridge",
                "ModuleApiPlugin",
            )?));
            Ok(())
        })
        .build()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resources_are_not_arbitrary_paths_or_methods() {
        assert!(serde_json::from_str::<Resource>("\"info\"").is_ok());
        assert!(serde_json::from_str::<Resource>("\"service\"").is_ok());
        for forbidden in [
            "/info",
            "POST",
            "config",
            "auth/provision",
            "https://127.0.0.1",
        ] {
            assert!(serde_json::from_value::<Resource>(json!(forbidden)).is_err());
        }
    }

    #[test]
    fn service_actions_only_accept_start_and_stop() {
        for action in ["start", "stop"] {
            let parsed = serde_json::from_value::<ServiceAction>(json!(action)).unwrap();
            assert_eq!(serde_json::to_value(parsed).unwrap(), json!(action));
        }
        for forbidden in [
            "restart",
            "reload",
            "check",
            "toggle",
            "status",
            "Start",
            " start",
            "stop ",
            "/service/start",
            "start?x=1",
            "POST",
            "https://127.0.0.1",
            "",
        ] {
            assert!(serde_json::from_value::<ServiceAction>(json!(forbidden)).is_err());
        }
        for forbidden in [json!(null), json!(1), json!({"action": "start"})] {
            assert!(serde_json::from_value::<ServiceAction>(forbidden).is_err());
        }
    }

    #[test]
    fn identifiers_and_topics_are_bounded() {
        assert!(valid_id("np-123_abc"));
        for id in ["", "../../secret", "a b", &"x".repeat(97)] {
            assert!(!valid_id(id));
        }
        assert!(valid_topics(&[
            "status".into(),
            "groups".into(),
            "subscriptions".into()
        ]));
        assert!(!valid_topics(&[]));
        assert!(!valid_topics(&["status".into(), "status".into()]));
        assert!(!valid_topics(&["traffic".into()]));
        assert!(!valid_topics(&["auth".into()]));
    }
}
