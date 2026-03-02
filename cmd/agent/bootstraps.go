package main

// Bootstrap scripts for interpreted runtimes.
// Each bootstrap:
// 1. Reads event JSON from input file (argv[1])
// 2. Builds an AWS Lambda-compatible context object from NOVA_* env vars
// 3. Imports the user's handler function from /code/handler
// 4. Calls handler(event, context) and prints result as JSON
//
// Context follows the AWS Lambda convention (without aws prefix):
//   Python:  context.function_name, context.request_id, context.get_remaining_time_in_millis()
//   Node.js: context.functionName, context.requestId, context.getRemainingTimeInMillis()
//
// Each also supports --persistent mode: stdin/stdout JSON loop.

const bootstrapPython = `import json, sys, os, time as _time

class _LambdaContext:
    def __init__(self):
        self.function_name = os.environ.get("NOVA_FUNCTION_NAME", "")
        self.function_version = os.environ.get("NOVA_FUNCTION_VERSION", "$LATEST")
        self.invoked_function_arn = ""
        self.memory_limit_in_mb = int(os.environ.get("NOVA_MEMORY_LIMIT_MB", "128"))
        self.request_id = os.environ.get("NOVA_REQUEST_ID", "")
        self.log_group_name = ""
        self.log_stream_name = ""
        self._timeout_s = int(os.environ.get("NOVA_TIMEOUT_S", "0"))
        self._start_ms = _time.time() * 1000

    def get_remaining_time_in_millis(self):
        if self._timeout_s <= 0:
            return 300000
        return max(0, int(self._timeout_s * 1000 - (_time.time() * 1000 - self._start_ms)))

def _load_handler():
    _code_dir = os.environ.get("NOVA_CODE_DIR", "/code")
    _handler_path = os.path.join(_code_dir, "handler")
    with open(_handler_path) as f:
        code = f.read()
    ns = {}
    exec(compile(code, _handler_path, "exec"), ns)
    return ns["handler"]

_handler = _load_handler()

if "--persistent" in sys.argv:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        req = json.loads(line)
        ctx = _LambdaContext()
        rid = req.get("context", {}).get("request_id")
        if rid:
            ctx.request_id = rid
        ctx._start_ms = _time.time() * 1000
        try:
            result = _handler(req.get("input", {}), ctx)
            print(json.dumps({"output": result}), flush=True)
        except Exception as e:
            print(json.dumps({"error": str(e)}), flush=True)
else:
    with open(sys.argv[1]) as f:
        event = json.load(f)
    ctx = _LambdaContext()
    result = _handler(event, ctx)
    print(json.dumps(result))
`

const bootstrapNode = `const fs = require('fs');
const path = require('path');

function buildContext() {
  const timeoutS = parseInt(process.env.NOVA_TIMEOUT_S || '0', 10);
  const startMs = Date.now();
  return {
    functionName: process.env.NOVA_FUNCTION_NAME || '',
    functionVersion: process.env.NOVA_FUNCTION_VERSION || '$LATEST',
    invokedFunctionArn: '',
    memoryLimitInMB: parseInt(process.env.NOVA_MEMORY_LIMIT_MB || '128', 10),
    requestId: process.env.NOVA_REQUEST_ID || '',
    logGroupName: '',
    logStreamName: '',
    getRemainingTimeInMillis: () => {
      if (timeoutS <= 0) return 300000;
      return Math.max(0, timeoutS * 1000 - (Date.now() - startMs));
    },
  };
}

const mod = require(path.join(process.env.NOVA_CODE_DIR || '/code', 'handler'));
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

_code_dir = ENV['NOVA_CODE_DIR'] || '/code'

# Load bundler-installed gems if present
vendor_dirs = [File.join(_code_dir, 'vendor/bundle'), File.join(_code_dir, 'vendor')]
vendor_dirs.each do |vdir|
  gemspec_dirs = Dir.glob(File.join(vdir, 'ruby/*/gems/*/lib'))
  gemspec_dirs.each { |d| $LOAD_PATH.unshift(d) unless $LOAD_PATH.include?(d) }
end

class LambdaContext
  attr_accessor :function_name, :function_version, :invoked_function_arn,
                :memory_limit_in_mb, :request_id, :log_group_name, :log_stream_name

  def initialize
    @function_name = ENV['NOVA_FUNCTION_NAME'] || ''
    @function_version = ENV['NOVA_FUNCTION_VERSION'] || '$LATEST'
    @invoked_function_arn = ''
    @memory_limit_in_mb = (ENV['NOVA_MEMORY_LIMIT_MB'] || '128').to_i
    @request_id = ENV['NOVA_REQUEST_ID'] || ''
    @log_group_name = ''
    @log_stream_name = ''
    @timeout_s = (ENV['NOVA_TIMEOUT_S'] || '0').to_i
    @start_ms = Process.clock_gettime(Process::CLOCK_MONOTONIC) * 1000
  end

  def get_remaining_time_in_millis
    return 300000 if @timeout_s <= 0
    elapsed = Process.clock_gettime(Process::CLOCK_MONOTONIC) * 1000 - @start_ms
    [0, (@timeout_s * 1000 - elapsed).to_i].max
  end
end

load File.join(_code_dir, 'handler')

if ARGV.include?('--persistent')
  STDIN.each_line do |line|
    line.strip!
    next if line.empty?
    req = JSON.parse(line)
    ctx = LambdaContext.new
    ctx.request_id = req.dig('context', 'request_id') || ctx.request_id
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
  ctx = LambdaContext.new
  result = handler(event, ctx)
  puts JSON.generate(result)
end
`

