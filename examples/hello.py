#!/usr/bin/env python3
"""Example Python function for Nova serverless platform"""

import json
import sys

def handler(event):
    """
    Main handler function
    Args:
        event: dict with input data
    Returns:
        dict with response
    """
    name = event.get("name", "Anonymous")
    return {
        "message": f"Hello, {name}!",
        "runtime": "python"
    }

if __name__ == "__main__":
    # Read input from file (passed as argument)
    input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"

    with open(input_file, "r") as f:
        event = json.load(f)

    result = handler(event)
    print(json.dumps(result))
