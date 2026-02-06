// Example Bun function for Nova serverless platform

function handler(event, context) {
  const name = typeof event?.name === "string" && event.name ? event.name : "Anonymous"
  return {
    message: `Hello, ${name}!`,
    runtime: "bun",
    requestId: context.requestId,
  }
}

module.exports = { handler }
