"""Example Python function for Nova (AWS Lambda-compatible signature)"""


def handler(event, context):
    name = event.get("name", "Anonymous")
    return {
        "message": f"Hello, {name}!",
        "runtime": "python",
        "request_id": context.request_id,
    }
