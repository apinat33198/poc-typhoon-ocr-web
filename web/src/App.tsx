import { useEffect, useRef, useState } from "react";
import {
  api,
  DocInfo,
  FigureLanguage,
  OcrResult,
  ServerConfig,
  TaskType,
} from "./api";
import { Scanner } from "./components/Scanner";
import { ModePicker } from "./components/ModePicker";
import { ResultPanel } from "./components/ResultPanel";

export default function App() {
  const [config, setConfig] = useState<ServerConfig | null>(null);
  const [doc, setDoc] = useState<DocInfo | null>(null);
  const [page, setPage] = useState(1);
  const [mode, setMode] = useState<TaskType>("v1.5");
  const [figLang, setFigLang] = useState<FigureLanguage>("Thai");
  const [results, setResults] = useState<Record<number, OcrResult>>({});
  const [activePage, setActivePage] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState("");
  const [error, setError] = useState("");
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    api.config().then(setConfig).catch(() => setConfig(null));
  }, []);

  async function handleUpload(file: File) {
    setError("");
    try {
      const info = await api.upload(file);
      setDoc(info);
      setPage(1);
      setResults({});
      setActivePage(null);
    } catch (err) {
      setError(`Upload failed: ${(err as Error).message}`);
    }
  }

  function handleClear() {
    if (busy) return;
    setDoc(null);
    setResults({});
    setActivePage(null);
    setError("");
  }

  async function readPage(p: number, signal: AbortSignal): Promise<OcrResult> {
    const r = await api.ocr(doc!.doc_id, p, mode, figLang, signal);
    setResults((prev) => ({ ...prev, [p]: r }));
    setActivePage(p);
    return r;
  }

  function handleCancel() {
    abortRef.current?.abort();
  }

  function flashProgress(msg: string) {
    setProgress(msg);
    setTimeout(() => setProgress(""), 2500);
  }

  async function handleRun() {
    if (!doc || busy) return;
    setError("");
    setBusy(true);
    const ctl = new AbortController();
    abortRef.current = ctl;
    setProgress(`reading page ${page}…`);
    try {
      await readPage(page, ctl.signal);
      setProgress("");
    } catch (err) {
      if (ctl.signal.aborted) {
        flashProgress("cancelled");
      } else {
        setError((err as Error).message);
        setProgress("");
      }
    } finally {
      abortRef.current = null;
      setBusy(false);
    }
  }

  async function handleRunAll() {
    if (!doc || busy) return;
    setError("");
    setBusy(true);
    const ctl = new AbortController();
    abortRef.current = ctl;
    // Resume: only read pages without a result yet; if every page is done,
    // re-read the whole document (e.g. after switching modes).
    const all = Array.from({ length: doc.pages }, (_, i) => i + 1);
    let todo = all.filter((p) => !results[p]);
    if (todo.length === 0) todo = all;
    let current = todo[0];
    try {
      for (const p of todo) {
        current = p;
        setPage(p);
        setProgress(`reading page ${p} / ${doc.pages}…`);
        await readPage(p, ctl.signal);
      }
      flashProgress(`done — ${doc.pages} pages`);
    } catch (err) {
      if (ctl.signal.aborted) {
        flashProgress(`cancelled at page ${current}`);
      } else {
        setError(`Stopped at page ${current}: ${(err as Error).message}`);
        setProgress("");
      }
    } finally {
      abortRef.current = null;
      setBusy(false);
    }
  }

  function handlePageChange(p: number) {
    setPage(p);
    if (results[p]) setActivePage(p);
  }

  const remaining = doc
    ? Array.from({ length: doc.pages }, (_, i) => i + 1).filter((p) => !results[p])
        .length
    : 0;

  return (
    <>
      <header>
        <div className="wordmark">
          <h1>
            TYPHOON<span className="accent">·</span>OCR
          </h1>
          <span className="thai">แปลงเอกสารเป็นมาร์กดาวน์</span>
        </div>
        <div className={`endpoint${config ? " up" : ""}`}>
          <span className="dot" />
          <span>
            {config ? `${config.model} @ ${config.base_url}` : "server unreachable"}
          </span>
        </div>
      </header>

      <main>
        <section className="panel">
          <div className="panel-label">Document</div>
          <Scanner
            doc={doc}
            page={page}
            busy={busy}
            onUpload={handleUpload}
            onClear={handleClear}
            onPageChange={handlePageChange}
          />

          <div className="panel-label gap-top">Mode</div>
          <ModePicker
            mode={mode}
            figLang={figLang}
            onMode={setMode}
            onFigLang={setFigLang}
          />

          <div className="actions">
            {busy ? (
              <button className="secondary" onClick={handleCancel}>
                Cancel
              </button>
            ) : (
              <>
                <button className="primary" disabled={!doc} onClick={handleRun}>
                  Read this page
                </button>
                <button
                  className="secondary"
                  disabled={!doc || (doc?.pages ?? 1) === 1}
                  onClick={handleRunAll}
                >
                  {remaining > 0 && remaining < (doc?.pages ?? 0)
                    ? `Resume (${remaining} left)`
                    : "Read all pages"}
                </button>
              </>
            )}
          </div>
          {progress && <div className="progress">{progress}</div>}
        </section>

        <ResultPanel
          doc={doc}
          results={results}
          activePage={activePage}
          error={error}
          onSelectPage={setActivePage}
        />
      </main>

      <footer>
        Built on{" "}
        <a
          href="https://github.com/scb-10x/typhoon-ocr"
          target="_blank"
          rel="noopener noreferrer"
        >
          scb-10x/typhoon-ocr
        </a>{" "}
        · Go backend · runs against any OpenAI-compatible endpoint
      </footer>
    </>
  );
}
