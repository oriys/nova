import { NextRequest, NextResponse } from "next/server";

export async function POST(req: NextRequest) {
  try {
    const body = await req.json();
    const {
      method,
      url,
      headers: reqHeaders,
      body: reqBody,
    } = body as {
      method: string;
      url: string;
      headers?: Record<string, string>;
      body?: string;
    };

    if (!url) {
      return NextResponse.json({ error: "URL is required" }, { status: 400 });
    }

    const fetchHeaders = new Headers();
    if (reqHeaders) {
      for (const [key, value] of Object.entries(reqHeaders)) {
        if (key && value) {
          fetchHeaders.set(key, value);
        }
      }
    }

    const startTime = Date.now();

    const response = await fetch(url, {
      method: method || "GET",
      headers: fetchHeaders,
      body: method !== "GET" && method !== "HEAD" && method !== "OPTIONS" ? reqBody : undefined,
    });

    const elapsed = Date.now() - startTime;
    const responseBody = await response.text();

    const responseHeaders: Record<string, string> = {};
    response.headers.forEach((value, key) => {
      responseHeaders[key] = value;
    });

    return NextResponse.json({
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
      body: responseBody,
      elapsed,
      size: new TextEncoder().encode(responseBody).length,
    });
  } catch (error) {
    const message =
      error instanceof Error ? error.message : "Request failed";
    return NextResponse.json(
      { error: message },
      { status: 502 }
    );
  }
}
