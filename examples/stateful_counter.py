"""
Stateful counter function using persistent mode.

The counter state is maintained in memory across invocations within the same VM.
Supports increment, decrement, get, and reset operations.

Usage:
  nova register stateful-counter \
    --runtime python \
    --code examples/stateful_counter.py \
    --mode persistent

  nova invoke stateful-counter '{"action": "increment"}'
  nova invoke stateful-counter '{"action": "increment", "step": 5}'
  nova invoke stateful-counter '{"action": "get"}'
  nova invoke stateful-counter '{"action": "decrement"}'
  nova invoke stateful-counter '{"action": "reset"}'
"""

_counter = 0
_invocations = 0


def handler(event, context):
    global _counter, _invocations
    _invocations += 1

    action = event.get("action", "increment")
    step = event.get("step", 1)

    if action == "increment":
        _counter += step
    elif action == "decrement":
        _counter -= step
    elif action == "reset":
        _counter = 0
    elif action == "get":
        pass
    else:
        return {"error": f"Unknown action: {action}"}

    return {
        "counter": _counter,
        "action": action,
        "invocations": _invocations,
        "request_id": context.request_id,
    }
