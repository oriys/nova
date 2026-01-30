#!/usr/bin/env python3
"""Disk I/O test function - write and read temporary files"""

import json
import sys
import time
import os


def handler(event):
    size_kb = event.get("size_kb", 1024)  # Default 1MB
    iterations = event.get("iterations", 10)

    data = b"x" * (size_kb * 1024)
    test_file = "/tmp/disk_test.bin"

    results = {
        "size_kb": size_kb,
        "iterations": iterations,
        "write_times_ms": [],
        "read_times_ms": [],
    }

    for i in range(iterations):
        # Write test
        start = time.time()
        with open(test_file, "wb") as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        results["write_times_ms"].append(int((time.time() - start) * 1000))

        # Read test
        start = time.time()
        with open(test_file, "rb") as f:
            _ = f.read()
        results["read_times_ms"].append(int((time.time() - start) * 1000))

    # Cleanup
    os.remove(test_file)

    # Calculate averages
    results["avg_write_ms"] = sum(results["write_times_ms"]) / iterations
    results["avg_read_ms"] = sum(results["read_times_ms"]) / iterations
    results["write_throughput_mbps"] = round(size_kb / 1024 / (results["avg_write_ms"] / 1000), 2)
    results["read_throughput_mbps"] = round(size_kb / 1024 / (results["avg_read_ms"] / 1000), 2)

    return results


if __name__ == "__main__":
    input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"
    with open(input_file) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))