const bootstrapPHP = `<?php
function nova_build_context() {
    $timeoutS = intval(getenv('NOVA_TIMEOUT_S') ?: '0');
    $startMs = microtime(true) * 1000;
    return [
        'function_name' => getenv('NOVA_FUNCTION_NAME') ?: '',
        'function_version' => getenv('NOVA_FUNCTION_VERSION') ?: '$LATEST',
        'invoked_function_arn' => '',
        'memory_limit_in_mb' => intval(getenv('NOVA_MEMORY_LIMIT_MB') ?: '128'),
        'request_id' => getenv('NOVA_REQUEST_ID') ?: '',
        'log_group_name' => '',
        'log_stream_name' => '',
        'get_remaining_time_in_millis' => function() use ($timeoutS, $startMs) {
            if ($timeoutS <= 0) return 300000;
            return max(0, intval($timeoutS * 1000 - (microtime(true) * 1000 - $startMs)));
        },
    ];
}

require (getenv('NOVA_CODE_DIR') ?: '/code') . '/handler';

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
            flush();
        } catch (Exception $e) {
            echo json_encode(['error' => $e->getMessage()]) . "\n";
            flush();
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
const _handlerCode = await Deno.readTextFile((Deno.env.get("NOVA_CODE_DIR") || "/code") + "/handler");
const _blob = new Blob([_handlerCode], { type: "application/typescript" });
const _url = URL.createObjectURL(_blob);
const _mod = await import(_url);
const handler = _mod.handler || _mod.default;

function buildContext() {
  const timeoutS = parseInt(Deno.env.get("NOVA_TIMEOUT_S") || "0", 10);
  const startMs = Date.now();
  return {
    functionName: Deno.env.get("NOVA_FUNCTION_NAME") || "",
    functionVersion: Deno.env.get("NOVA_FUNCTION_VERSION") || "$LATEST",
    invokedFunctionArn: "",
    memoryLimitInMB: parseInt(Deno.env.get("NOVA_MEMORY_LIMIT_MB") || "128", 10),
    requestId: Deno.env.get("NOVA_REQUEST_ID") || "",
    logGroupName: "",
    logStreamName: "",
    getRemainingTimeInMillis: () => {
      if (timeoutS <= 0) return 300000;
      return Math.max(0, timeoutS * 1000 - (Date.now() - startMs));
    },
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
    local timeout_s = tonumber(os.getenv("NOVA_TIMEOUT_S") or "0")
    local start_ms = os.clock() * 1000
    return {
        function_name = os.getenv("NOVA_FUNCTION_NAME") or "",
        function_version = os.getenv("NOVA_FUNCTION_VERSION") or "$LATEST",
        invoked_function_arn = "",
        memory_limit_in_mb = tonumber(os.getenv("NOVA_MEMORY_LIMIT_MB") or "128"),
        request_id = os.getenv("NOVA_REQUEST_ID") or "",
        log_group_name = "",
        log_stream_name = "",
        get_remaining_time_in_millis = function()
            if timeout_s <= 0 then return 300000 end
            return math.max(0, math.floor(timeout_s * 1000 - (os.clock() * 1000 - start_ms)))
        end,
    }
end

dofile((os.getenv("NOVA_CODE_DIR") or "/code") .. "/handler")

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

const bootstrapElixir = `code_dir = System.get_env("NOVA_CODE_DIR", "/code")
Code.require_file(Path.join(code_dir, "handler"))

