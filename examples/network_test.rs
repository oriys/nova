use serde::{Deserialize, Serialize};
use std::env;
use std::fs;
use std::io::Read;
use std::time::{Duration, Instant};

#[derive(Deserialize)]
struct Event {
    url: Option<String>,
    timeout: Option<u64>,
}

#[derive(Serialize)]
struct Response {
    url: String,
    status: u16,
    elapsed_ms: u128,
    response: serde_json::Value,
}

fn handler(event: Event) -> Response {
    let url = event.url.unwrap_or_else(|| "https://httpbin.org/get".to_string());
    let timeout = event.timeout.unwrap_or(10);

    let start = Instant::now();

    // Note: This requires ureq crate. For minimal example without dependencies,
    // we'll use a simple approach. In production, use reqwest or ureq.
    // This example assumes the binary is compiled with network support.

    match ureq::AgentBuilder::new()
        .timeout(Duration::from_secs(timeout))
        .build()
        .get(&url)
        .set("User-Agent", "Nova/1.0")
        .call()
    {
        Ok(resp) => {
            let status = resp.status();
            let body = resp.into_string().unwrap_or_default();
            let elapsed = start.elapsed().as_millis();

            let response: serde_json::Value = serde_json::from_str(&body)
                .unwrap_or_else(|_| serde_json::Value::String(body.chars().take(500).collect()));

            Response {
                url,
                status,
                elapsed_ms: elapsed,
                response,
            }
        }
        Err(e) => {
            let elapsed = start.elapsed().as_millis();
            Response {
                url,
                status: 0,
                elapsed_ms: elapsed,
                response: serde_json::json!({"error": e.to_string()}),
            }
        }
    }
}

fn main() {
    let args: Vec<String> = env::args().collect();
    let input_file = args.get(1).map(|s| s.as_str()).unwrap_or("/tmp/input.json");

    let data = fs::read_to_string(input_file).expect("Failed to read input");
    let event: Event = serde_json::from_str(&data).expect("Failed to parse input");

    let result = handler(event);
    println!("{}", serde_json::to_string(&result).unwrap());
}
