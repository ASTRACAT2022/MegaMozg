import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";

const REALM = 'Basic realm="MezaMozg Panel", charset="UTF-8"';
const SESSION_COOKIE_NAME = "meza_panel_session";

function unauthorized() {
  return new NextResponse("Authentication required", {
    status: 401,
    headers: {
      "WWW-Authenticate": REALM,
    },
  });
}

function parseBasicAuth(header: string | null): { user: string; password: string } | null {
  if (!header || !header.startsWith("Basic ")) {
    return null;
  }

  const encoded = header.slice("Basic ".length).trim();
  if (!encoded) {
    return null;
  }

  try {
    const decoded = atob(encoded);
    const delimiter = decoded.indexOf(":");
    if (delimiter < 0) {
      return null;
    }

    return {
      user: decoded.slice(0, delimiter),
      password: decoded.slice(delimiter + 1),
    };
  } catch {
    return null;
  }
}

export function proxy(request: NextRequest) {
  const enabled = (process.env.MEZA_PANEL_BASIC_AUTH_ENABLED ?? "true").toLowerCase() === "true";
  if (!enabled) {
    return NextResponse.next();
  }

  const expectedUser = process.env.MEZA_PANEL_BASIC_AUTH_USER ?? "admin";
  const expectedPassword = process.env.MEZA_PANEL_BASIC_AUTH_PASSWORD ?? "admin";
  const expectedSessionToken =
    process.env.MEZA_PANEL_BASIC_AUTH_SESSION_TOKEN ?? `${expectedUser}:${expectedPassword}`;

  const existingSession = request.cookies.get(SESSION_COOKIE_NAME)?.value;
  if (existingSession && existingSession === expectedSessionToken) {
    return NextResponse.next();
  }

  const credentials = parseBasicAuth(request.headers.get("authorization"));
  if (!credentials) {
    return unauthorized();
  }

  if (credentials.user !== expectedUser || credentials.password !== expectedPassword) {
    return unauthorized();
  }

  const response = NextResponse.next();
  const xForwardedProto = request.headers.get("x-forwarded-proto");
  const isSecureRequest = request.nextUrl.protocol === "https:" || xForwardedProto === "https";
  response.cookies.set(SESSION_COOKIE_NAME, expectedSessionToken, {
    httpOnly: true,
    sameSite: "lax",
    secure: isSecureRequest,
    path: "/",
    maxAge: 60 * 60 * 24 * 30,
  });
  return response;
}

export const config = {
  matcher: ["/((?!_next|favicon.ico).*)"],
};
