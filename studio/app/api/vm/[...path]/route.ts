import { type NextRequest, NextResponse } from "next/server";

const VICTORIA_METRICS_URL = process.env.VICTORIA_METRICS_URL || "http://localhost:8428";

// Proxy requests to VictoriaMetrics
export async function GET(req: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  try {
    const params = await context.params;
    const path = params.path.join("/");
    const searchParams = req.nextUrl.searchParams.toString();
    const url = `${VICTORIA_METRICS_URL}/api/v1/${path}${searchParams ? `?${searchParams}` : ""}`;

    const response = await fetch(url, {
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      return NextResponse.json(
        { error: `VictoriaMetrics returned ${response.status}` },
        { status: response.status },
      );
    }

    const data = await response.json();
    return NextResponse.json(data);
  } catch (error) {
    console.error("VictoriaMetrics proxy error:", error);
    return NextResponse.json({ error: "Failed to fetch metrics" }, { status: 500 });
  }
}

// Support POST for certain endpoints (e.g., /api/v1/import)
export async function POST(req: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  try {
    const params = await context.params;
    const path = params.path.join("/");
    const body = await req.text();
    const url = `${VICTORIA_METRICS_URL}/api/v1/${path}`;

    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": req.headers.get("content-type") || "application/json",
      },
      body,
    });

    if (!response.ok) {
      return NextResponse.json(
        { error: `VictoriaMetrics returned ${response.status}` },
        { status: response.status },
      );
    }

    const data = await response.text();
    return new NextResponse(data, {
      status: response.status,
      headers: {
        "Content-Type": response.headers.get("content-type") || "application/json",
      },
    });
  } catch (error) {
    console.error("VictoriaMetrics proxy error:", error);
    return NextResponse.json({ error: "Failed to send metrics" }, { status: 500 });
  }
}
