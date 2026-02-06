package main

// Bootstrap scripts for interpreted runtimes.
// Each bootstrap:
// 1. Reads event JSON from input file (argv[1])
// 2. Builds a context object from NOVA_* env vars
// 3. Imports the user's handler function from /code/handler
// 4. Calls handler(event, context) and prints result as JSON
//
// Each also supports --persistent mode: stdin/stdout JSON loop.

const bootstrapPython = `import json, sys, os

def _build_context():
    return {
        "request_id": os.environ.get("NOVA_REQUEST_ID", ""),
        "function_name": os.environ.get("NOVA_FUNCTION_NAME", ""),
        "function_version": os.environ.get("NOVA_FUNCTION_VERSION", ""),
        "memory_limit_mb": int(os.environ.get("NOVA_MEMORY_LIMIT_MB", "0")),
        "timeout_s": int(os.environ.get("NOVA_TIMEOUT_S", "0")),
        "runtime": os.environ.get("NOVA_RUNTIME", ""),
    }

def _load_handler():
    with open("/code/handler") as f:
        code = f.read()
    ns = {}
    exec(compile(code, "/code/handler", "exec"), ns)
    return ns["handler"]

_handler = _load_handler()

if "--persistent" in sys.argv:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        req = json.loads(line)
        ctx = _build_context()
        ctx["request_id"] = req.get("context", {}).get("request_id", ctx["request_id"])
        try:
            result = _handler(req.get("input", {}), ctx)
            print(json.dumps({"output": result}), flush=True)
        except Exception as e:
            print(json.dumps({"error": str(e)}), flush=True)
else:
    with open(sys.argv[1]) as f:
        event = json.load(f)
    ctx = _build_context()
    result = _handler(event, ctx)
    print(json.dumps(result))
`

const bootstrapNode = `const fs = require('fs');
const path = require('path');

function buildContext() {
  return {
    requestId: process.env.NOVA_REQUEST_ID || '',
    functionName: process.env.NOVA_FUNCTION_NAME || '',
    functionVersion: process.env.NOVA_FUNCTION_VERSION || '',
    memoryLimitMB: parseInt(process.env.NOVA_MEMORY_LIMIT_MB || '0', 10),
    timeoutS: parseInt(process.env.NOVA_TIMEOUT_S || '0', 10),
    runtime: process.env.NOVA_RUNTIME || '',
  };
}

const mod = require('/code/handler');
const handler = mod.handler || mod.default || mod;

if (process.argv.includes('--persistent')) {
  const readline = require('readline');
  const rl = readline.createInterface({ input: process.stdin });
  rl.on('line', (line) => {
    if (!line.trim()) return;
    const req = JSON.parse(line);
    const ctx = buildContext();
    if (req.context && req.context.request_id) ctx.requestId = req.context.request_id;
    try {
      const result = handler(req.input || {}, ctx);
      if (result && typeof result.then === 'function') {
        result.then(r => console.log(JSON.stringify({ output: r })))
              .catch(e => console.log(JSON.stringify({ error: e.message })));
      } else {
        console.log(JSON.stringify({ output: result }));
      }
    } catch (e) {
      console.log(JSON.stringify({ error: e.message }));
    }
  });
} else {
  const event = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
  const ctx = buildContext();
  const result = handler(event, ctx);
  if (result && typeof result.then === 'function') {
    result.then(r => console.log(JSON.stringify(r)));
  } else {
    console.log(JSON.stringify(result));
  }
}
`

const bootstrapRuby = `require 'json'

def build_context
  {
    'request_id' => ENV['NOVA_REQUEST_ID'] || '',
    'function_name' => ENV['NOVA_FUNCTION_NAME'] || '',
    'function_version' => ENV['NOVA_FUNCTION_VERSION'] || '',
    'memory_limit_mb' => (ENV['NOVA_MEMORY_LIMIT_MB'] || '0').to_i,
    'timeout_s' => (ENV['NOVA_TIMEOUT_S'] || '0').to_i,
    'runtime' => ENV['NOVA_RUNTIME'] || '',
  }
end

load '/code/handler'

if ARGV.include?('--persistent')
  STDIN.each_line do |line|
    line.strip!
    next if line.empty?
    req = JSON.parse(line)
    ctx = build_context
    ctx['request_id'] = req.dig('context', 'request_id') || ctx['request_id']
    begin
      result = handler(req['input'] || {}, ctx)
      puts JSON.generate({ 'output' => result })
      STDOUT.flush
    rescue => e
      puts JSON.generate({ 'error' => e.message })
      STDOUT.flush
    end
  end
else
  event = JSON.parse(File.read(ARGV[0]))
  ctx = build_context
  result = handler(event, ctx)
  puts JSON.generate(result)
end
`

const bootstrapPHP = `<?php
function nova_build_context() {
    return [
        'request_id' => getenv('NOVA_REQUEST_ID') ?: '',
        'function_name' => getenv('NOVA_FUNCTION_NAME') ?: '',
        'function_version' => getenv('NOVA_FUNCTION_VERSION') ?: '',
        'memory_limit_mb' => intval(getenv('NOVA_MEMORY_LIMIT_MB') ?: '0'),
        'timeout_s' => intval(getenv('NOVA_TIMEOUT_S') ?: '0'),
        'runtime' => getenv('NOVA_RUNTIME') ?: '',
    ];
}

require '/code/handler';

if (in_array('--persistent', $argv)) {
    while ($line = fgets(STDIN)) {
        $line = trim($line);
        if ($line === '') continue;
        $req = json_decode($line, true);
        $ctx = nova_build_context();
        if (isset($req['context']['request_id'])) $ctx['request_id'] = $req['context']['request_id'];
        try {
            $result = handler($req['input'] ?? [], $ctx);
            echo json_encode(['output' => $result]) . "\n";
        } catch (Exception $e) {
            echo json_encode(['error' => $e->getMessage()]) . "\n";
        }
    }
} else {
    $event = json_decode(file_get_contents($argv[1]), true);
    $ctx = nova_build_context();
    $result = handler($event, $ctx);
    echo json_encode($result);
}
`

