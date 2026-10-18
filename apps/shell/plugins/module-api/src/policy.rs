use crate::{BridgeError, valid_id, validate_body};
use serde::{Deserialize, Serialize, de::DeserializeOwned};

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct Select {
    request_id: String,
    group_tag: String,
    member_tag: String,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct Delay {
    request_id: String,
    group_tag: String,
}

fn parse<T: DeserializeOwned>(body: &tauri::ipc::InvokeBody) -> Result<T, BridgeError> {
    let tauri::ipc::InvokeBody::Json(value) = body else {
        return Err(BridgeError::invalid());
    };
    validate_body(value)?;
    serde_json::from_value(value.clone()).map_err(|_| BridgeError::invalid())
}

fn valid_tag(tag: &str) -> bool {
    !tag.trim().is_empty() && !tag.chars().any(char::is_control)
}

impl Select {
    pub(crate) fn parse(body: &tauri::ipc::InvokeBody) -> Result<Self, BridgeError> {
        let request: Self = parse(body)?;
        if !valid_id(&request.request_id)
            || !valid_tag(&request.group_tag)
            || !valid_tag(&request.member_tag)
        {
            return Err(BridgeError::invalid());
        }
        Ok(request)
    }
}

impl Delay {
    pub(crate) fn parse(body: &tauri::ipc::InvokeBody) -> Result<Self, BridgeError> {
        let request: Self = parse(body)?;
        if !valid_id(&request.request_id) || !valid_tag(&request.group_tag) {
            return Err(BridgeError::invalid());
        }
        Ok(request)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use tauri::ipc::InvokeBody::Json;

    #[test]
    fn policy_requests_preserve_tags_and_reject_extra_missing_or_invalid_fields() {
        for tag in [
            "Proxy",
            " Auto/group ",
            "\u{9999}\u{6e2f}/\u{1f680}",
            "../tag?x=1",
        ] {
            let value = json!({"requestId":"r","groupTag":tag,"memberTag":tag});
            assert_eq!(
                serde_json::to_value(Select::parse(&Json(value.clone())).unwrap()).unwrap(),
                value
            );
            Delay::parse(&Json(json!({"requestId":"r","groupTag":tag}))).unwrap();
        }
        for value in [
            json!(null),
            json!({}),
            json!({"requestId":"r","groupTag":"Proxy"}),
            json!({"requestId":"r","groupTag":"Proxy","memberTag":null}),
            json!({"requestId":"r","groupTag":1,"memberTag":"a"}),
            json!({"requestId":"bad/id","groupTag":"Proxy","memberTag":"a"}),
        ] {
            assert!(Select::parse(&Json(value)).is_err());
        }
        for key in [
            "method", "path", "body", "query", "headers", "url", "target",
        ] {
            let mut value = json!({"requestId":"r","groupTag":"Proxy","memberTag":"a"});
            value[key] = json!("x");
            assert!(Select::parse(&Json(value)).is_err());
        }
        for tag in ["", " ", "\u{3000}", "a\n", "\0", "\u{7f}", "\u{85}"] {
            assert!(
                Select::parse(&Json(
                    json!({"requestId":"r","groupTag":"Proxy","memberTag":tag})
                ))
                .is_err()
            );
            assert!(Delay::parse(&Json(json!({"requestId":"r","groupTag":tag}))).is_err());
        }
        assert!(
            Delay::parse(&Json(
                json!({"requestId":"r","groupTag":"Proxy","memberTag":"a"})
            ))
            .is_err()
        );
        assert!(Select::parse(&tauri::ipc::InvokeBody::Raw(vec![])).is_err());
        assert_eq!(
            Delay::parse(&Json(
                json!({"requestId":"r","groupTag":"x".repeat(crate::BODY_LIMIT)})
            ))
            .unwrap_err()
            .status,
            413
        );
        assert!(
            serde_json::from_str::<Select>(
                r#"{"requestId":"r","groupTag":"\ud800","memberTag":"a"}"#
            )
            .is_err()
        );
    }

    #[test]
    fn policy_permissions_replace_legacy_commands_and_both_envelopes_are_strict() {
        let permissions = include_str!("../permissions/default.toml");
        for name in ["allow-policy-select", "allow-policy-delay"] {
            assert!(permissions.contains(name));
        }
        for name in ["allow-select-node", "allow-test-delay"] {
            assert!(!permissions.contains(name));
        }
        for select in [false, true] {
            let mut value = json!({"requestId":"r","groupTag":"Proxy"});
            if select {
                value["memberTag"] = json!("member");
            }
            let accepted = |value| {
                let body = Json(value);
                if select {
                    Select::parse(&body).is_ok()
                } else {
                    Delay::parse(&body).is_ok()
                }
            };
            assert!(accepted(value.clone()));
            for key in value.as_object().unwrap().keys() {
                let mut missing = value.clone();
                missing.as_object_mut().unwrap().remove(key);
                assert!(!accepted(missing));
                for item in [json!(null), json!(1), json!(true), json!({}), json!([])] {
                    let mut invalid = value.clone();
                    invalid[key] = item;
                    assert!(!accepted(invalid));
                }
            }
            for key in [
                "method", "path", "url", "headers", "query", "body", "target",
            ] {
                let mut invalid = value.clone();
                invalid[key] = json!("x");
                assert!(!accepted(invalid));
            }
            for id in ["", "bad/id", &"x".repeat(97)] {
                let mut invalid = value.clone();
                invalid["requestId"] = json!(id);
                assert!(!accepted(invalid));
            }
            let mut invalid = value.clone();
            invalid["groupTag"] = json!("\u{9f99}".repeat(crate::BODY_LIMIT / 3));
            assert!(!accepted(invalid));
        }
    }

    #[test]
    fn policy_requests_accept_exactly_four_mib_and_reject_the_next_byte() {
        for select in [false, true] {
            let mut value = json!({"requestId":"r","groupTag":""});
            if select {
                value["memberTag"] = json!("a");
            }
            let overhead = serde_json::to_vec(&value).unwrap().len();
            value["groupTag"] = json!("x".repeat(crate::BODY_LIMIT - overhead));
            let accepted = |body| {
                if select {
                    Select::parse(&body).map(|_| ())
                } else {
                    Delay::parse(&body).map(|_| ())
                }
            };
            assert_eq!(serde_json::to_vec(&value).unwrap().len(), crate::BODY_LIMIT);
            accepted(Json(value.clone())).unwrap();
            value["groupTag"] = json!("x".repeat(crate::BODY_LIMIT - overhead + 1));
            assert_eq!(accepted(Json(value)).unwrap_err().status, 413);
        }
    }
}
