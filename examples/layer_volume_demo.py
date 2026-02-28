"""
Demo function using dependency layers and persistent volumes.

This function uses:
  - A dependency layer providing the `yaml` library (pyyaml)
  - A persistent volume mounted at /mnt/data for durable key-value storage

Supported actions:
  set   - Store a key/value pair to the persistent volume
  get   - Read a key from the persistent volume
  list  - List all stored keys
  clear - Remove all stored data

Setup:

  # 1. Create a dependency layer with pyyaml
  curl -X POST http://localhost:9000/layers -H 'Content-Type: application/json' -d '{
    "name": "pyyaml-layer",
    "runtime": "python",
    "files": {
      "yaml/__init__.py": "<base64 of yaml package>"
    }
  }'

  # 2. Create a persistent volume
  curl -X POST http://localhost:9000/volumes -H 'Content-Type: application/json' -d '{
    "name": "demo-store",
    "size_mb": 64,
    "description": "Persistent KV store for layer_volume_demo"
  }'

  # 3. Register the function
  nova register layer-volume-demo --runtime python --code examples/layer_volume_demo.py

  # 4. Attach the layer
  curl -X PUT http://localhost:9000/functions/layer-volume-demo/layers \
    -H 'Content-Type: application/json' -d '{"layer_ids": ["<layer-id>"]}'

  # 5. Mount the volume
  curl -X PUT http://localhost:9000/functions/layer-volume-demo/mounts \
    -H 'Content-Type: application/json' -d '{
      "mounts": [{"volume_id": "<volume-id>", "mount_path": "/mnt/data", "read_only": false}]
    }'

  # 6. Invoke
  nova invoke layer-volume-demo '{"action": "set", "key": "greeting", "value": "hello world"}'
  nova invoke layer-volume-demo '{"action": "get", "key": "greeting"}'
  nova invoke layer-volume-demo '{"action": "list"}'
"""

import json
import os
import time

STORE_DIR = "/mnt/data/kv"


def _ensure_store():
    os.makedirs(STORE_DIR, exist_ok=True)


def _key_path(key):
    safe = key.replace("/", "_").replace("..", "_")
    return os.path.join(STORE_DIR, safe + ".json")


def _try_import_yaml():
    """Attempt to load YAML from the dependency layer."""
    try:
        import yaml
        return yaml
    except ImportError:
        return None


def handler(event, context):
    action = event.get("action", "get")
    key = event.get("key", "")
    value = event.get("value")
    fmt = event.get("format", "json")  # "json" or "yaml"

    _ensure_store()

    if action == "set":
        if not key:
            return {"error": "key is required for set"}
        record = {
            "key": key,
            "value": value,
            "updated_at": time.time(),
        }
        with open(_key_path(key), "w") as f:
            json.dump(record, f)
        return {"status": "ok", "key": key, "stored": True}

    elif action == "get":
        if not key:
            return {"error": "key is required for get"}
        path = _key_path(key)
        if not os.path.exists(path):
            return {"error": f"key not found: {key}"}
        with open(path, "r") as f:
            record = json.load(f)

        # Use YAML formatting from the dependency layer if requested
        if fmt == "yaml":
            yaml = _try_import_yaml()
            if yaml:
                return {
                    "key": key,
                    "value": record["value"],
                    "yaml": yaml.dump(record, default_flow_style=False),
                    "layer": "pyyaml",
                }
            return {
                "key": key,
                "value": record["value"],
                "yaml": None,
                "layer_warning": "yaml layer not attached",
            }

        return {"key": key, "value": record["value"], "updated_at": record["updated_at"]}

    elif action == "list":
        keys = []
        for fname in sorted(os.listdir(STORE_DIR)):
            if fname.endswith(".json"):
                keys.append(fname[:-5])

        result = {"keys": keys, "count": len(keys)}

        # If yaml format requested, dump via layer dependency
        if fmt == "yaml":
            yaml = _try_import_yaml()
            if yaml:
                result["yaml"] = yaml.dump(result, default_flow_style=False)
                result["layer"] = "pyyaml"

        return result

    elif action == "clear":
        removed = 0
        for fname in os.listdir(STORE_DIR):
            if fname.endswith(".json"):
                os.remove(os.path.join(STORE_DIR, fname))
                removed += 1
        return {"status": "ok", "removed": removed}

    else:
        return {"error": f"unknown action: {action}"}
