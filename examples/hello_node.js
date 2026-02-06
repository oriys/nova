// Example Node.js function for Nova serverless platform

function handler(event, context) {
  const name = typeof event?.name === "string" && event.name ? event.name : "Anonymous"
  return {
    message: `Hello, ${name}!`,
    runtime: "node",
    requestId: context.requestId,
  }
}

module.exports = { handler }
