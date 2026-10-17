use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use std::collections::BTreeMap;
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
    Nodes,
    Selection,
    Mode,
    Catalog,
    Subscriptions,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum SubscriptionResource {
    Detail,
    History,
    Progress,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum SubscriptionAction {
    Update,
    Cancel,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct SubscriptionActionRequest {
    request_id: String,
    group_id: String,
    action: SubscriptionAction,
}

impl SubscriptionActionRequest {
    fn parse(body: &tauri::ipc::InvokeBody) -> Result<Self, BridgeError> {
        let tauri::ipc::InvokeBody::Json(value) = body else {
            return Err(BridgeError::invalid());
        };
        validate_body(value)?;
        let request: Self =
            serde_json::from_value(value.clone()).map_err(|_| BridgeError::invalid())?;
        request.validate()?;
        Ok(request)
    }

    fn validate(&self) -> Result<(), BridgeError> {
        if !valid_id(&self.request_id) || !valid_group_id(&self.group_id) {
            return Err(BridgeError::invalid());
        }
        validate_body(self)
    }
}

#[derive(Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct SubscriptionInput {
    #[serde(skip_serializing_if = "Option::is_none")]
    name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    url: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    user_agent: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    hwid: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    custom_headers: Option<BTreeMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    auto_update: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    update_interval: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    update_via_proxy: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    include: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    exclude: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    allow_insecure: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    download_timeout: Option<i64>,
}

impl SubscriptionInput {
    fn validate(&self, create: bool) -> Result<(), BridgeError> {
        let invalid_text = [
            &self.name,
            &self.url,
            &self.user_agent,
            &self.hwid,
            &self.include,
            &self.exclude,
        ]
        .into_iter()
        .flatten()
        .any(|text| text.chars().any(char::is_control));
        let invalid_url = self.url.as_ref().is_some_and(|value| {
            let prefix = value
                .get(..7)
                .is_some_and(|s| s.eq_ignore_ascii_case("http://"))
                || value
                    .get(..8)
                    .is_some_and(|s| s.eq_ignore_ascii_case("https://"));
            !prefix
                || value.chars().any(char::is_whitespace)
                || tauri::Url::parse(value)
                    .ok()
                    .and_then(|url| url.host_str().map(str::to_owned))
                    .is_none_or(|host| host.is_empty())
        });
        let invalid_headers = self.custom_headers.as_ref().is_some_and(|headers| {
            headers.iter().any(|(name, value)| {
                name.is_empty()
                    || !name
                        .bytes()
                        .all(|b| b.is_ascii_alphanumeric() || b"!#$%&'*+-.^_`|~".contains(&b))
                    || value.chars().any(|c| c != '\t' && c.is_control())
            })
        });
        const MAX_SAFE_INTEGER: i64 = 9_007_199_254_740_991;
        if invalid_text
            || invalid_url
            || invalid_headers
            || (create && self.url.is_none())
            || self
                .name
                .as_ref()
                .is_some_and(|name| name.trim().is_empty())
            || self
                .update_interval
                .is_some_and(|value| !(900..=MAX_SAFE_INTEGER).contains(&value))
            || self
                .download_timeout
                .is_some_and(|value| !(1..=MAX_SAFE_INTEGER).contains(&value))
            || self
                .update_via_proxy
                .as_ref()
                .is_some_and(|value| !matches!(value.as_str(), "auto" | "always" | "never"))
        {
            return Err(BridgeError::invalid());
        }
        Ok(())
    }
}

#[derive(Deserialize, Serialize)]
#[serde(tag = "action", rename_all = "snake_case", deny_unknown_fields)]
enum SubscriptionWriteRequest {
    Add {
        #[serde(rename = "requestId")]
        request_id: String,
        input: SubscriptionInput,
    },
    Edit {
        #[serde(rename = "requestId")]
        request_id: String,
        #[serde(rename = "groupId")]
        group_id: String,
        input: SubscriptionInput,
    },
    Remove {
        #[serde(rename = "requestId")]
        request_id: String,
        #[serde(rename = "groupId")]
        group_id: String,
        #[serde(rename = "replacementGroupId", skip_serializing_if = "Option::is_none")]
        replacement_group_id: Option<String>,
    },
    UpdateAll {
        #[serde(rename = "requestId")]
        request_id: String,
    },
}

impl SubscriptionWriteRequest {
    fn parse(body: &tauri::ipc::InvokeBody) -> Result<Self, BridgeError> {
        let tauri::ipc::InvokeBody::Json(value) = body else {
            return Err(BridgeError::invalid());
        };
        validate_body(value)?;
        // Option preserves omission, but explicit null is never an instruction to clear a field.
        let fields = value.as_object().ok_or_else(BridgeError::invalid)?;
        if fields.values().any(Value::is_null)
            || fields
                .get("input")
                .and_then(Value::as_object)
                .is_some_and(|input| input.values().any(Value::is_null))
        {
            return Err(BridgeError::invalid());
        }
        let request: Self =
            serde_json::from_value(value.clone()).map_err(|_| BridgeError::invalid())?;
        let request_id = match &request {
            Self::Add { request_id, input } => {
                input.validate(true)?;
                request_id
            }
            Self::Edit {
                request_id,
                group_id,
                input,
            } => {
                if !valid_group_id(group_id) {
                    return Err(BridgeError::invalid());
                }
                input.validate(false)?;
                request_id
            }
            Self::Remove {
                request_id,
                group_id,
                replacement_group_id,
            } => {
                if !valid_group_id(group_id)
                    || replacement_group_id
                        .as_ref()
                        .is_some_and(|id| !valid_group_id(id) || id == group_id)
                {
                    return Err(BridgeError::invalid());
                }
                request_id
            }
            Self::UpdateAll { request_id } => request_id,
        };
        if !valid_id(request_id) {
            return Err(BridgeError::invalid());
        }
        Ok(request)
    }
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct NodeTarget {
    group_id: String,
    node_tag: String,
}

const BODY_LIMIT: usize = 4 * 1024 * 1024;

fn valid_group_id(value: &str) -> bool {
    value
        .as_bytes()
        .first()
        .is_some_and(u8::is_ascii_alphanumeric)
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'.' | b'_' | b'-'))
        && value != "staging"
        && !value.contains("..")
}

