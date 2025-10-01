import { NextRequest, NextResponse } from "next/server";

// Platform API URL - uses Docker DNS when running server-side
const PLATFORM_API_URL = process.env.PLATFORM_API_URL || "http://platform-api:8080";
const DEVICE_API_URL = process.env.DEVICE_API_URL || "http://device-api:8081";

/**
 * API Proxy Route
 * Proxies all /api/* requests to the appropriate backend service
 */
export async function GET(
  request: NextRequest,
  context: { params: Promise<{ proxy: string[] }> }
) {
  const params = await context.params;
  return handleRequest(request, params.proxy, "GET");
}

export async function POST(
  request: NextRequest,
  context: { params: Promise<{ proxy: string[] }> }
) {
  const params = await context.params;
  return handleRequest(request, params.proxy, "POST");
}

export async function PUT(
  request: NextRequest,
  context: { params: Promise<{ proxy: string[] }> }
) {
  const params = await context.params;
  return handleRequest(request, params.proxy, "PUT");
}

export async function DELETE(
  request: NextRequest,
  context: { params: Promise<{ proxy: string[] }> }
) {
  const params = await context.params;
  return handleRequest(request, params.proxy, "DELETE");
}

export async function PATCH(
  request: NextRequest,
  context: { params: Promise<{ proxy: string[] }> }
) {
  const params = await context.params;
  return handleRequest(request, params.proxy, "PATCH");
}

async function handleRequest(
  request: NextRequest,
  pathSegments: string[],
  method: string
) {
  try {
    // Determine which backend service to proxy to
    const path = pathSegments.join("/");
    let backendUrl = PLATFORM_API_URL;

    // Route to device-api for device-specific endpoints
    if (path.startsWith("device/") || path.startsWith("agents/")) {
      backendUrl = DEVICE_API_URL;
    }

    // Build the target URL
    const targetUrl = `${backendUrl}/${path}${request.nextUrl.search}`;

    // Forward headers
    const headers = new Headers();
    request.headers.forEach((value, key) => {
      // Skip Next.js specific headers
      if (!key.startsWith("x-") && key !== "host") {
        headers.set(key, value);
      }
    });

    // Add authorization if present
    const authHeader = request.headers.get("authorization");
    if (authHeader) {
      headers.set("authorization", authHeader);
    }

    // Prepare request options
    const requestOptions: RequestInit = {
      method,
      headers,
    };

    // Add body for non-GET requests
    if (method !== "GET" && method !== "HEAD") {
      const contentType = request.headers.get("content-type");
      if (contentType?.includes("application/json")) {
        requestOptions.body = JSON.stringify(await request.json());
      } else if (contentType?.includes("multipart/form-data")) {
        requestOptions.body = await request.formData();
      } else {
        requestOptions.body = await request.text();
      }
    }

    // Make the request to the backend
    const response = await fetch(targetUrl, requestOptions);

    // Forward the response
    const responseHeaders = new Headers();
    response.headers.forEach((value, key) => {
      // Forward all headers except connection-specific ones
      if (!["connection", "transfer-encoding"].includes(key.toLowerCase())) {
        responseHeaders.set(key, value);
      }
    });

    // Add CORS headers for browser access
    responseHeaders.set("Access-Control-Allow-Origin", "*");
    responseHeaders.set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS");
    responseHeaders.set("Access-Control-Allow-Headers", "Content-Type, Authorization");

    // Handle the response based on content type
    const contentType = response.headers.get("content-type");

    if (contentType?.includes("application/json")) {
      const data = await response.json();
      return NextResponse.json(data, {
        status: response.status,
        headers: responseHeaders,
      });
    } else if (contentType?.includes("text/")) {
      const text = await response.text();
      return new NextResponse(text, {
        status: response.status,
        headers: responseHeaders,
      });
    } else {
      // For binary data or other content types
      const blob = await response.blob();
      return new NextResponse(blob, {
        status: response.status,
        headers: responseHeaders,
      });
    }
  } catch (error) {
    console.error("API Proxy Error:", error);
    return NextResponse.json(
      {
        error: "Failed to proxy request",
        details: error instanceof Error ? error.message : "Unknown error"
      },
      { status: 500 }
    );
  }
}

// Handle preflight requests
export async function OPTIONS() {
  return new NextResponse(null, {
    status: 200,
    headers: {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, PATCH, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type, Authorization",
    },
  });
}