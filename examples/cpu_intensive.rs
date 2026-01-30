use serde::{Deserialize, Serialize};
use std::env;
use std::fs;
use std::time::Instant;

#[derive(Deserialize)]
struct Event {
    limit: Option<i32>,
}

#[derive(Serialize)]
struct Response {
    limit: i32,
    count: usize,
    last_10: Vec<i32>,
    elapsed_ms: u128,
}

fn is_prime(n: i32) -> bool {
    if n < 2 {
        return false;
    }
    let sqrt = (n as f64).sqrt() as i32;
    for i in 2..=sqrt {
        if n % i == 0 {
            return false;
        }
    }
    true
}

fn handler(event: Event) -> Response {
    let limit = event.limit.unwrap_or(10000);
    let start = Instant::now();

    let primes: Vec<i32> = (2..=limit).filter(|&n| is_prime(n)).collect();
    let elapsed = start.elapsed().as_millis();

    let last_10: Vec<i32> = if primes.len() >= 10 {
        primes[primes.len() - 10..].to_vec()
    } else {
        primes.clone()
    };

    Response {
        limit,
        count: primes.len(),
        last_10,
        elapsed_ms: elapsed,
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
