import { FormEvent, useEffect, useMemo, useState } from "react";
import "./tester.css";

export type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export type RouteHint = {
  method: HttpMethod;
  path: string;
  body?: string;
};

type TesterResponse = {
  status: number;
  statusText: string;
  ms: number;
  headers: [string, string][];
  body: string;
};

type Props = {
  baseUrl: string | null;
  hint: RouteHint;
  disabled?: boolean;
};

const METHODS: HttpMethod[] = ["GET", "POST", "PUT", "PATCH", "DELETE"];

/**
 * Inspects BASIC source and picks a sensible default method/path/body for the
 * request tester (first declared ROUTE, respecting any enclosing ROUTE GROUP).
 */
export function extractRouteHint(code: string): RouteHint {
  let groupPrefix = "";
  for (const raw of code.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("'") || /^REM\b/i.test(line)) continue;

    const groupMatch = line.match(/^(?:\d+\s+)?ROUTE\s+GROUP\s+"([^"]+)"/i);
    if (groupMatch) {
      groupPrefix = groupMatch[1].replace(/\/+$/, "");
      continue;
    }
    if (/^(?:\d+\s+)?END\s+GROUP\b/i.test(line)) {
      groupPrefix = "";
      continue;
    }

    const routeMatch = line.match(/^(?:\d+\s+)?ROUTE\s+(GET|POST|PUT|PATCH|DELETE)\s+"([^"]+)"/i);
    if (routeMatch) {
      const method = routeMatch[1].toUpperCase() as HttpMethod;
      const routePath = routeMatch[2].startsWith("/") ? routeMatch[2] : `/${routeMatch[2]}`;
      const fullPath = `${groupPrefix}${routePath}`;
      return {
        method,
        path: fullPath,
        body: defaultBodyFor(method, fullPath),
      };
    }
  }
  return { method: "GET", path: "/hello" };
}

function defaultBodyFor(method: HttpMethod, path: string): string | undefined {
  if (method === "GET" || method === "DELETE") return undefined;
  if (path.includes("invoice")) {
    return JSON.stringify({ items: [{ quantity: 2, unitPrice: "19.99" }] }, null, 2);
  }
  if (path.includes("todo")) {
    return JSON.stringify({ title: "RISE FROM THE GRAVE" }, null, 2);
  }
  return "{}";
}

function normalizePath(path: string): string {
  const trimmed = path.trim();
  if (!trimmed) return "/";
  return trimmed.startsWith("/") ? trimmed : `/${trimmed}`;
}

function parseHeaders(raw: string): [string, string][] {
  const out: [string, string][] = [];
  for (const line of raw.split(/\r?\n/)) {
    const idx = line.indexOf(":");
    if (idx <= 0) continue;
    const name = line.slice(0, idx).trim();
    const value = line.slice(idx + 1).trim();
    if (name) out.push([name, value]);
  }
  return out;
}