const bootstrapDeno = `// Read and evaluate user handler
const _handlerCode = await Deno.readTextFile("/code/handler");
const _blob = new Blob([_handlerCode], { type: "application/javascript" });
const _url = URL.createObjectURL(_blob);
const _mod = await import(_url);
const handler = _mod.handler || _mod.default;

function buildContext() {
  return {
    requestId: Deno.env.get("NOVA_REQUEST_ID") || "",
    functionName: Deno.env.get("NOVA_FUNCTION_NAME") || "",
    functionVersion: Deno.env.get("NOVA_FUNCTION_VERSION") || "",
    memoryLimitMB: parseInt(Deno.env.get("NOVA_MEMORY_LIMIT_MB") || "0", 10),
    timeoutS: parseInt(Deno.env.get("NOVA_TIMEOUT_S") || "0", 10),
    runtime: Deno.env.get("NOVA_RUNTIME") || "",
  };
}

if (Deno.args.includes("--persistent")) {
  const decoder = new TextDecoder();
  const buf = new Uint8Array(65536);
  let leftover = "";
  while (true) {
    const n = await Deno.stdin.read(buf);
    if (n === null) break;
    leftover += decoder.decode(buf.subarray(0, n));
    const lines = leftover.split("\n");
    leftover = lines.pop() || "";
    for (const line of lines) {
      if (!line.trim()) continue;
      const req = JSON.parse(line);
      const ctx = buildContext();
      if (req.context?.request_id) ctx.requestId = req.context.request_id;
      try {
        const result = await handler(req.input || {}, ctx);
        console.log(JSON.stringify({ output: result }));
      } catch (e) {
        console.log(JSON.stringify({ error: e.message }));
      }
    }
  }
} else {
  const event = JSON.parse(await Deno.readTextFile(Deno.args[0]));
  const ctx = buildContext();
  const result = await handler(event, ctx);
  console.log(JSON.stringify(result));
}
`

const bootstrapLua = `local json_path = arg[1]
local persistent = false
for _, a in ipairs(arg) do
    if a == "--persistent" then persistent = true end
end

-- Minimal JSON encode/decode for environments without cjson
local json = {}
function json.decode(str)
    -- Try cjson first
    local ok, cjson = pcall(require, "cjson")
    if ok then return cjson.decode(str) end
    -- Fallback: use load (Lua 5.2+) for simple JSON
    local fn = load("return " .. str:gsub('null', 'nil'):gsub('%[', '{'):gsub('%]', '}'):gsub('"([^"]-)":', '["%1"]='))
    if fn then return fn() end
    return {}
end

function json.encode(val)
    local ok, cjson = pcall(require, "cjson")
    if ok then return cjson.encode(val) end
    if type(val) == "table" then
        local parts = {}
        for k, v in pairs(val) do
            local key = type(k) == "string" and ('"' .. k .. '"') or k
            parts[#parts+1] = key .. ":" .. json.encode(v)
        end
        return "{" .. table.concat(parts, ",") .. "}"
    elseif type(val) == "string" then
        return '"' .. val:gsub('"', '\\"') .. '"'
    elseif type(val) == "number" then
        return tostring(val)
    elseif type(val) == "boolean" then
        return tostring(val)
    else
        return "null"
    end
end

local function build_context()
    return {
        request_id = os.getenv("NOVA_REQUEST_ID") or "",
        function_name = os.getenv("NOVA_FUNCTION_NAME") or "",
        function_version = os.getenv("NOVA_FUNCTION_VERSION") or "",
        memory_limit_mb = tonumber(os.getenv("NOVA_MEMORY_LIMIT_MB") or "0"),
        timeout_s = tonumber(os.getenv("NOVA_TIMEOUT_S") or "0"),
        runtime = os.getenv("NOVA_RUNTIME") or "",
    }
end

dofile("/code/handler")

if persistent then
    while true do
        local line = io.read("*l")
        if not line then break end
        if line ~= "" then
            local ok, req = pcall(json.decode, line)
            if ok then
                local ctx = build_context()
                if req.context and req.context.request_id then
                    ctx.request_id = req.context.request_id
                end
                local success, result = pcall(handler, req.input or {}, ctx)
                if success then
                    io.write(json.encode({output = result}) .. "\n")
                else
                    io.write(json.encode({error = tostring(result)}) .. "\n")
                end
                io.flush()
            end
        end
    end
else
    local f = io.open(json_path, "r")
    local content = f:read("*a")
    f:close()
    local event = json.decode(content)
    local ctx = build_context()
    local result = handler(event, ctx)
    print(json.encode(result))
end
`

// bootstrapExtension returns the file extension for a given runtime's bootstrap.
func bootstrapExtension(runtime string) string {
	switch runtime {
	case "python":
		return ".py"
	case "node", "bun":
		return ".js"
	case "ruby":
		return ".rb"
	case "php":
		return ".php"
	case "deno":
		return ".ts"
	case "lua":
		return ".lua"
	default:
		return ""
	}
}

// bootstrapContent returns the bootstrap script content for a given runtime.
func bootstrapContent(runtime string) string {
	switch runtime {
	case "python":
		return bootstrapPython
	case "node", "bun":
		return bootstrapNode
	case "ruby":
		return bootstrapRuby
	case "php":
		return bootstrapPHP
	case "deno":
		return bootstrapDeno
	case "lua":
		return bootstrapLua
	default:
		return ""
	}
}
