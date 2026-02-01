export interface FunctionData {
  id: string
  name: string
  runtime: string
  status: "active" | "inactive" | "error"
  memory: number
  timeout: number
  invocations: number
  errors: number
  avgDuration: number
  lastModified: string
  region: string
  handler: string
  description?: string
  code?: string
}

export interface LogEntry {
  id: string
  functionId: string
  functionName: string
  timestamp: string
  level: "info" | "warn" | "error" | "debug"
  message: string
  requestId: string
  duration?: number
}

export interface HistoryEntry {
  id: string
  functionId: string
  functionName: string
  action: "deploy" | "update" | "delete" | "rollback" | "config_change"
  timestamp: string
  user: string
  version: string
  details: string
}

export interface RuntimeInfo {
  id: string
  name: string
  version: string
  status: "available" | "deprecated" | "maintenance"
  functionsCount: number
  icon: string
}

export interface ConfigItem {
  id: string
  key: string
  value: string
  type: "string" | "number" | "boolean" | "secret"
  scope: "global" | "function"
  functionId?: string
  lastModified: string
}

// Mock Functions
export const mockFunctions: FunctionData[] = [
  {
    id: "fn-001",
    name: "user-auth-service",
    runtime: "Node.js 20.x",
    status: "active",
    memory: 128,
    timeout: 30,
    invocations: 45230,
    errors: 12,
    avgDuration: 45,
    lastModified: "2024-01-15T10:30:00Z",
    region: "us-east-1",
    handler: "index.handler",
    description: "Handles user authentication and session management",
    code: `export async function handler(event, context) {
  const { action, payload } = JSON.parse(event.body);
  
  switch (action) {
    case 'login':
      return await handleLogin(payload);
    case 'logout':
      return await handleLogout(payload);
    default:
      return { statusCode: 400, body: 'Invalid action' };
  }
}`,
  },
  {
    id: "fn-002",
    name: "payment-processor",
    runtime: "Python 3.11",
    status: "active",
    memory: 256,
    timeout: 60,
    invocations: 12450,
    errors: 3,
    avgDuration: 120,
    lastModified: "2024-01-14T15:45:00Z",
    region: "us-east-1",
    handler: "main.process_payment",
    description: "Processes payment transactions via Stripe",
  },
  {
    id: "fn-003",
    name: "email-sender",
    runtime: "Node.js 18.x",
    status: "active",
    memory: 128,
    timeout: 30,
    invocations: 8920,
    errors: 5,
    avgDuration: 200,
    lastModified: "2024-01-13T08:20:00Z",
    region: "eu-west-1",
    handler: "index.sendEmail",
    description: "Sends transactional emails via SendGrid",
  },
  {
    id: "fn-004",
    name: "image-resizer",
    runtime: "Go 1.21",
    status: "error",
    memory: 512,
    timeout: 120,
    invocations: 3400,
    errors: 156,
    avgDuration: 350,
    lastModified: "2024-01-12T22:10:00Z",
    region: "us-west-2",
    handler: "main.ResizeImage",
    description: "Resizes and optimizes uploaded images",
  },
  {
    id: "fn-005",
    name: "data-aggregator",
    runtime: "Python 3.11",
    status: "inactive",
    memory: 1024,
    timeout: 300,
    invocations: 890,
    errors: 0,
    avgDuration: 5000,
    lastModified: "2024-01-10T16:00:00Z",
    region: "us-east-1",
    handler: "aggregator.run",
    description: "Aggregates analytics data from multiple sources",
  },
  {
    id: "fn-006",
    name: "webhook-handler",
    runtime: "Node.js 20.x",
    status: "active",
    memory: 128,
    timeout: 15,
    invocations: 67800,
    errors: 24,
    avgDuration: 25,
    lastModified: "2024-01-15T12:00:00Z",
    region: "us-east-1",
    handler: "index.handleWebhook",
    description: "Handles incoming webhooks from third-party services",
  },
]

// Mock Logs
export const mockLogs: LogEntry[] = [
  {
    id: "log-001",
    functionId: "fn-001",
    functionName: "user-auth-service",
    timestamp: "2024-01-15T14:30:45.123Z",
    level: "info",
    message: "User login successful for user_id: usr_12345",
    requestId: "req-abc123",
    duration: 42,
  },
  {
    id: "log-002",
    functionId: "fn-004",
    functionName: "image-resizer",
    timestamp: "2024-01-15T14:30:42.456Z",
    level: "error",
    message: "Failed to process image: Invalid format detected",
    requestId: "req-def456",
    duration: 150,
  },
  {
    id: "log-003",
    functionId: "fn-002",
    functionName: "payment-processor",
    timestamp: "2024-01-15T14:30:40.789Z",
    level: "info",
    message: "Payment processed successfully: $49.99",
    requestId: "req-ghi789",
    duration: 115,
  },
  {
    id: "log-004",
    functionId: "fn-003",
    functionName: "email-sender",
    timestamp: "2024-01-15T14:30:38.012Z",
    level: "warn",
    message: "Email delivery delayed: Rate limit approaching",
    requestId: "req-jkl012",
    duration: 220,
  },
  {
    id: "log-005",
    functionId: "fn-006",
    functionName: "webhook-handler",
    timestamp: "2024-01-15T14:30:35.345Z",
    level: "debug",
    message: "Webhook received from Stripe: evt_payment_intent_succeeded",
    requestId: "req-mno345",
    duration: 18,
  },
  {
    id: "log-006",
    functionId: "fn-001",
    functionName: "user-auth-service",
    timestamp: "2024-01-15T14:30:32.678Z",
    level: "error",
    message: "Authentication failed: Invalid credentials",
    requestId: "req-pqr678",
    duration: 35,
  },
  {
    id: "log-007",
    functionId: "fn-002",
    functionName: "payment-processor",
    timestamp: "2024-01-15T14:30:30.901Z",
    level: "info",
    message: "Refund initiated for transaction txn_98765",
    requestId: "req-stu901",
    duration: 98,
  },
  {
    id: "log-008",
    functionId: "fn-004",
    functionName: "image-resizer",
    timestamp: "2024-01-15T14:30:28.234Z",
    level: "info",
    message: "Image resized successfully: 1920x1080 -> 640x360",
    requestId: "req-vwx234",
    duration: 280,
  },
]

