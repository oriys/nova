"use client"

import React from "react"
import { useState, useEffect } from "react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { CodeEditor } from "@/components/code-editor"
import { RuntimeInfo } from "@/lib/types"
import { Loader2, FileCode, FolderOpen } from "lucide-react"

// Code templates for each runtime (base language)
const CODE_TEMPLATES: Record<string, string> = {
  python: `import json
import sys

def handler(event):
    name = event.get("name", "World")
    return {"message": f"Hello, {name}!"}

if __name__ == "__main__":
    with open(sys.argv[1]) as f:
        event = json.load(f)
    result = handler(event)
    print(json.dumps(result))
`,
  go: `package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Event struct {
	Name string \`json:"name"\`
}

func main() {
	data, _ := os.ReadFile(os.Args[1])
	var event Event
	json.Unmarshal(data, &event)
	name := event.Name
	if name == "" {
		name = "World"
	}
	result := map[string]string{"message": fmt.Sprintf("Hello, %s!", name)}
	output, _ := json.Marshal(result)
	fmt.Println(string(output))
}
`,
  node: `const fs = require('fs');

function handler(event) {
  const name = event.name || 'World';
  return { message: \`Hello, \${name}!\` };
}

const event = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
console.log(JSON.stringify(handler(event)));
`,
  rust: `use std::env;
use std::fs;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
struct Event { name: Option<String> }

#[derive(Serialize)]
struct Response { message: String }

fn main() {
    let data = fs::read_to_string(&env::args().nth(1).unwrap()).unwrap();
    let event: Event = serde_json::from_str(&data).unwrap();
    let name = event.name.unwrap_or_else(|| "World".to_string());
    let resp = Response { message: format!("Hello, {}!", name) };
    println!("{}", serde_json::to_string(&resp).unwrap());
}
`,
  ruby: `require 'json'

def handler(event)
  name = event['name'] || 'World'
  { message: "Hello, #{name}!" }
end

event = JSON.parse(File.read(ARGV[0]))
puts JSON.generate(handler(event))
`,
  java: `import java.nio.file.*;

public class Handler {
    public static void main(String[] args) throws Exception {
        String content = Files.readString(Path.of(args[0]));
        String name = content.contains("name") ? "User" : "World";
        System.out.println("{\\"message\\": \\"Hello, " + name + "!\\"}");
    }
}
`,
  deno: `const event = JSON.parse(await Deno.readTextFile(Deno.args[0]));
const name = event.name || 'World';
console.log(JSON.stringify({ message: \`Hello, \${name}!\` }));
`,
  bun: `const event = JSON.parse(await Bun.file(Bun.argv[2]).text());
const name = event.name || 'World';
console.log(JSON.stringify({ message: \`Hello, \${name}!\` }));
`,
  php: `<?php
$event = json_decode(file_get_contents($argv[1]), true);
$name = $event['name'] ?? 'World';
echo json_encode(['message' => "Hello, $name!"]);
`,
  dotnet: `using System.Text.Json;

var json = File.ReadAllText(args[0]);
var evt = JsonSerializer.Deserialize<Dictionary<string, string>>(json);
var name = evt?.GetValueOrDefault("name", "World") ?? "World";
Console.WriteLine(JsonSerializer.Serialize(new { message = $"Hello, {name}!" }));
`,
  elixir: `event = File.read!(System.argv() |> hd()) |> Jason.decode!()
name = Map.get(event, "name", "World")
IO.puts(Jason.encode!(%{message: "Hello, #{name}!"}))
`,
  kotlin: `import java.io.File
import kotlinx.serialization.json.*

fun main(args: Array<String>) {
    val json = File(args[0]).readText()
    val event = Json.parseToJsonElement(json).jsonObject
    val name = event["name"]?.jsonPrimitive?.content ?: "World"
    println("""{"message": "Hello, $name!"}""")
}
`,
  swift: `import Foundation

let data = try! Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1]))
let event = try! JSONSerialization.jsonObject(with: data) as! [String: Any]
let name = event["name"] as? String ?? "World"
print("{\\"message\\": \\"Hello, \\(name)!\\"}")
`,
  zig: `const std = @import("std");

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    const allocator = gpa.allocator();
    const args = try std.process.argsAlloc(allocator);
    const file = try std.fs.cwd().openFile(args[1], .{});
    defer file.close();
    const stdout = std.io.getStdOut().writer();
    try stdout.print("{{\\"message\\": \\"Hello, World!\\"}}\n", .{});
}
`,
  lua: `local json = require("cjson")
local file = io.open(arg[1], "r")
local event = json.decode(file:read("*all"))
file:close()
local name = event.name or "World"
print(json.encode({message = "Hello, " .. name .. "!"}))
`,
  perl: `use JSON;
open(my $fh, '<', $ARGV[0]);
my $event = decode_json(do { local $/; <$fh> });
my $name = $event->{name} // 'World';
print encode_json({message => "Hello, $name!"});
`,
  r: `library(jsonlite)
args <- commandArgs(trailingOnly = TRUE)
event <- fromJSON(args[1])
name <- ifelse(is.null(event$name), "World", event$name)
cat(toJSON(list(message = paste0("Hello, ", name, "!")), auto_unbox = TRUE))
`,
  julia: `using JSON3
event = JSON3.read(read(ARGS[1], String))
name = get(event, :name, "World")
println(JSON3.write(Dict(:message => "Hello, $name!")))
`,
  scala: `import scala.io.Source
import spray.json._

object Handler extends App {
  val json = Source.fromFile(args(0)).mkString.parseJson.asJsObject
  val name = json.fields.get("name").map(_.convertTo[String]).getOrElse("World")
  println(s"""{"message": "Hello, $name!"}""")
}
`,
  wasm: `// WebAssembly - compile with WASI support
// Rust: cargo build --target wasm32-wasi --release
// Go: GOOS=wasip1 GOARCH=wasm go build -o handler.wasm
`,
}

