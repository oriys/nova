"""Disk I/O test function - write and read temporary files"""

import time
import os


def handler(event, context):
    size_kb = event.get("size_kb", 1024)
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
        start = time.time()
        with open(test_file, "wb") as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        results["write_times_ms"].append(int((time.time() - start) * 1000))

        start = time.time()
        with open(test_file, "rb") as f:
            _ = f.read()
        results["read_times_ms"].append(int((time.time() - start) * 1000))

    os.remove(test_file)

    results["avg_write_ms"] = sum(results["write_times_ms"]) / iterations
    results["avg_read_ms"] = sum(results["read_times_ms"]) / iterations
    results["write_throughput_mbps"] = round(size_kb / 1024 / (results["avg_write_ms"] / 1000), 2)
    results["read_throughput_mbps"] = round(size_kb / 1024 / (results["avg_read_ms"] / 1000), 2)

    return results
