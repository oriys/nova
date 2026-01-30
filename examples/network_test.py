#!/usr/bin/env python3
"""Network test function - fetch external URL"""

import json
import sys
import time
import urllib.request
import urllib.error


def handler(event):
    url = event.get("url", "https://httpbin.org/get")
    timeout = event.get("timeout", 10)

    start = time.time()
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "Nova/1.0"})
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            status = resp.status
            body = resp.read().decode("utf-8")
            # Try to parse as JSON, fallback to raw
            try:
                data = json.loads(body)
            except:
                data = body[:500]  # Truncate if too long
    except urllib.error.HTTPError as e:
        status = e.code
        data = {"error": str(e)}
    except urllib.error.URLError as e:
        status = 0
        data = {"error": str(e.reason)}
    except Exception as e:
        status = 0
        data = {"error": str(e)}

    elapsed_ms = int((time.time() - start) * 1000)

    return {
        "url": url,
        "status": status,
        "elapsed_ms": elapsed_ms,
        "response": data,
    }


if __name__ == "__main__":
    input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"
    with open(input_file) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))
