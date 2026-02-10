import { NextRequest, NextResponse } from "next/server";

const BACKEND_URL =
  process.env.BACKEND_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  process.env.ZENITH_URL ||
  "http://localhost:9000";

async function handler(req: NextRequest) {
  const path = req.nextUrl.pathname.replace(/^\/api/, "");
  const search = req.nextUrl.search;
  const targetUrl = `${BACKEND_URL}${path}${search}`;

  const headers = new Headers();
  req.headers.forEach((value, key) => {
    if (key.toLowerCase() !== "host") {
      headers.set(key, value);
    }
  });

  let body: BodyInit | undefined;
  if (req.method !== "GET" && req.method !== "HEAD") {
    const contentType = req.headers.get("content-type") || "";
    if (contentType.includes("multipart/form-data")) {
      // Preserve raw binary data for multipart uploads
      body = await req.blob();
    } else {
      body = await req.text();
    }
  }

  try {
    const response = await fetch(targetUrl, {
      method: req.method,
      headers,
      body,
    });

    const responseHeaders = new Headers();
    response.headers.forEach((value, key) => {
      if (!["content-encoding", "transfer-encoding"].includes(key.toLowerCase())) {
        responseHeaders.set(key, value);
      }
    });

    return new NextResponse(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
    });
  } catch (error) {
    console.error("Proxy error:", error);
    return NextResponse.json(
      { error: "Backend unavailable" },
      { status: 502 }
    );
  }
}

export const GET = handler;
export const POST = handler;
export const PUT = handler;
export const PATCH = handler;
export const DELETE = handler;
export const OPTIONS = handler;
