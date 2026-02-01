#!/usr/bin/env ruby
# Example Ruby function for Nova serverless platform
# Reads JSON from file path argv[0], prints JSON result to stdout.

require "json"

def handler(event)
  name = event["name"]
  name = "Anonymous" if name.nil? || name == ""
  {
    message: "Hello, #{name}!",
    runtime: "ruby",
  }
end

input_file = ARGV[0] || "/tmp/input.json"
event = {}
begin
  event = JSON.parse(File.read(input_file))
rescue StandardError
  event = {}
end

puts(handler(event).to_json)

