#!/usr/bin/env python3
"""Timeout test function - sleeps for specified duration"""

import json
import sys
import time


def handler(event):
    sleep_seconds = event.get("sleep_seconds", 5)

    start = time.time()
    time.sleep(sleep_seconds)
    elapsed = time.time() - start

    return {
        "requested_sleep": sleep_seconds,
        "actual_sleep": round(elapsed, 2),
        "status": "completed",
    }


if __name__ == "__main__":
    input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"
    with open(input_file) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))
