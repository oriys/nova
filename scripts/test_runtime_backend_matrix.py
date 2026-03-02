#!/usr/bin/env python3
"""
Test all runtime × backend combinations for Nova.
Creates a function for each combo and invokes it.
"""

import json
import sys
import time
import urllib.request
import urllib.error

API = "http://localhost:9000"

# Handler code for each runtime (interpreted use handler(event,context) pattern)
RUNTIME_CODE = {
    "python": 'def handler(event, context):\n    name = event.get("name", "World")\n    return {"message": f"Hello, {name}!", "runtime": "python"}',
    "node": 'function handler(event, context) {\n  const name = event.name || "World";\n  return { message: "Hello, " + name + "!", runtime: "node" };\n}\nmodule.exports = { handler };',
    "ruby": 'def handler(event, context)\n  name = event["name"] || "World"\n  { message: "Hello, #{name}!", runtime: "ruby" }\nend',
    "php": '<?php\nfunction handler($event, $context) {\n    $name = $event["name"] ?? "World";\n    return ["message" => "Hello, $name!", "runtime" => "php"];\n}',
    "deno": 'export function handler(event, context) {\n  const name = event.name || "World";\n  return { message: `Hello, ${name}!`, runtime: "deno" };\n}',
    "bun": 'function handler(event, context) {\n  const name = event.name || "World";\n  return { message: `Hello, ${name}!`, runtime: "bun" };\n}\nmodule.exports = { handler };',
    "lua": 'function handler(event, context)\n  local name = event.name or "World"\n  return { message = "Hello, " .. name .. "!", runtime = "lua" }\nend',
    "perl": 'sub handler {\n    my ($event, $context) = @_;\n    my $name = $event->{name} || "World";\n    return { message => "Hello, $name!", runtime => "perl" };\n}',
    "elixir": 'defmodule Handler do\n  def handler(event, _context) do\n    name = Map.get(event, "name", "World")\n    %{message: "Hello, #{name}!", runtime: "elixir"}\n  end\nend',
    "r": 'handler <- function(event, context) {\n  name <- if (!is.null(event$name)) event$name else "World"\n  list(message = paste0("Hello, ", name, "!"), runtime = "r")\n}',
    "julia": 'function handler(event, context)\n    name = get(event, "name", "World")\n    return Dict("message" => "Hello, $(name)!", "runtime" => "julia")\nend',
    # Compiled languages - use server-side compilation
    "go": 'package main\n\nimport (\n\t"encoding/json"\n\t"fmt"\n)\n\ntype Event struct {\n\tName string `json:"name"`\n}\n\nfunc Handler(event json.RawMessage, ctx Context) (interface{}, error) {\n\tvar e Event\n\tjson.Unmarshal(event, &e)\n\tif e.Name == "" {\n\t\te.Name = "World"\n\t}\n\treturn map[string]string{"message": fmt.Sprintf("Hello, %s!", e.Name), "runtime": "go"}, nil\n}',
    "rust": 'use serde::{Deserialize, Serialize};\nuse serde_json::Value;\n\n#[derive(Deserialize)]\nstruct Event {\n    name: Option<String>,\n}\n\n#[derive(Serialize)]\nstruct Response {\n    message: String,\n    runtime: String,\n}\n\npub fn handler(event: Value, _ctx: crate::context::Context) -> Result<Value, String> {\n    let e: Event = serde_json::from_value(event).map_err(|e| e.to_string())?;\n    let name = e.name.unwrap_or_else(|| "World".to_string());\n    let resp = Response {\n        message: format!("Hello, {}!", name),\n        runtime: "rust".to_string(),\n    };\n    serde_json::to_value(&resp).map_err(|e| e.to_string())\n}',
    "java": 'import java.util.*;\n\npublic class Handler {\n    public static Object handler(String event, Map<String, Object> context) {\n        return "{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"java\\\"}";\n    }\n}',
    "kotlin": 'object Handler {\n    fun handler(event: String, context: Map<String, Any>): Any {\n        return "{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"kotlin\\\"}";\n    }\n}',
    "scala": 'object Handler {\n  def handler(event: String, context: Map[String, Any]): Any = {\n    "{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"scala\\\"}";\n  }\n}',
    "c": '#include <stdio.h>\n#include <string.h>\n\nconst char* handler(const char* event, const char* context) {\n    static char result[256];\n    snprintf(result, sizeof(result), "{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"c\\\"}");\n    return result;\n}',
    "cpp": '#include <string>\n\nstd::string handler(const std::string& event, const std::string& context) {\n    return std::string("{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"cpp\\\"}");\n}',
    "swift": 'import Foundation\n\nfunc handler(event: [String: Any], context: [String: Any]) -> [String: Any] {\n    let name = event["name"] as? String ?? "World"\n    return ["message": "Hello, \\(name)!", "runtime": "swift"]\n}',
    "zig": 'const std = @import("std");\n\nexport fn handler() void {\n    const stdout = std.io.getStdOut().writer();\n    stdout.writeAll("{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"zig\\\"}")\n    catch {};\n}',
    "graalvm": 'import java.util.*;\n\npublic class Handler {\n    public static Object handler(String event, Map<String, Object> context) {\n        return "{\\\"message\\\":\\\"Hello, World!\\\",\\\"runtime\\\":\\\"graalvm\\\"}";\n    }\n}',
    "wasm": None,  # Skip for now - needs pre-compiled .wasm binary
}

