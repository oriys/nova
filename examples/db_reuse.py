#!/usr/bin/env python3
"""
Database function with connection reuse across invocations.

This function maintains a persistent database connection that survives
across multiple invocations within the same VM lifecycle.

The connection is established on first use and reused for subsequent calls.
"""

import json
import sys
import os

# 全局连接 - 在 VM 生命周期内复用
_db_conn = None


def get_connection():
    """获取或创建数据库连接"""
    global _db_conn

    if _db_conn is not None:
        try:
            # 检查连接是否有效
            _db_conn.cursor().execute("SELECT 1")
            return _db_conn
        except:
            _db_conn = None

    # 创建新连接
    import psycopg2

    _db_conn = psycopg2.connect(
        host=os.environ.get("DB_HOST", "172.30.0.1"),
        port=int(os.environ.get("DB_PORT", "5432")),
        dbname=os.environ.get("DB_NAME", "mydb"),
        user=os.environ.get("DB_USER", "nova"),
        password=os.environ.get("DB_PASSWORD", "secret"),
    )
    _db_conn.autocommit = True
    print(f"[db] New connection established", file=sys.stderr)
    return _db_conn


def handler(event):
    """
    执行数据库查询

    连接复用策略：
    - 第一次调用：建立连接 (~50-100ms)
    - 后续调用：复用连接 (~1-5ms)
    """
    query = event.get("query", "SELECT 1 as result")
    params = event.get("params", [])

    conn = get_connection()
    cur = conn.cursor()

    try:
        cur.execute(query, params)

        if cur.description:
            columns = [desc[0] for desc in cur.description]
            rows = [dict(zip(columns, row)) for row in cur.fetchall()]
            return {
                "success": True,
                "rows": rows,
                "count": len(rows),
            }
        else:
            return {
                "success": True,
                "affected": cur.rowcount,
            }
    except Exception as e:
        return {
            "success": False,
            "error": str(e),
        }
    finally:
        cur.close()


if __name__ == "__main__":
    input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"
    with open(input_file) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))
