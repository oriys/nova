"""
Persistent mode database function with connection reuse.

This function maintains database connections across invocations.
The bootstrap handles persistent mode stdin/stdout protocol automatically.

Usage:
  nova register db-persistent \
    --runtime python \
    --code examples/db_persistent.py \
    --mode persistent \
    --env DB_HOST=172.30.0.1 \
    --env DB_USER=nova \
    --env DB_PASSWORD=secret
"""

import os
import sys

_db_conn = None


def get_connection():
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
    print("[db] Connection established", file=sys.stderr)
    return _db_conn


def handler(event, context):
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
