import { useEffect, useState } from "react";
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

  async function readPage(p: number): Promise<OcrResult> {
    const r = await api.ocr(doc!.doc_id, p, mode, figLang);
    setResults((prev) => ({ ...prev, [p]: r }));
    setActivePage(p);
    return r;
  }

  async function handleRun() {
    if (!doc || busy) return;
    setError("");
    setBusy(true);
    setProgress(`reading page ${page}…`);
    try {
      await readPage(page);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
      setProgress("");
    }
  }

  async function handleRunAll() {
    if (!doc || busy) return;
    setError("");
    setBusy(true);
    let current = 1;
    try {
      for (let p = 1; p <= doc.pages; p++) {
        current = p;
        setPage(p);
        setProgress(`reading page ${p} / ${doc.pages}…`);
        await readPage(p);
      }
      setProgress(`done — ${doc.pages} pages`);
      setTimeout(() => setProgress(""), 2500);
    } catch (err) {
      setError(`Stopped at page ${current}: ${(err as Error).message}`);
      setProgress("");
    } finally {
      setBusy(false);
    }
  }

  function handlePageChange(p: number) {
    setPage(p);
    if (results[p]) setActivePage(p);
  }

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
            <button className="primary" disabled={!doc || busy} onClick={handleRun}>
              Read this page
            </button>
            <button
              className="secondary"
              disabled={!doc || busy || (doc?.pages ?? 1) === 1}
              onClick={handleRunAll}
            >
              Read all pages
            </button>
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
