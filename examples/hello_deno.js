// Example Deno function for Nova serverless platform
// Reads JSON from file path argv[0], prints JSON result to stdout.

function handler(event) {
  const name = typeof event?.name === "string" && event.name ? event.name : "Anonymous"
  return {
    message: `Hello, ${name}!`,
    runtime: "deno",
  }
}

function main() {
  const inputFile = Deno.args[0] || "/tmp/input.json"
  let event = {}
  try {
    event = JSON.parse(Deno.readTextFileSync(inputFile))
  } catch {
    event = {}
  }

  console.log(JSON.stringify(handler(event)))
}

main()

