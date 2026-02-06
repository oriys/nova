use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Deserialize)]
struct Event {
    name: Option<String>,
}

#[derive(Serialize)]
struct Response {
    message: String,
    runtime: String,
    request_id: String,
}

pub fn handler(event: Value, ctx: crate::context::Context) -> Result<Value, String> {
    let e: Event = serde_json::from_value(event).map_err(|e| e.to_string())?;
    let name = e.name.unwrap_or_else(|| "Anonymous".to_string());
    let resp = Response {
        message: format!("Hello, {}!", name),
        runtime: "rust".to_string(),
        request_id: ctx.request_id,
    };
    serde_json::to_value(&resp).map_err(|e| e.to_string())
}