build_context = fn ->
  timeout_s = String.to_integer(System.get_env("NOVA_TIMEOUT_S", "0"))
  start_ms = System.monotonic_time(:millisecond)
  %{
    function_name: System.get_env("NOVA_FUNCTION_NAME", ""),
    function_version: System.get_env("NOVA_FUNCTION_VERSION", "$LATEST"),
    invoked_function_arn: "",
    memory_limit_in_mb: String.to_integer(System.get_env("NOVA_MEMORY_LIMIT_MB", "128")),
    request_id: System.get_env("NOVA_REQUEST_ID", ""),
    log_group_name: "",
    log_stream_name: "",
    get_remaining_time_in_millis: fn ->
      if timeout_s <= 0, do: 300_000,
      else: max(0, timeout_s * 1000 - (System.monotonic_time(:millisecond) - start_ms))
    end
  }
end

persistent = "--persistent" in System.argv()

if persistent do
  Enum.each(IO.stream(:stdio, :line), fn line ->
    line = String.trim(line)
    if line != "" do
      req = :json.decode(line)
      ctx = build_context.()
      ctx = case req do
        %{"context" => %{"request_id" => rid}} -> %{ctx | request_id: rid}
        _ -> ctx
      end
      try do
        input = Map.get(req, "input", %{})
        result = Handler.handler(input, ctx)
        IO.puts(:json.encode(result) |> then(fn encoded -> "{\"output\":#{encoded}}" end))
      rescue
        e -> IO.puts(:json.encode(%{"error" => Exception.message(e)}))
      end
    end
  end)
else
  [json_path | _] = System.argv()
  event = File.read!(json_path) |> :json.decode()
  ctx = build_context.()
  result = Handler.handler(event, ctx)
  IO.puts(:json.encode(result))
end
`

const bootstrapPerl = `#!/usr/bin/env perl
use strict;
use warnings;
use JSON::PP;

my $code_dir = $ENV{NOVA_CODE_DIR} // "/code";

