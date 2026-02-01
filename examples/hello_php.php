<?php
// Example PHP function for Nova serverless platform
// Reads JSON from file path argv[1], prints JSON result to stdout.

$inputFile = $argv[1] ?? "/tmp/input.json";
$event = [];

if (is_file($inputFile)) {
  $raw = file_get_contents($inputFile);
  $decoded = json_decode($raw, true);
  if (is_array($decoded)) {
    $event = $decoded;
  }
}

$name = $event["name"] ?? "Anonymous";
if (!is_string($name) || $name === "") {
  $name = "Anonymous";
}

$out = [
  "message" => "Hello, {$name}!",
  "runtime" => "php",
];

echo json_encode($out);

