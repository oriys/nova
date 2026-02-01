// Example Node.js function for Nova serverless platform
// Reads JSON from file path argv[1], prints JSON result to stdout.

const fs = require("fs")

function handler(event) {
  const name = typeof event?.name === "string" && event.name ? event.name : "Anonymous"
  return {
    message: `Hello, ${name}!`,
    runtime: "node",
  }
}

function main() {
  const inputFile = process.argv[2] || "/tmp/input.json"
  let event = {}
  try {
    event = JSON.parse(fs.readFileSync(inputFile, "utf8"))
  } catch {
    event = {}
  }

  const result = handler(event)
  process.stdout.write(JSON.stringify(result))
}

main()

