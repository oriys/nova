// Example Deno function for Nova (AWS Lambda-compatible signature)

export function handler(event, context) {
  const name = typeof event?.name === "string" && event.name ? event.name : "Anonymous"
  return {
    message: `Hello, ${name}!`,
    runtime: "deno",
    requestId: context.requestId,
  }
}
