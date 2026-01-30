use serde::{Deserialize, Serialize};
use std::env;
use std::fs;
use std::thread;
use std::time::{Duration, Instant};

#[derive(Deserialize)]
struct Event {
    sleep_seconds: Option<u64>,
}

#[derive(Serialize)]
struct Response {
    requested_sleep: u64,
    actual_sleep: f64,
    status: String,
}

fn handler(event: Event) -> Response {
    let sleep_sec = event.sleep_seconds.unwrap_or(5);

    let start = Instant::now();
    thread::sleep(Duration::from_secs(sleep_sec));
    let elapsed = start.elapsed().as_secs_f64();

    Response {
        requested_sleep: sleep_sec,
        actual_sleep: (elapsed * 100.0).round() / 100.0,
        status: "completed".to_string(),
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
