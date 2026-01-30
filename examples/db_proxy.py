#!/usr/bin/env python3
"""Database function using connection pool proxy"""

import json
import sys
import socket

# 连接池代理地址 (宿主机网关)
PROXY_HOST = "172.30.0.1"
PROXY_PORT = 6432  # PgBouncer 端口


def handler(event):
    """
    通过连接池代理访问数据库
    VM 每次建立短连接到代理，代理复用长连接到数据库
    """
    import psycopg2

    query = event.get("query", "SELECT 1")

    # 连接到代理而非直接连数据库
    conn = psycopg2.connect(
        host=PROXY_HOST,
        port=PROXY_PORT,
        dbname="mydb",
        user="nova",
        password="secret",
        # 关键：短连接模式，用完即关
        connect_timeout=5,
    )

    try:
        cur = conn.cursor()
        cur.execute(query)

        if cur.description:
            columns = [desc[0] for desc in cur.description]
            rows = [dict(zip(columns, row)) for row in cur.fetchall()]
            return {"rows": rows, "count": len(rows)}
        else:
            return {"affected": cur.rowcount}
    finally:
        conn.close()  # 关闭到代理的连接，代理保持到DB的连接


if __name__ == "__main__":
    input_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/input.json"
    with open(input_file) as f:
        event = json.load(f)
    print(json.dumps(handler(event)))
