#!/usr/bin/env python3
"""RSA key generation test for VM capacity testing"""

import json
import sys
import time

def generate_prime(bits=512):
    """Generate a probable prime number using simple method"""
    import random

    def is_probable_prime(n, k=10):
        if n < 2:
            return False
        if n == 2 or n == 3:
            return True
        if n % 2 == 0:
            return False

        # Miller-Rabin primality test
        r, d = 0, n - 1
        while d % 2 == 0:
            r += 1
            d //= 2

        for _ in range(k):
            a = random.randrange(2, n - 1)
            x = pow(a, d, n)
            if x == 1 or x == n - 1:
                continue
            for _ in range(r - 1):
                x = pow(x, 2, n)
                if x == n - 1:
                    break
            else:
                return False
        return True

    while True:
        candidate = random.getrandbits(bits) | (1 << bits - 1) | 1
        if is_probable_prime(candidate):
            return candidate

def handler(event):
    bits = event.get("bits", 512)
    start = time.time()

    # Generate two primes
    p = generate_prime(bits // 2)
    q = generate_prime(bits // 2)

    # Compute RSA modulus
    n = p * q
    phi = (p - 1) * (q - 1)

    # Common public exponent
    e = 65537

    # Compute private exponent
    def mod_inverse(a, m):
        def extended_gcd(a, b):
            if a == 0:
                return b, 0, 1
            gcd, x1, y1 = extended_gcd(b % a, a)
            return gcd, y1 - (b // a) * x1, x1
        _, x, _ = extended_gcd(a % m, m)
        return (x % m + m) % m

    d = mod_inverse(e, phi)

    elapsed = time.time() - start

    return {
        "bits": bits,
        "modulus_bits": n.bit_length(),
        "elapsed_ms": int(elapsed * 1000),
        "success": True
    }

if __name__ == "__main__":
    input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"

    with open(input_file, "r") as f:
        event = json.load(f)

    result = handler(event)
    print(json.dumps(result))
