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

fn handler(event: Event) -> Response {
    let name = event.name.unwrap_or_else(|| "Anonymous".to_string());
    Response {
        message: format!("Hello, {}!", name),
        runtime: "rust".to_string(),
    }
}

fn main() {
    let args: Vec<String> = env::args().collect();
    let input_file = args.get(1).map(|s| s.as_str()).unwrap_or("/tmp/input.json");

    let data = fs::read_to_string(input_file).expect("Failed to read input file");
    let event: Event = serde_json::from_str(&data).expect("Failed to parse input");

    let result = handler(event);
    let output = serde_json::to_string(&result).expect("Failed to serialize output");
    println!("{}", output);
}
