import { useCallback, useEffect, useRef, useState } from "react";
import { EditorView, basicSetup } from "codemirror";
import { StreamLanguage } from "@codemirror/language";
import { ripBasic } from "./basic";
import { samples } from "./samples";
import { RequestTester, RouteHint, extractRouteHint } from "./RequestTester";

type RunResponse = {
  id?: string;
  url?: string;
  status?: string;
  error?: string;
  diagnostics?: string[];
  log?: string[];
};

type LogEvent = {
  seq: number;
  time: string;
  stream: "stdout" | "stderr" | "request" | "host";
  text: string;
};

type LogEntry = {
  key: string;
  text: string;
  dim?: boolean;
};

function formatError(msg: string): string {
  return msg.startsWith("?") ? msg : `?${msg}`;
}

export function App() {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const logRef = useRef<HTMLElement>(null);
  const streamRef = useRef<EventSource | null>(null);
  const lastSeqRef = useRef(0);
  const localKeyRef = useRef(0);

  const [sample, setSample] = useState(samples[0].name);
  const [running, setRunning] = useState(false);
  const [url, setUrl] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [hint, setHint] = useState<RouteHint>(() => extractRouteHint(samples[0].code));
  const [log, setLog] = useState<LogEntry[]>([{ key: "init", text: "READY." }]);

  const appendLog = useCallback((lines: { text: string; dim?: boolean; key?: string }[]) => {
    setLog((prev) => [
      ...prev,
      ...lines.map((l) => ({
        key: l.key ?? `local-${++localKeyRef.current}`,
        text: l.text,
        dim: l.dim,
      })),
    ]);
  }, []);

  const closeStream = useCallback(() => {
    if (streamRef.current) {
      streamRef.current.close();
      streamRef.current = null;
    }
  }, []);

  useEffect(() => {
    if (!host.current) return;
    view.current = new EditorView({
      doc: samples[0].code,
      extensions: [basicSetup, StreamLanguage.define(ripBasic), EditorView.theme({}, { dark: true })],
      parent: host.current,
    });
    return () => {
      view.current?.destroy();
      closeStream();
    };
  }, [closeStream]);

  useEffect(() => {
    const el = logRef.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  }, [log]);

  function loadSample(name: string) {
    const s = samples.find((x) => x.name === name);
    const v = view.current;
    if (!s || !v) return;
    setSample(name);
    v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: s.code } });
    setHint(extractRouteHint(s.code));
  }

  const subscribeLogs = useCallback(
    (id: string) => {
      closeStream();
      lastSeqRef.current = 0;
      const es = new EventSource(`/api/programs/${encodeURIComponent(id)}/logs`);
      streamRef.current = es;
      es.onmessage = (ev) => {
        try {
          const line = JSON.parse(ev.data) as LogEvent;
          if (line.seq <= lastSeqRef.current) return;
          lastSeqRef.current = line.seq;
          const prefix =
            line.stream === "request" ? "[REQ] " : line.stream === "stderr" ? "[ERR] " : "";
          appendLog([
            {
              key: `sse-${id}-${line.seq}`,
              text: `${prefix}${line.text}`,
              dim: line.stream === "host",
            },
          ]);
        } catch {
          // Ignore malformed event payloads.
        }
      };
      es.onerror = () => {
        if (es.readyState === EventSource.CLOSED) {
          appendLog([{ text: "LOG STREAM CLOSED.", dim: true }]);
        }
      };
    },
    [appendLog, closeStream],
  );

  const run = useCallback(async () => {
    const code = view.current?.state.doc.toString() ?? "";
    closeStream();
    setRunning(true);
    setUrl(null);
    setStatus(null);
    setHint(extractRouteHint(code));
    appendLog([{ text: "RUN" }, { text: "COMPILING & DEPLOYING…", dim: true }]);
    try {
      const res = await fetch("/api/programs", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const body = (await res.json()) as RunResponse;
      if (res.ok && body.id && body.url) {
        const fullUrl = new URL(body.url, window.location.origin).toString();
        const progStatus = body.status ?? "RUNNING";
        setUrl(fullUrl);
        setStatus(progStatus);

        const nextLines: { text: string; dim?: boolean }[] = [];
        for (const diag of body.diagnostics ?? []) {
          nextLines.push({ text: diag, dim: true });
        }
        nextLines.push({
          text:
            progStatus === "ENDED"
              ? `DEPLOYED AT ${body.url} (ENDED WITHOUT SERVE)`
              : `LIVE AT ${body.url}`,
        });
        appendLog(nextLines);
        subscribeLogs(body.id);
      } else {
        const errLines: { text: string; dim?: boolean }[] = [];
        if (body.diagnostics && body.diagnostics.length > 0) {
          for (const d of body.diagnostics) errLines.push({ text: d });
        } else {
          errLines.push({ text: formatError(body.error ?? `HTTP ${res.status}`) });
        }
        for (const line of body.log ?? []) {
          errLines.push({ text: line, dim: true });
        }
        errLines.push({ text: "READY." });
        appendLog(errLines);
      }
    } catch (err) {
      appendLog([{ text: `?NETWORK ERROR — ${String(err)}` }, { text: "READY." }]);
    } finally {
      setRunning(false);
    }
  }, [appendLog, closeStream, subscribeLogs]);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter" && !running) {
        e.preventDefault();
        void run();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [run, running]);

  return (
    <div className="crt">
      <header>
        <h1>REST IN PEACE</h1>
        <p>BASIC, RAISED FROM THE GRAVE. RUN IT AND GET A LIVE API.</p>
      </header>

      <div className="toolbar">
        <label>
          SAMPLE{" "}
          <select value={sample} onChange={(e) => loadSample(e.target.value)}>
            {samples.map((s) => (
              <option key={s.name}>{s.name}</option>
            ))}
          </select>
        </label>
        <button type="button" onClick={run} disabled={running}>
          {running ? "RUNNING…" : "RUN"}
        </button>
        <button
          type="button"
          onClick={() => setLog([{ key: `clear-${++localKeyRef.current}`, text: "READY." }])}
        >
          CLEAR LOG
        </button>
        {url && (
          <a href={url} target="_blank" rel="noreferrer">
            {url}
          </a>
        )}
        {status && <span className="tester-hint">[{status}]</span>}
      </div>

      <main>
        <section className="editor" ref={host} aria-label="BASIC editor" />
        <section className="log" ref={logRef} aria-label="Program log" aria-live="polite">
          {log.map((line) => (
            <div key={line.key} className={line.dim ? "log-line-dim" : undefined}>
              {line.text}
            </div>
          ))}
          <span className="cursor" aria-hidden="true">
            █
          </span>
        </section>
      </main>

      <RequestTester baseUrl={url} hint={hint} disabled={status === "ENDED"} />
    </div>
  );
}