// Get base runtime from versioned ID (e.g., "python3.11" -> "python")
function getBaseRuntime(runtimeId: string): string {
  const prefixes = ['python', 'go', 'node', 'rust', 'ruby', 'java', 'php', 'dotnet', 'scala']
  for (const prefix of prefixes) {
    if (runtimeId.startsWith(prefix)) return prefix
  }
  return runtimeId
}

interface CreateFunctionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (
    name: string,
    runtime: string,
    handler: string,
    memory: number,
    timeout: number,
    codeOrPath: string,
    isCode: boolean
  ) => Promise<void>
  runtimes?: RuntimeInfo[]
}

export function CreateFunctionDialog({
  open,
  onOpenChange,
  onCreate,
  runtimes = [],
}: CreateFunctionDialogProps) {
  const [name, setName] = useState("")
  const [runtime, setRuntime] = useState("python")
  const [memory, setMemory] = useState("128")
  const [timeout, setTimeout] = useState("30")
  const [handler, setHandler] = useState("main.handler")
  const [codeMode, setCodeMode] = useState<"code" | "path">("code")
  const [code, setCode] = useState(CODE_TEMPLATES.python)
  const [codePath, setCodePath] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Update code template when runtime changes
  useEffect(() => {
    if (codeMode === "code") {
      const baseRuntime = getBaseRuntime(runtime)
      setCode(CODE_TEMPLATES[baseRuntime] || CODE_TEMPLATES.python)
    }
  }, [runtime, codeMode])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    const codeOrPath = codeMode === "code" ? code : codePath
    if (!codeOrPath.trim()) {
      setError(codeMode === "code" ? "Code is required" : "Code path is required")
      return
    }

    try {
      setSubmitting(true)
      await onCreate(name, runtime, handler, parseInt(memory), parseInt(timeout), codeOrPath, codeMode === "code")

      // Reset form
      setName("")
      setRuntime("python")
      setMemory("128")
      setTimeout("30")
      setHandler("main.handler")
      setCode(CODE_TEMPLATES.python)
      setCodePath("")
      setCodeMode("code")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create function")
    } finally {
      setSubmitting(false)
    }
  }

  // Group runtimes by language for better UX
  const groupedRuntimes = runtimes.length > 0 ? runtimes : [
    { id: "python", name: "Python", version: "3.12", status: "available" as const, functionsCount: 0, icon: "python" },
    { id: "python3.11", name: "Python", version: "3.11", status: "available" as const, functionsCount: 0, icon: "python" },
    { id: "go", name: "Go", version: "1.22", status: "available" as const, functionsCount: 0, icon: "go" },
    { id: "go1.21", name: "Go", version: "1.21", status: "available" as const, functionsCount: 0, icon: "go" },
    { id: "node", name: "Node.js", version: "22.x", status: "available" as const, functionsCount: 0, icon: "nodejs" },
    { id: "node20", name: "Node.js", version: "20.x", status: "available" as const, functionsCount: 0, icon: "nodejs" },
    { id: "rust", name: "Rust", version: "1.76", status: "available" as const, functionsCount: 0, icon: "rust" },
    { id: "deno", name: "Deno", version: "1.40", status: "available" as const, functionsCount: 0, icon: "deno" },
    { id: "bun", name: "Bun", version: "1.0", status: "available" as const, functionsCount: 0, icon: "bun" },
    { id: "ruby", name: "Ruby", version: "3.3", status: "available" as const, functionsCount: 0, icon: "ruby" },
    { id: "java", name: "Java", version: "21", status: "available" as const, functionsCount: 0, icon: "java" },
    { id: "java17", name: "Java", version: "17", status: "available" as const, functionsCount: 0, icon: "java" },
    { id: "kotlin", name: "Kotlin", version: "1.9", status: "available" as const, functionsCount: 0, icon: "kotlin" },
    { id: "scala", name: "Scala", version: "3.3", status: "available" as const, functionsCount: 0, icon: "scala" },
    { id: "php", name: "PHP", version: "8.3", status: "available" as const, functionsCount: 0, icon: "php" },
    { id: "dotnet", name: ".NET", version: "8.0", status: "available" as const, functionsCount: 0, icon: "dotnet" },
    { id: "elixir", name: "Elixir", version: "1.16", status: "available" as const, functionsCount: 0, icon: "elixir" },
    { id: "swift", name: "Swift", version: "5.9", status: "available" as const, functionsCount: 0, icon: "swift" },
    { id: "zig", name: "Zig", version: "0.11", status: "available" as const, functionsCount: 0, icon: "zig" },
    { id: "lua", name: "Lua", version: "5.4", status: "available" as const, functionsCount: 0, icon: "lua" },
    { id: "perl", name: "Perl", version: "5.38", status: "available" as const, functionsCount: 0, icon: "perl" },
    { id: "r", name: "R", version: "4.3", status: "available" as const, functionsCount: 0, icon: "r" },
    { id: "julia", name: "Julia", version: "1.10", status: "available" as const, functionsCount: 0, icon: "julia" },
    { id: "wasm", name: "WebAssembly", version: "wasmtime", status: "available" as const, functionsCount: 0, icon: "wasm" },
  ]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Create New Function</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          {error && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="name">Function Name</Label>
              <Input
                id="name"
                placeholder="my-function"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="runtime">Runtime</Label>
              <Select value={runtime} onValueChange={setRuntime}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="max-h-64">
                  {groupedRuntimes.map((rt) => (
                    <SelectItem key={rt.id} value={rt.id}>
                      {rt.name} {rt.version}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label>Code Source</Label>
            <Tabs value={codeMode} onValueChange={(v) => setCodeMode(v as "code" | "path")}>
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="code" className="flex items-center gap-2">
                  <FileCode className="h-4 w-4" />
                  Write Code
                </TabsTrigger>
                <TabsTrigger value="path" className="flex items-center gap-2">
                  <FolderOpen className="h-4 w-4" />
                  File Path
                </TabsTrigger>
              </TabsList>

              <TabsContent value="code" className="mt-3">
                <CodeEditor
                  code={code}
                  onChange={setCode}
                  runtime={runtime}
                  minHeight="256px"
                />
                <p className="text-xs text-muted-foreground mt-1">
                  Template loaded for {getBaseRuntime(runtime)}. Modify as needed.
                </p>
              </TabsContent>

              <TabsContent value="path" className="mt-3">
                <Input
                  placeholder="/path/to/handler.py"
                  value={codePath}
                  onChange={(e) => setCodePath(e.target.value)}
                />
                <p className="text-xs text-muted-foreground mt-1">
                  Absolute path to the handler file on the server
                </p>
              </TabsContent>
            </Tabs>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <div className="space-y-2">
              <Label htmlFor="memory">Memory (MB)</Label>
              <Select value={memory} onValueChange={setMemory}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="128">128 MB</SelectItem>
                  <SelectItem value="256">256 MB</SelectItem>
                  <SelectItem value="512">512 MB</SelectItem>
                  <SelectItem value="1024">1024 MB</SelectItem>
                  <SelectItem value="2048">2048 MB</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="timeout">Timeout (s)</Label>
              <Input
                id="timeout"
                type="number"
                min="1"
                max="900"
                value={timeout}
                onChange={(e) => setTimeout(e.target.value)}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="handler">Handler</Label>
              <Input
                id="handler"
                placeholder="main.handler"
                value={handler}
                onChange={(e) => setHandler(e.target.value)}
              />
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!name.trim() || (codeMode === "code" ? !code.trim() : !codePath.trim()) || submitting}
            >
              {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create Function
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
