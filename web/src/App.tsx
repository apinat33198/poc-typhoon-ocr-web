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
    let done = 0;
    const failed: number[] = [];
    const tick = () =>
      setProgress(`reading ${todo.length} pages — ${done}/${todo.length} done…`);
    tick();
    try {
      await api.ocrAll(doc.doc_id, todo, mode, figLang, ctl.signal, {
        onResult: (r) => {
          done++;
          setResults((prev) => ({ ...prev, [r.page]: r }));
          setPage(r.page);
          setActivePage(r.page);
          tick();
        },
        onPageError: (p) => {
          done++;
          failed.push(p);
          tick();
        },
      });
      if (failed.length > 0) {
        failed.sort((a, b) => a - b);
        setError(
          `${failed.length} page(s) failed: ${failed.join(", ")}. Press Resume to retry them.`,
        );
        setProgress("");
      } else {
        flashProgress(`done — ${todo.length} pages`);
      }
    } catch (err) {
      if (ctl.signal.aborted) {
        flashProgress(`cancelled — ${done}/${todo.length} pages read`);
      } else {
        setError((err as Error).message);
        setProgress("");
      }
    } finally {
      abortRef.current = null;
      setBusy(false);
    }
  }

  // Preview and result stay on the same page: stepping the preview shows that
  // page's result (or the empty hint), picking a result tab moves the preview.
  function handlePageChange(p: number) {
    setPage(p);
    setActivePage(results[p] ? p : null);
  }

  function handleSelectPage(p: number) {
    setActivePage(p);
    setPage(p);
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
            {config ? config.model : "server unreachable"}
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
          onSelectPage={handleSelectPage}
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
