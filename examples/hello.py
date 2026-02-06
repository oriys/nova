"""Example Python function for Nova serverless platform"""


def handler(event, context):
    name = event.get("name", "Anonymous")
    return {
        "message": f"Hello, {name}!",
        "runtime": "python",
        "request_id": context.get("request_id", ""),
    }