function shellQuote(s: string): string {
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

export function buildCurlCommand(
  baseUrl: string | null,
  method: HttpMethod,
  path: string,
  headersRaw: string,
  body: string,
): string {
  const origin = (baseUrl ?? `${window.location.origin}/p/<programId>/`).replace(/\/+$/, "");
  const target = `${origin}${normalizePath(path)}`;
  const parts: string[] = ["curl"];
  if (method !== "GET") {
    parts.push("-X", method);
  }

  const headers = parseHeaders(headersRaw);
  const hasContentType = headers.some(([k]) => k.toLowerCase() === "content-type");
  const sendBody = method !== "GET" && method !== "DELETE" && body.trim() !== "";

  if (sendBody && !hasContentType) {
    parts.push("-H", shellQuote("Content-Type: application/json"));
  }
  for (const [k, v] of headers) {
    parts.push("-H", shellQuote(`${k}: ${v}`));
  }
  if (sendBody) {
    parts.push("-d", shellQuote(body));
  }
  parts.push(shellQuote(target));
  return parts.join(" ");
}

export function RequestTester({ baseUrl, hint, disabled }: Props) {
  const [method, setMethod] = useState<HttpMethod>(hint.method);
  const [path, setPath] = useState(hint.path);
  const [headersRaw, setHeadersRaw] = useState("");
  const [body, setBody] = useState(hint.body ?? "");
  const [sending, setSending] = useState(false);
  const [copied, setCopied] = useState(false);
  const [response, setResponse] = useState<TesterResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setMethod(hint.method);
    setPath(hint.path);
    setBody(hint.body ?? "");
    setResponse(null);
    setError(null);
  }, [hint.method, hint.path, hint.body]);

  const curlCmd = useMemo(
    () => buildCurlCommand(baseUrl, method, path, headersRaw, body),
    [baseUrl, method, path, headersRaw, body],
  );

  async function sendRequest(e: FormEvent) {
    e.preventDefault();
    if (!baseUrl || sending || disabled) return;

    setSending(true);
    setError(null);
    setResponse(null);

    const target = `${baseUrl.replace(/\/+$/, "")}${normalizePath(path)}`;
    const reqHeaders = new Headers();
    const parsed = parseHeaders(headersRaw);
    for (const [k, v] of parsed) {
      reqHeaders.append(k, v);
    }

    const sendBody = method !== "GET" && method !== "DELETE" && body.trim() !== "";
    if (sendBody && !reqHeaders.has("content-type")) {
      reqHeaders.set("content-type", "application/json");
    }

    const started = performance.now();
    try {
      const res = await fetch(target, {
        method,
        headers: reqHeaders,
        body: sendBody ? body : undefined,
      });
      const ms = Math.round(performance.now() - started);
      const rawText = await res.text();
      let formatted = rawText;
      if (rawText) {
        try {
          formatted = JSON.stringify(JSON.parse(rawText), null, 2);
        } catch {
          formatted = rawText;
        }
      }
      const resHeaders: [string, string][] = [];
      res.headers.forEach((v, k) => resHeaders.push([k, v]));
      setResponse({
        status: res.status,
        statusText: res.statusText,
        ms,
        headers: resHeaders,
        body: formatted || "(EMPTY BODY)",
      });
    } catch (err) {
      setError(`?NETWORK ERROR — ${String(err)}`);
    } finally {
      setSending(false);
    }
  }

  async function copyCurl() {
    try {
      await navigator.clipboard.writeText(curlCmd);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  }

  return (
    <section className="tester" aria-label="Request tester">
      <div className="tester-header">
        <h2>REQUEST TESTER</h2>
        <span className="tester-hint">
          {baseUrl ? `TARGET: ${baseUrl}` : "CLICK RUN TO DEPLOY A LIVE ENDPOINT"}
        </span>
      </div>

      <form onSubmit={sendRequest}>
        <div className="tester-row">
          <select
            aria-label="HTTP method"
            value={method}
            onChange={(e) => setMethod(e.target.value as HttpMethod)}
          >
            {METHODS.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
          <input
            className="tester-path"
            type="text"
            aria-label="Request path"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="/hello"
          />
          <button type="submit" disabled={!baseUrl || sending || disabled}>
            {sending ? "SENDING…" : "SEND"}
          </button>
        </div>

        <div className="tester-inputs">
          <label className="tester-field">
            HEADERS (ONE &quot;NAME: VALUE&quot; PER LINE)
            <textarea
              value={headersRaw}
              onChange={(e) => setHeadersRaw(e.target.value)}
              placeholder={"x-api-key: demo\naccept: application/json"}
              spellCheck={false}
            />
          </label>
          <label className="tester-field">
            BODY (JSON)
            <textarea
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder='{"title": "HELLO"}'
              spellCheck={false}
            />
          </label>
        </div>
      </form>

      <div className="tester-curl">
        <pre aria-label="curl command">{curlCmd}</pre>
        <button type="button" onClick={copyCurl}>
          {copied ? "COPIED" : "COPY CURL"}
        </button>
      </div>

      {error && (
        <div className="tester-response" role="alert">
          <pre>{error}</pre>
        </div>
      )}

      {response && (
        <div className="tester-response" aria-live="polite">
          <div className="tester-status">
            <strong>
              HTTP {response.status} {response.statusText}
            </strong>
            <span className="tester-hint">{response.ms} MS</span>
          </div>
          {response.headers.length > 0 && (
            <details className="tester-headers">
              <summary>RESPONSE HEADERS ({response.headers.length})</summary>
              <div>
                {response.headers.map(([k, v]) => (
                  <div key={k}>
                    {k}: {v}
                  </div>
                ))}
              </div>
            </details>
          )}
          <pre>{response.body}</pre>
        </div>
      )}
    </section>
  );
}
