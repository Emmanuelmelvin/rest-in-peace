import { useEffect, useRef, useState } from "react";
import { EditorView, basicSetup } from "codemirror";
import { StreamLanguage } from "@codemirror/language";
import { ripBasic } from "./basic";
import { samples } from "./samples";

type RunResponse = { id?: string; url?: string; error?: string };

export function App() {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const [sample, setSample] = useState(samples[0].name);
  const [running, setRunning] = useState(false);
  const [url, setUrl] = useState<string | null>(null);
  const [log, setLog] = useState<string[]>(["READY."]);

  useEffect(() => {
    if (!host.current) return;
    view.current = new EditorView({
      doc: samples[0].code,
      extensions: [basicSetup, StreamLanguage.define(ripBasic), EditorView.theme({}, { dark: true })],
      parent: host.current,
    });
    return () => view.current?.destroy();
  }, []);

  function loadSample(name: string) {
    const s = samples.find((x) => x.name === name);
    const v = view.current;
    if (!s || !v) return;
    setSample(name);
    v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: s.code } });
  }

  async function run() {
    const code = view.current?.state.doc.toString() ?? "";
    setRunning(true);
    setUrl(null);
    setLog((l) => [...l, "RUN"]);
    try {
      // TODO(task 5.2): the host compiles, builds and deploys; then subscribe to
      // /api/programs/<id>/logs over SSE.
      const res = await fetch("/api/programs", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const body = (await res.json()) as RunResponse;
      if (res.ok && body.url) {
        setUrl(new URL(body.url, window.location.origin).toString());
        setLog((l) => [...l, `LIVE AT ${body.url}`, "READY."]);
      } else {
        setLog((l) => [...l, `?${body.error ?? `HTTP ${res.status}`}`, "READY."]);
      }
    } catch (err) {
      setLog((l) => [...l, `?NETWORK ERROR — ${String(err)}`, "READY."]);
    } finally {
      setRunning(false);
    }
  }

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
        <button onClick={run} disabled={running}>
          {running ? "RUNNING…" : "RUN"}
        </button>
        {url && (
          <a href={url} target="_blank" rel="noreferrer">
            {url}
          </a>
        )}
      </div>

      <main>
        <section className="editor" ref={host} aria-label="BASIC editor" />
        <section className="log" aria-label="Program log" aria-live="polite">
          {log.map((line, i) => (
            <div key={i}>{line}</div>
          ))}
          <span className="cursor" aria-hidden="true">
            █
          </span>
        </section>
      </main>
    </div>
  );
}
