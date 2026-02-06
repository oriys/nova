// Example Deno function for Nova serverless platform

export function handler(event, context) {
  const name = typeof event?.name === "string" && event.name ? event.name : "Anonymous"
  return {
    message: `Hello, ${name}!`,
    runtime: "deno",
    requestId: context.requestId,
  }
}
