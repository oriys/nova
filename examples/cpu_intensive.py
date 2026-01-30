#!/usr/bin/env python3
"""CPU intensive function - calculate prime numbers"""

import json
import sys
import time


def is_prime(n):
    if n < 2:
        return False
    for i in range(2, int(n**0.5) + 1):
        if n % i == 0:
            return False
    return True


def handler(event):
    limit = event.get("limit", 10000)
    start = time.time()

    primes = []
    for n in range(2, limit + 1):
        if is_prime(n):
            primes.append(n)

    elapsed_ms = int((time.time() - start) * 1000)

    return {
        "limit": limit,
        "count": len(primes),
        "last_10": primes[-10:] if len(primes) >= 10 else primes,
        "elapsed_ms": elapsed_ms,
    }


if __name__ == "__main__":
    input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"
    with open(input_file) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))
