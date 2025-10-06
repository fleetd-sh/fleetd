import { type NextRequest, NextResponse } from "next/server";

const LOKI_URL = process.env.LOKI_URL || "http://localhost:3100";

// Proxy requests to Loki
export async function GET(req: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  try {
    const params = await context.params;
    const path = params.path.join("/");
    const searchParams = req.nextUrl.searchParams.toString();
    const url = `${LOKI_URL}/loki/api/v1/${path}${searchParams ? `?${searchParams}` : ""}`;

    const response = await fetch(url, {
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      return NextResponse.json(
        { error: `Loki returned ${response.status}` },
        { status: response.status },
      );
    }

    const data = await response.json();
    return NextResponse.json(data);
  } catch (error) {
    console.error("Loki proxy error:", error);
    return NextResponse.json({ error: "Failed to fetch logs" }, { status: 500 });
  }
}

// Support POST for push endpoint
export async function POST(req: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  try {
    const params = await context.params;
    const path = params.path.join("/");
    const body = await req.json();
    const url = `${LOKI_URL}/loki/api/v1/${path}`;

    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });

    if (!response.ok) {
      return NextResponse.json(
        { error: `Loki returned ${response.status}` },
        { status: response.status },
      );
    }

    // Push endpoint returns 204 No Content on success
    if (response.status === 204) {
      return new NextResponse(null, { status: 204 });
    }

    const data = await response.json();
    return NextResponse.json(data);
  } catch (error) {
    console.error("Loki proxy error:", error);
    return NextResponse.json({ error: "Failed to send logs" }, { status: 500 });
  }
}
