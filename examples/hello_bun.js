// Example Bun function for Nova serverless platform
// Reads JSON from file path argv[1], prints JSON result to stdout.

const fs = require("fs")

function handler(event) {
  const name = typeof event?.name === "string" && event.name ? event.name : "Anonymous"
  return {
    message: `Hello, ${name}!`,
    runtime: "bun",
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

  console.log(JSON.stringify(handler(event)))
}

main()