// Mock History
export const mockHistory: HistoryEntry[] = [
  {
    id: "hist-001",
    functionId: "fn-001",
    functionName: "user-auth-service",
    action: "deploy",
    timestamp: "2024-01-15T10:30:00Z",
    user: "developer@example.com",
    version: "v1.2.3",
    details: "Added OAuth2 support",
  },
  {
    id: "hist-002",
    functionId: "fn-002",
    functionName: "payment-processor",
    action: "config_change",
    timestamp: "2024-01-14T15:45:00Z",
    user: "admin@example.com",
    version: "v2.1.0",
    details: "Updated Stripe API key",
  },
  {
    id: "hist-003",
    functionId: "fn-004",
    functionName: "image-resizer",
    action: "rollback",
    timestamp: "2024-01-14T12:00:00Z",
    user: "developer@example.com",
    version: "v1.0.5",
    details: "Rolled back due to memory issues",
  },
  {
    id: "hist-004",
    functionId: "fn-003",
    functionName: "email-sender",
    action: "update",
    timestamp: "2024-01-13T08:20:00Z",
    user: "developer@example.com",
    version: "v1.1.0",
    details: "Updated email templates",
  },
  {
    id: "hist-005",
    functionId: "fn-005",
    functionName: "data-aggregator",
    action: "deploy",
    timestamp: "2024-01-10T16:00:00Z",
    user: "admin@example.com",
    version: "v1.0.0",
    details: "Initial deployment",
  },
]

// Mock Runtimes
export const mockRuntimes: RuntimeInfo[] = [
  {
    id: "rt-001",
    name: "Node.js",
    version: "20.x",
    status: "available",
    functionsCount: 3,
    icon: "nodejs",
  },
  {
    id: "rt-002",
    name: "Node.js",
    version: "18.x",
    status: "available",
    functionsCount: 1,
    icon: "nodejs",
  },
  {
    id: "rt-003",
    name: "Python",
    version: "3.11",
    status: "available",
    functionsCount: 2,
    icon: "python",
  },
  {
    id: "rt-004",
    name: "Go",
    version: "1.21",
    status: "available",
    functionsCount: 1,
    icon: "go",
  },
  {
    id: "rt-005",
    name: "Python",
    version: "3.9",
    status: "deprecated",
    functionsCount: 0,
    icon: "python",
  },
  {
    id: "rt-006",
    name: "Node.js",
    version: "16.x",
    status: "deprecated",
    functionsCount: 0,
    icon: "nodejs",
  },
]

// Mock Configurations
export const mockConfigs: ConfigItem[] = [
  {
    id: "cfg-001",
    key: "DATABASE_URL",
    value: "postgresql://...",
    type: "secret",
    scope: "global",
    lastModified: "2024-01-15T10:00:00Z",
  },
  {
    id: "cfg-002",
    key: "API_TIMEOUT",
    value: "30000",
    type: "number",
    scope: "global",
    lastModified: "2024-01-14T14:00:00Z",
  },
  {
    id: "cfg-003",
    key: "STRIPE_SECRET_KEY",
    value: "sk_live_...",
    type: "secret",
    scope: "function",
    functionId: "fn-002",
    lastModified: "2024-01-14T15:45:00Z",
  },
  {
    id: "cfg-004",
    key: "ENABLE_LOGGING",
    value: "true",
    type: "boolean",
    scope: "global",
    lastModified: "2024-01-13T09:00:00Z",
  },
  {
    id: "cfg-005",
    key: "SENDGRID_API_KEY",
    value: "SG...",
    type: "secret",
    scope: "function",
    functionId: "fn-003",
    lastModified: "2024-01-13T08:20:00Z",
  },
  {
    id: "cfg-006",
    key: "MAX_RETRIES",
    value: "3",
    type: "number",
    scope: "global",
    lastModified: "2024-01-12T11:00:00Z",
  },
]

// Chart data generators
export function generateInvocationData(hours: number = 24) {
  const data = []
  const now = new Date()
  
  for (let i = hours; i >= 0; i--) {
    const time = new Date(now.getTime() - i * 60 * 60 * 1000)
    data.push({
      time: time.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
      invocations: Math.floor(Math.random() * 500) + 100,
      errors: Math.floor(Math.random() * 20),
    })
  }
  
  return data
}

export function generateDurationData(hours: number = 24) {
  const data = []
  const now = new Date()
  
  for (let i = hours; i >= 0; i--) {
    const time = new Date(now.getTime() - i * 60 * 60 * 1000)
    data.push({
      time: time.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
      avgDuration: Math.floor(Math.random() * 150) + 20,
      p95Duration: Math.floor(Math.random() * 300) + 80,
    })
  }
  
  return data
}

export function generateMemoryData(hours: number = 24) {
  const data = []
  const now = new Date()
  
  for (let i = hours; i >= 0; i--) {
    const time = new Date(now.getTime() - i * 60 * 60 * 1000)
    data.push({
      time: time.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }),
      memoryUsed: Math.floor(Math.random() * 80) + 40,
    })
  }
  
  return data
}
