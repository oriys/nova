#!/usr/bin/env python3
"""
Persistent mode database function with connection reuse.

This function runs as a long-lived process, reading JSON requests from stdin
and writing JSON responses to stdout. Database connections are maintained
across invocations.

Protocol:
  Input:  {"input": {...}}\n
  Output: {"output": {...}}\n  or  {"error": "..."}\n

Usage:
  nova register db-persistent \
    --runtime python \
    --code examples/db_persistent.py \
    --env DB_HOST=172.30.0.1 \
    --env DB_USER=nova \
    --env DB_PASSWORD=secret
"""

import json
import sys
import os

# Global connection - reused across invocations
_db_conn = None


def get_connection():
    """Get or create database connection"""
    global _db_conn

    if _db_conn is not None:
        try:
            _db_conn.cursor().execute("SELECT 1")
            return _db_conn
        except:
            _db_conn = None

    import psycopg2

    _db_conn = psycopg2.connect(
        host=os.environ.get("DB_HOST", "172.30.0.1"),
        port=int(os.environ.get("DB_PORT", "5432")),
        dbname=os.environ.get("DB_NAME", "mydb"),
        user=os.environ.get("DB_USER", "nova"),
        password=os.environ.get("DB_PASSWORD", "secret"),
    )
    _db_conn.autocommit = True
    print(f"[db] Connection established", file=sys.stderr)
    return _db_conn


def handler(event):
    """Execute database query"""
    query = event.get("query", "SELECT 1 as result")
    params = event.get("params", [])

    conn = get_connection()
    cur = conn.cursor()

    try:
        cur.execute(query, params)

        if cur.description:
            columns = [desc[0] for desc in cur.description]
            rows = [dict(zip(columns, row)) for row in cur.fetchall()]
            return {"rows": rows, "count": len(rows)}
        else:
            return {"affected": cur.rowcount}
    except Exception as e:
        return {"error": str(e)}
    finally:
        cur.close()


def run_persistent():
    """Persistent mode: read requests from stdin, write responses to stdout"""
    print("[persistent] Starting persistent mode", file=sys.stderr)

    for line in sys.stdin:
        try:
            req = json.loads(line.strip())
            event = req.get("input", {})

            result = handler(event)

            response = {"output": result}
            print(json.dumps(response), flush=True)

        except Exception as e:
            print(json.dumps({"error": str(e)}), flush=True)


def run_single(input_file):
    """Single invocation mode: read from file, write to stdout"""
    with open(input_file) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))


if __name__ == "__main__":
    if "--persistent" in sys.argv or os.environ.get("NOVA_MODE") == "persistent":
        run_persistent()
    else:
        input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"
        run_single(input_file)