sub build_context {
    my $timeout_s = int($ENV{NOVA_TIMEOUT_S} // 0);
    my $start_ms = time() * 1000;
    return {
        function_name => $ENV{NOVA_FUNCTION_NAME} // "",
        function_version => $ENV{NOVA_FUNCTION_VERSION} // '$LATEST',
        invoked_function_arn => "",
        memory_limit_in_mb => int($ENV{NOVA_MEMORY_LIMIT_MB} // 128),
        request_id => $ENV{NOVA_REQUEST_ID} // "",
        log_group_name => "",
        log_stream_name => "",
        get_remaining_time_in_millis => sub {
            return 300000 if $timeout_s <= 0;
            my $elapsed = time() * 1000 - $start_ms;
            my $remaining = $timeout_s * 1000 - $elapsed;
            return $remaining > 0 ? int($remaining) : 0;
        },
    };
}

do "$code_dir/handler" or die "Cannot load handler: $@$!";

my $persistent = grep { $_ eq "--persistent" } @ARGV;

if ($persistent) {
    while (my $line = <STDIN>) {
        chomp $line;
        next if $line eq "";
        my $req = decode_json($line);
        my $ctx = build_context();
        if ($req->{context} && $req->{context}{request_id}) {
            $ctx->{request_id} = $req->{context}{request_id};
        }
        eval {
            my $result = handler($req->{input} // {}, $ctx);
            print encode_json({output => $result}) . "\n";
        };
        if ($@) {
            print encode_json({error => "$@"}) . "\n";
        }
        STDOUT->flush();
    }
} else {
    open my $fh, "<", $ARGV[0] or die "Cannot open $ARGV[0]: $!";
    my $json = do { local $/; <$fh> };
    close $fh;
    my $event = decode_json($json);
    my $ctx = build_context();
    my $result = handler($event, $ctx);
    print encode_json($result) . "\n";
}
`

const bootstrapR = `args <- commandArgs(trailingOnly = TRUE)
persistent <- "--persistent" %in% args

build_context <- function() {
  timeout_s <- as.integer(Sys.getenv("NOVA_TIMEOUT_S", "0"))
  start_ms <- as.numeric(proc.time()["elapsed"]) * 1000
  list(
    function_name = Sys.getenv("NOVA_FUNCTION_NAME", ""),
    function_version = Sys.getenv("NOVA_FUNCTION_VERSION", "$LATEST"),
    invoked_function_arn = "",
    memory_limit_in_mb = as.integer(Sys.getenv("NOVA_MEMORY_LIMIT_MB", "128")),
    request_id = Sys.getenv("NOVA_REQUEST_ID", ""),
    log_group_name = "",
    log_stream_name = "",
    get_remaining_time_in_millis = function() {
      if (timeout_s <= 0) return(300000)
      elapsed <- as.numeric(proc.time()["elapsed"]) * 1000 - start_ms
      return(max(0, as.integer(timeout_s * 1000 - elapsed)))
    }
  )
}

code_dir <- Sys.getenv("NOVA_CODE_DIR", "/code")
source(file.path(code_dir, "handler"))

if (persistent) {
  con <- file("stdin", "r")
  while (TRUE) {
    line <- readLines(con, n = 1)
    if (length(line) == 0) break
    if (nchar(trimws(line)) == 0) next
    req <- jsonlite::fromJSON(line, simplifyVector = FALSE)
    ctx <- build_context()
    if (!is.null(req$context$request_id)) ctx$request_id <- req$context$request_id
    tryCatch({
      result <- handler(if (is.null(req$input)) list() else req$input, ctx)
      cat(jsonlite::toJSON(list(output = result), auto_unbox = TRUE), "\n", sep = "")
      flush(stdout())
    }, error = function(e) {
      cat(jsonlite::toJSON(list(error = conditionMessage(e)), auto_unbox = TRUE), "\n", sep = "")
      flush(stdout())
    })
  }
  close(con)
} else {
  json_path <- args[!args %in% "--persistent"][1]
  event <- jsonlite::fromJSON(json_path, simplifyVector = FALSE)
  ctx <- build_context()
  result <- handler(event, ctx)
  cat(jsonlite::toJSON(result, auto_unbox = TRUE), "\n", sep = "")
}
`

const bootstrapJulia = `# Minimal JSON parser/serializer (no external packages required)
module MiniJSON
    function parse_value(s, i)
        i = skip_ws(s, i)
        c = s[i]
        if c == '"'
            return parse_string(s, i)
        elseif c == '{'
            return parse_object(s, i)
        elseif c == '['
            return parse_array(s, i)
        elseif c == 't' && SubString(s, i, i+3) == "true"
            return true, i+4
        elseif c == 'f' && SubString(s, i, i+4) == "false"
            return false, i+5
        elseif c == 'n' && SubString(s, i, i+3) == "null"
            return nothing, i+4
        else
            return parse_number(s, i)
        end
    end
    function skip_ws(s, i)
        while i <= lastindex(s) && s[i] in (' ', '\t', '\n', '\r'); i = nextind(s, i); end
        return i
    end
    function parse_string(s, i)
        i = nextind(s, i)  # skip opening "
        buf = IOBuffer()
        while i <= lastindex(s) && s[i] != '"'
            if s[i] == '\\'
                i = nextind(s, i)
                c = s[i]
                if c == 'n'; write(buf, '\n')
                elseif c == 't'; write(buf, '\t')
                elseif c == 'r'; write(buf, '\r')
                elseif c == '"'; write(buf, '"')
                elseif c == '\\'; write(buf, '\\')
                elseif c == '/'; write(buf, '/')
                elseif c == 'u'
                    hex = SubString(s, nextind(s, i), nextind(s, i, 4))
                    write(buf, Char(Base.parse(UInt16, hex; base=16)))
                    i = nextind(s, i, 4)
                end
            else
                write(buf, s[i])
            end
            i = nextind(s, i)
        end
        return String(take!(buf)), nextind(s, i)  # skip closing "
    end
    function parse_number(s, i)
        j = i
        while j <= lastindex(s) && s[j] in ('-', '+', '.', 'e', 'E', '0':'9'...); j = nextind(s, j); end
        ns = SubString(s, i, prevind(s, j))
        val = occursin('.', ns) || occursin('e', ns) || occursin('E', ns) ? Base.parse(Float64, ns) : Base.parse(Int64, ns)
        return val, j
    end
    function parse_object(s, i)
        d = Dict{String,Any}()
        i = skip_ws(s, nextind(s, i))
        s[i] == '}' && return d, nextind(s, i)
        while true
            i = skip_ws(s, i)
            key, i = parse_string(s, i)
            i = skip_ws(s, i)
            i = nextind(s, i)  # skip :
            val, i = parse_value(s, i)
            d[key] = val
            i = skip_ws(s, i)
            s[i] == '}' && return d, nextind(s, i)
            i = nextind(s, i)  # skip ,
        end
    end
    function parse_array(s, i)
        a = Any[]
        i = skip_ws(s, nextind(s, i))
        s[i] == ']' && return a, nextind(s, i)
        while true
            val, i = parse_value(s, i)
            push!(a, val)
            i = skip_ws(s, i)
            s[i] == ']' && return a, nextind(s, i)
            i = nextind(s, i)  # skip ,
        end
    end
    function parse(s::AbstractString)
        val, _ = parse_value(s, firstindex(s))
        return val
    end
    parsefile(path) = parse(read(path, String))
    function json(x)
        buf = IOBuffer()
        _write(buf, x)
        return String(take!(buf))
    end
    _write(io, ::Nothing) = print(io, "null")
    _write(io, x::Bool) = print(io, x ? "true" : "false")
    _write(io, x::Integer) = print(io, x)
    _write(io, x::AbstractFloat) = isinteger(x) && isfinite(x) ? print(io, Int(x)) : print(io, x)
    function _write(io, s::AbstractString)
        print(io, '"')
        for c in s
            if c == '"'; print(io, "\\\"")
            elseif c == '\\'; print(io, "\\\\")
            elseif c == '\n'; print(io, "\\n")
            elseif c == '\r'; print(io, "\\r")
            elseif c == '\t'; print(io, "\\t")
            else print(io, c)
            end
        end
        print(io, '"')
    end
    function _write(io, d::AbstractDict)
        print(io, '{')
        first = true
        for (k, v) in d
            first || print(io, ',')
            _write(io, string(k))
            print(io, ':')
            _write(io, v)
            first = false
        end
        print(io, '}')
    end
    function _write(io, a::AbstractVector)
        print(io, '[')
        for (i, v) in enumerate(a)
            i > 1 && print(io, ',')
            _write(io, v)
        end
        print(io, ']')
    end
    _write(io, x) = _write(io, string(x))
end

function build_context()
    timeout_s = Base.parse(Int, get(ENV, "NOVA_TIMEOUT_S", "0"))
    start_ms = time() * 1000
    return Dict{String,Any}(
        "function_name" => get(ENV, "NOVA_FUNCTION_NAME", ""),
        "function_version" => get(ENV, "NOVA_FUNCTION_VERSION", "\$LATEST"),
        "invoked_function_arn" => "",
        "memory_limit_in_mb" => Base.parse(Int, get(ENV, "NOVA_MEMORY_LIMIT_MB", "128")),
        "request_id" => get(ENV, "NOVA_REQUEST_ID", ""),
        "log_group_name" => "",
        "log_stream_name" => "",
        "get_remaining_time_in_millis" => () -> begin
            timeout_s <= 0 && return 300000
            return max(0, round(Int, timeout_s * 1000 - (time() * 1000 - start_ms)))
        end,
    )
end

code_dir = get(ENV, "NOVA_CODE_DIR", "/code")
include(joinpath(code_dir, "handler"))

persistent = "--persistent" in ARGS

if persistent
    for line in eachline(stdin)
        line = strip(line)
        isempty(line) && continue
        req = MiniJSON.parse(line)
        ctx = build_context()
        if haskey(req, "context") && haskey(req["context"], "request_id")
            ctx["request_id"] = req["context"]["request_id"]
        end
        try
            input = get(req, "input", Dict{String,Any}())
            result = handler(input, ctx)
            println(MiniJSON.json(Dict{String,Any}("output" => result)))
            flush(stdout)
        catch e
            println(MiniJSON.json(Dict{String,Any}("error" => string(e))))
            flush(stdout)
        end
    end
else
    json_path = ARGS[findfirst(a -> a != "--persistent", ARGS)]
    event = MiniJSON.parsefile(json_path)
    ctx = build_context()
    result = handler(event, ctx)
    println(MiniJSON.json(result))
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
	case "elixir":
		return ".exs"
	case "perl":
		return ".pl"
	case "r":
		return ".R"
	case "julia":
		return ".jl"
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
	case "elixir":
		return bootstrapElixir
	case "perl":
		return bootstrapPerl
	case "r":
		return bootstrapR
	case "julia":
		return bootstrapJulia
	default:
		return ""
	}
}