fn validate_body(value: &impl Serialize) -> Result<(), BridgeError> {
    if serde_json::to_vec(value)
        .map_err(|_| BridgeError::invalid())?
        .len()
        > BODY_LIMIT
    {
        return Err(BridgeError {
            code: "api.body_too_large".into(),
            status: 413,
            message: "Native request exceeds 4 MiB",
        });
    }
    Ok(())
}

impl NodeTarget {
    fn validate(&self) -> Result<(), BridgeError> {
        if !valid_group_id(&self.group_id) {
            return Err(BridgeError::invalid());
        }
        validate_body(self)
    }
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
async fn subscription_read<R: Runtime>(
    app: AppHandle<R>,
    request_id: String,
    group_id: String,
    resource: SubscriptionResource,
) -> Result<Value, BridgeError> {
    if !valid_id(&request_id) || !valid_group_id(&group_id) {
        return Err(BridgeError::invalid());
    }
    validate_body(&group_id)?;
    dispatch(
        app,
        "subscriptionRead",
        json!({
            "requestId": request_id, "groupId": group_id, "resource": resource
        }),
    )
    .await
}

#[tauri::command]
async fn subscription_action<R: Runtime>(
    app: AppHandle<R>,
    request: tauri::ipc::Request<'_>,
) -> Result<Value, BridgeError> {
    let request = SubscriptionActionRequest::parse(request.body())?;
    dispatch(app, "subscriptionAction", json!(request)).await
}

#[tauri::command]
async fn subscription_write<R: Runtime>(
    app: AppHandle<R>,
    request: tauri::ipc::Request<'_>,
) -> Result<Value, BridgeError> {
    let request = SubscriptionWriteRequest::parse(request.body())?;
    dispatch(app, "subscriptionWrite", json!(request)).await
}

#[tauri::command]
async fn select_node<R: Runtime>(
    app: AppHandle<R>,
    request_id: String,
    target: NodeTarget,
) -> Result<Value, BridgeError> {
    if !valid_id(&request_id) {
        return Err(BridgeError::invalid());
    }
    target.validate()?;
    dispatch(
        app,
        "selectNode",
        json!({"requestId": request_id, "target": target}),
    )
    .await
}

#[tauri::command]
async fn change_mode<R: Runtime>(
    app: AppHandle<R>,
    request_id: String,
    mode: String,
) -> Result<Value, BridgeError> {
    if !valid_id(&request_id) || mode.trim().is_empty() {
        return Err(BridgeError::invalid());
    }
    validate_body(&json!({"mode": mode}))?;
    dispatch(
        app,
        "changeMode",
        json!({"requestId": request_id, "mode": mode}),
    )
    .await
}

#[tauri::command]
async fn test_delay<R: Runtime>(
    app: AppHandle<R>,
    request_id: String,
    target: NodeTarget,
) -> Result<Value, BridgeError> {
    if !valid_id(&request_id) {
        return Err(BridgeError::invalid());
    }
    target.validate()?;
    dispatch(
        app,
        "testDelay",
        json!({"requestId": request_id, "target": target}),
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
            subscription_read,
            subscription_action,
            subscription_write,
            service_action,
            select_node,
            change_mode,
            test_delay,
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

    fn write_request(value: Value) -> Result<SubscriptionWriteRequest, BridgeError> {
        SubscriptionWriteRequest::parse(&tauri::ipc::InvokeBody::Json(value))
    }

    #[test]
    fn subscription_writes_are_flat_tagged_and_preserve_omission_and_explicit_clearing() {
        for value in [
            json!({"action":"add","requestId":"r","input":{"url":"https://example.invalid/feed"}}),
            json!({"action":"add","requestId":"r","input":{"name":"feed","url":"HTTP://127.0.0.1/feed","user_agent":"","hwid":"","custom_headers":{"X-Token":"a\tb"},"auto_update":false,"update_interval":900,"update_via_proxy":"always","include":"","exclude":"","allow_insecure":false,"download_timeout":1}}),
            json!({"action":"edit","requestId":"r","groupId":"sub-a","input":{}}),
            json!({"action":"edit","requestId":"r","groupId":"sub-a","input":{"user_agent":"","hwid":"","custom_headers":{},"include":"","exclude":""}}),
            json!({"action":"remove","requestId":"r","groupId":"sub-a"}),
            json!({"action":"remove","requestId":"r","groupId":"sub-a","replacementGroupId":"default"}),
            json!({"action":"update_all","requestId":"r"}),
        ] {
            let request = write_request(value.clone())
                .ok()
                .expect("Valid subscription write");
            assert_eq!(serde_json::to_value(request).unwrap(), value);
        }
    }

    #[test]
    fn subscription_write_envelopes_reject_extra_null_wrong_types_and_unsafe_ids() {
        let operations = [
            json!({"action":"add","requestId":"r","input":{"url":"https://example.invalid/feed"}}),
            json!({"action":"edit","requestId":"r","groupId":"sub-a","input":{}}),
            json!({"action":"remove","requestId":"r","groupId":"sub-a"}),
            json!({"action":"update_all","requestId":"r"}),
        ];
        for value in &operations {
            for key in [
                "method", "path", "url", "query", "body", "headers", "private", "unknown",
            ] {
                let mut invalid = value.clone();
                invalid[key] = json!(null);
                assert!(write_request(invalid).is_err());
            }
            for key in value.as_object().unwrap().keys() {
                let mut invalid = value.clone();
                invalid[key] = Value::Null;
                assert!(write_request(invalid).is_err());
                let mut missing = value.clone();
                missing.as_object_mut().unwrap().remove(key);
                assert!(write_request(missing).is_err());
            }
            let mut invalid = value.clone();
            invalid["requestId"] = json!(123);
            assert!(write_request(invalid).is_err());
        }
        for value in [
            json!({"action":"update_all","requestId":"r","groupId":"default"}),
            json!({"action":"remove","requestId":"r","groupId":"sub-a","input":{}}),
            json!({"action":"add","requestId":"r","groupId":"sub-a","input":{"url":"https://example.invalid"}}),
            json!({"action":"edit","requestId":"r","groupId":"sub-a","input":{},"replacementGroupId":"default"}),
            json!({"action":"remove","requestId":"r","groupId":"sub-a","replacementGroupId":null}),
            json!({"action":"remove","requestId":"r","groupId":"sub-a","replacementGroupId":"sub-a"}),
        ] {
            assert!(write_request(value).is_err());
        }
        for action in [
            "update",
            "cancel",
            "update-all",
            "delete",
            "POST",
            "private",
            "",
        ] {
            assert!(write_request(json!({"action":action,"requestId":"r"})).is_err());
        }
        for group in ["", "staging", "a..b", "a/b", "a?x=1", "a%2Fb", "_a"] {
            assert!(
                write_request(json!({"action":"edit","requestId":"r","groupId":group,"input":{}}))
                    .is_err()
            );
            assert!(write_request(json!({"action":"remove","requestId":"r","groupId":"a","replacementGroupId":group})).is_err());
        }
    }

    #[test]
    fn subscription_inputs_reject_null_unknown_fields_and_invalid_values_before_dispatch() {
        for key in [
            "name",
            "url",
            "user_agent",
            "hwid",
            "custom_headers",
            "auto_update",
            "update_interval",
            "update_via_proxy",
            "include",
            "exclude",
            "allow_insecure",
            "download_timeout",
        ] {
            assert!(
                write_request(
                    json!({"action":"edit","requestId":"r","groupId":"sub-a","input":{key:null}})
                )
                .is_err()
            );
        }
        for input in [
            json!({"unknown":true}),
            json!({"name":" "}),
            json!({"name":2}),
            json!({"url":""}),
            json!({"url":"ftp://example.invalid"}),
            json!({"url":"https:///"}),
            json!({"url":"https://example.invalid/a b"}),
            json!({"url":"https://example.invalid/\t"}),
            json!({"user_agent":"a\nb"}),
            json!({"hwid":"a\tb"}),
            json!({"include":"a\rb"}),
            json!({"exclude":"a\u{7f}b"}),
            json!({"auto_update":"false"}),
            json!({"allow_insecure":1}),
            json!({"update_interval":899}),
            json!({"update_interval":"900"}),
            json!({"update_interval":900.5}),
            json!({"download_timeout":0}),
            json!({"download_timeout":-1}),
            json!({"download_timeout":9007199254740992_i64}),
            json!({"update_via_proxy":"direct"}),
            json!({"custom_headers":[]}),
            json!({"custom_headers":{"":"v"}}),
            json!({"custom_headers":{"Bad Header":"v"}}),
            json!({"custom_headers":{"X-Test":"a\rb"}}),
            json!({"custom_headers":{"X-Test":null}}),
            json!({"custom_headers":{"X-Test":false}}),
        ] {
            assert!(
                write_request(
                    json!({"action":"edit","requestId":"r","groupId":"sub-a","input":input})
                )
                .is_err()
            );
        }
        assert!(write_request(json!({"action":"add","requestId":"r","input":{}})).is_err());
        assert_eq!(write_request(json!({"action":"add","requestId":"r","input":{"url":"https://example.invalid","user_agent":"x".repeat(BODY_LIMIT)}})).err().unwrap().status, 413);
    }

    #[test]
    fn subscription_actions_only_accept_typed_update_and_cancel_without_extra_fields() {
        for action in ["update", "cancel"] {
            let value = json!({"requestId":"request-1","groupId":"a.b-C_1","action":action});
            let request: SubscriptionActionRequest = serde_json::from_value(value.clone()).unwrap();
            request.validate().unwrap();
            assert_eq!(serde_json::to_value(request).unwrap(), value);
            SubscriptionActionRequest::parse(&tauri::ipc::InvokeBody::Json(value)).unwrap();
        }
        for action in [
            "add",
            "edit",
            "delete",
            "update-all",
            "POST",
            "Update",
            "/update",
            "update?x=1",
        ] {
            assert!(
                serde_json::from_value::<SubscriptionActionRequest>(json!({
                    "requestId":"request-1","groupId":"default","action":action
                }))
                .is_err()
            );
        }
        for field in [
            "method", "path", "query", "body", "headers", "url", "private",
        ] {
            let mut value = json!({"requestId":"request-1","groupId":"default","action":"update"});
            value[field] = json!({});
            assert!(serde_json::from_value::<SubscriptionActionRequest>(value.clone()).is_err());
            assert!(
                SubscriptionActionRequest::parse(&tauri::ipc::InvokeBody::Json(value)).is_err()
            );
        }
        assert!(SubscriptionActionRequest::parse(&tauri::ipc::InvokeBody::Raw(vec![])).is_err());
        assert!(
            SubscriptionActionRequest::parse(&tauri::ipc::InvokeBody::Json(json!({
                "request": {"requestId":"request-1","groupId":"default","action":"update"}
            })))
            .is_err()
        );
        for value in [
            json!({"requestId":"request-1","groupId":"default"}),
            json!({"requestId":"request-1","groupId":null,"action":"update"}),
            json!({"requestId":1,"groupId":"default","action":"cancel"}),
            json!({"requestId":"request-1","groupId":"default","action":{}}),
        ] {
            assert!(serde_json::from_value::<SubscriptionActionRequest>(value).is_err());
        }
        for group_id in [
            "",
            "staging",
            "a..b",
            "a/b",
            "a%2fb",
            "a?private=true",
            "_a",
            "中文",
        ] {
            assert!(
                SubscriptionActionRequest {
                    request_id: "valid".into(),
                    group_id: group_id.into(),
                    action: SubscriptionAction::Update,
                }
                .validate()
                .is_err()
            );
        }
        for request_id in ["", "bad/id", &"x".repeat(97)] {
            assert!(
                SubscriptionActionRequest {
                    request_id: request_id.into(),
                    group_id: "default".into(),
                    action: SubscriptionAction::Cancel,
                }
                .validate()
                .is_err()
            );
        }
        assert_eq!(
            SubscriptionActionRequest {
                request_id: "valid".into(),
                group_id: "a".repeat(BODY_LIMIT),
                action: SubscriptionAction::Update,
            }
            .validate()
            .unwrap_err()
            .status,
            413
        );
    }

    #[test]
    fn resources_are_not_arbitrary_paths_or_methods() {
        assert!(serde_json::from_str::<Resource>("\"info\"").is_ok());
        assert!(serde_json::from_str::<Resource>("\"service\"").is_ok());
        for resource in ["nodes", "selection", "mode", "catalog", "subscriptions"] {
            assert!(serde_json::from_value::<Resource>(json!(resource)).is_ok());
        }
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
    fn subscription_resources_are_public_readonly_and_group_ids_cannot_change_paths() {
        for resource in ["detail", "history", "progress"] {
            assert!(serde_json::from_value::<SubscriptionResource>(json!(resource)).is_ok());
        }
        for forbidden in [
            "private",
            "detail?private=true",
            "update",
            "update-all",
            "cancel",
            "add",
            "edit",
            "delete",
            "POST",
            "/subscriptions/default",
            "https://localhost",
        ] {
            assert!(serde_json::from_value::<SubscriptionResource>(json!(forbidden)).is_err());
        }
        for group in ["default", "a.b-C_1"] {
            assert!(valid_group_id(group));
        }
        for group in [
            "",
            "staging",
            "..",
            "a..b",
            "a/b",
            "a%2fb",
            "a?private=true",
            "a#b",
            "a\\b",
            "_a",
            "中文",
        ] {
            assert!(!valid_group_id(group));
        }
    }

    #[test]
    fn node_targets_reject_unsafe_groups_and_unknown_fields_but_preserve_original_tags() {
        for group in ["default", "a.b-C_1", "012"] {
            for tag in ["", "香港 / 节点🚀", "Auto/default", " name "] {
                let target: NodeTarget =
                    serde_json::from_value(json!({"group_id":group,"node_tag":tag})).unwrap();
                target.validate().unwrap();
                assert_eq!(target.node_tag, tag);
            }
        }
        for group in [
            "",
            ".",
            "..",
            "staging",
            "a..b",
            "../default",
            "a/b",
            "a\\b",
            "a?x",
            "中文",
            " a",
            "_a",
        ] {
            assert!(
                NodeTarget {
                    group_id: group.into(),
                    node_tag: "".into()
                }
                .validate()
                .is_err()
            );
        }
        for target in [
            json!({"group_id":"default"}),
            json!({"group_id":"default","node_tag":null}),
            json!({"group_id":"default","node_tag":2}),
            json!({"group_id":"default","node_tag":"","path":"/config"}),
        ] {
            assert!(serde_json::from_value::<NodeTarget>(target).is_err());
        }
        let target = NodeTarget {
            group_id: "default".into(),
            node_tag: "龙".repeat(BODY_LIMIT / 3),
        };
        assert_eq!(target.validate().unwrap_err().status, 413);
        // JSON 转义也计入正文上限，不能只检查字符串字符数。
        assert_eq!(
            validate_body(&json!({"mode":"\n".repeat(BODY_LIMIT / 2)}))
                .unwrap_err()
                .status,
            413
        );
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
