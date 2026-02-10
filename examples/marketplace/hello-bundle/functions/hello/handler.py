import json
import os
import sys

def handler(event):
    """Simple hello world handler"""
    greeting = os.getenv('GREETING', 'Hello')
    name = event.get('name', 'World')
    return {
        'message': f'{greeting}, {name}!',
        'timestamp': event.get('timestamp', None)
    }

if __name__ == '__main__':
    # Read input from file
    if len(sys.argv) < 2:
        print('Usage: handler.py <input.json>')
        sys.exit(1)
    
    with open(sys.argv[1], 'r') as f:
        event = json.load(f)
    
    result = handler(event)
    print(json.dumps(result))
