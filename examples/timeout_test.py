"""Timeout test function - sleeps for specified duration"""

import time


def handler(event, context):
    sleep_seconds = event.get("sleep_seconds", 5)

    start = time.time()
    time.sleep(sleep_seconds)
    elapsed = time.time() - start

    return {
        "requested_sleep": sleep_seconds,
        "actual_sleep": round(elapsed, 2),
        "status": "completed",
    }
