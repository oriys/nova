use serde::{Deserialize, Serialize};
use std::env;
use std::fs::{self, File};
use std::io::{Read, Write};
use std::time::Instant;

#[derive(Deserialize)]
struct Event {
    size_kb: Option<usize>,
    iterations: Option<usize>,
}

#[derive(Serialize)]
struct Response {
    size_kb: usize,
    iterations: usize,
    write_times_ms: Vec<u128>,
    read_times_ms: Vec<u128>,
    avg_write_ms: f64,
    avg_read_ms: f64,
    write_throughput_mbps: f64,
    read_throughput_mbps: f64,
}

fn handler(event: Event) -> Response {
    let size_kb = event.size_kb.unwrap_or(1024);
    let iterations = event.iterations.unwrap_or(10);

    let data = vec![b'x'; size_kb * 1024];
    let test_file = "/tmp/disk_test.bin";

    let mut write_times = Vec::with_capacity(iterations);
    let mut read_times = Vec::with_capacity(iterations);

    for _ in 0..iterations {
        // Write test
        let start = Instant::now();
        {
            let mut f = File::create(test_file).unwrap();
            f.write_all(&data).unwrap();
            f.sync_all().unwrap();
        }
        write_times.push(start.elapsed().as_millis());

        // Read test
        let start = Instant::now();
        {
            let mut f = File::open(test_file).unwrap();
            let mut buf = Vec::new();
            f.read_to_end(&mut buf).unwrap();
        }
        read_times.push(start.elapsed().as_millis());
    }

    // Cleanup
    let _ = fs::remove_file(test_file);

    // Calculate averages
    let total_write: u128 = write_times.iter().sum();
    let total_read: u128 = read_times.iter().sum();
    let avg_write = total_write as f64 / iterations as f64;
    let avg_read = total_read as f64 / iterations as f64;

    let write_throughput = if avg_write > 0.0 {
        (size_kb as f64 / 1024.0) / (avg_write / 1000.0)
    } else {
        0.0
    };
    let read_throughput = if avg_read > 0.0 {
        (size_kb as f64 / 1024.0) / (avg_read / 1000.0)
    } else {
        0.0
    };

    Response {
        size_kb,
        iterations,
        write_times_ms: write_times,
        read_times_ms: read_times,
        avg_write_ms: avg_write,
        avg_read_ms: avg_read,
        write_throughput_mbps: (write_throughput * 100.0).round() / 100.0,
        read_throughput_mbps: (read_throughput * 100.0).round() / 100.0,
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