BACKENDS = ["firecracker", "docker", "wasm", "kubernetes"]

# Handler name per runtime
def handler_name(runtime):
    if runtime in ("java", "kotlin", "scala", "graalvm"):
        return "Handler::handler"
    elif runtime in ("go", "rust", "swift", "zig", "wasm", "c", "cpp"):
        return "handler"
    else:
        return "main.handler"


def api_call(method, path, data=None):
    url = f"{API}{path}"
    body = json.dumps(data).encode() if data else None
    req = urllib.request.Request(url, data=body, method=method)
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            return json.loads(resp.read()), resp.status
    except urllib.error.HTTPError as e:
        try:
            body = json.loads(e.read())
        except Exception:
            body = {"error": str(e)}
        return body, e.code
    except Exception as e:
        return {"error": str(e)}, 0


def create_function(name, runtime, backend, code):
    data = {
        "name": name,
        "runtime": runtime,
        "handler": handler_name(runtime),
        "memory_mb": 256 if runtime in ("java", "kotlin", "scala", "graalvm") else 128,
        "timeout_s": 60,
        "backend": backend,
        "code": code,
    }
    resp, status = api_call("POST", "/functions", data)
    if status == 409:  # already exists, update code
        api_call("PUT", f"/functions/{name}/code", {"code": code})
        api_call("PATCH", f"/functions/{name}", {"backend": backend})
        return True
    return status == 200 or status == 201


def invoke_function(name, payload, retries=60):
    for attempt in range(retries):
        resp, status = api_call("POST", f"/functions/{name}/invoke", payload)
        if status == 200:
            err = resp.get("error", "")
            if err and "still compiling" in str(err):
                time.sleep(3)
                continue
            return resp, None
        elif status == 0:
            return None, resp.get("error", "timeout")
        else:
            err = resp.get("error", "") or resp.get("message", "")
            if "still compiling" in str(err):
                time.sleep(3)
                continue
            if "rootfs not found" in str(err) or "image" in str(err).lower():
                return None, err
            # Transient error, retry a few times
            if attempt < 5:
                time.sleep(2)
                continue
            return None, err
    return None, "timed out waiting for compilation"


def main():
    results = []
    payload = {"name": "Test"}

    # Determine which combos to test
    combos = []
    for runtime in sorted(RUNTIME_CODE.keys()):
        code = RUNTIME_CODE[runtime]
        if code is None:
            continue
        for backend in BACKENDS:
            combos.append((runtime, backend, code))

    print(f"Testing {len(combos)} runtime × backend combinations\n")

    for i, (runtime, backend, code) in enumerate(combos):
        name = f"test-{runtime}-{backend}"
        print(f"[{i+1}/{len(combos)}] {runtime} × {backend}", end=" ... ", flush=True)

        # Create function
        start = time.time()
        ok = create_function(name, runtime, backend, code)
        if not ok:
            print("CREATE FAILED")
            results.append((runtime, backend, "create_failed", "", 0))
            continue

        # Invoke
        resp, err = invoke_function(name, payload)
        elapsed = (time.time() - start) * 1000

        if err:
            print(f"FAIL ({elapsed:.0f}ms): {err[:100]}")
            results.append((runtime, backend, "fail", str(err)[:500], elapsed))
        else:
            print(f"OK ({elapsed:.0f}ms)")
            results.append((runtime, backend, "pass", json.dumps(resp)[:200], elapsed))

    # Summary
    print("\n" + "=" * 70)
    print("RESULTS SUMMARY")
    print("=" * 70)

    pass_count = sum(1 for r in results if r[2] == "pass")
    fail_count = sum(1 for r in results if r[2] == "fail")
    skip_count = sum(1 for r in results if r[2] == "create_failed")

    # Print matrix
    runtimes_seen = sorted(set(r[0] for r in results))
    backends_seen = sorted(set(r[1] for r in results))

    # Header
    header = f"{'Runtime':<12}"
    for b in backends_seen:
        header += f" {b[:5]:>6}"
    print(header)
    print("-" * len(header))

    for rt in runtimes_seen:
        row = f"{rt:<12}"
        for b in backends_seen:
            match = [r for r in results if r[0] == rt and r[1] == b]
            if match:
                status = match[0][2]
                if status == "pass":
                    row += "     ✅"
                elif status == "fail":
                    row += "     ❌"
                else:
                    row += "     ⏭️"
            else:
                row += "     --"
        print(row)

    print(f"\nTotal: {pass_count} pass, {fail_count} fail, {skip_count} skip out of {len(results)}")

    # Print failures
    failures = [r for r in results if r[2] == "fail"]
    if failures:
        print(f"\nFAILURES ({len(failures)}):")
        for rt, be, _, err, _ in failures:
            print(f"  {rt} × {be}: {err[:120]}")

    # Write results to file for SQL import
    with open("/tmp/test_results.json", "w") as f:
        json.dump(results, f)

    return 0 if fail_count == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
