use serde::{Deserialize, Serialize};
use std::env;
use std::fs;

#[derive(Deserialize)]
struct Event {
    name: Option<String>,
}

#[derive(Serialize)]
struct Response {
    message: String,
    runtime: String,
}

fn main() {
    let input_file = env::args()
        .nth(1)
        .unwrap_or_else(|| "/tmp/input.json".to_string());

    let data = fs::read_to_string(&input_file).unwrap_or_else(|_| "{}".to_string());
    let event: Event = serde_json::from_str(&data).unwrap_or(Event { name: None });

    let name = event.name.unwrap_or_else(|| "Anonymous".to_string());
    let resp = Response {
        message: format!("Hello, {}!", name),
        runtime: "wasm".to_string(),
    };

    let out = serde_json::to_string(&resp).expect("serialize output");
    println!("{}", out);
}

