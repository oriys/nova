# Example Ruby function for Nova serverless platform

def handler(event, context)
  name = event['name']
  name = "Anonymous" if name.nil? || name == ""
  {
    message: "Hello, #{name}!",
    runtime: "ruby",
    request_id: context['request_id'],
  }
end
