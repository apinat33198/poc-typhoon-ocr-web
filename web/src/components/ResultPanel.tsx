import DOMPurify from "dompurify";
import { marked } from "marked";
import { useMemo, useState } from "react";
import { DocInfo, OcrResult } from "../api";

interface Props {
  doc: DocInfo | null;
  results: Record<number, OcrResult>;
  activePage: number | null;
  error: string;
  onSelectPage: (p: number) => void;
}

function renderMarkdown(md: string): string {
  // typhoon-specific tags -> styled elements, before sanitizing
  const pre = md.replace(
    /<page_number>([\s\S]*?)<\/page_number>/g,
    (_, n: string) => `<span class="page-no">หน้า ${n.trim()}</span>`,
  );
  const html = marked.parse(pre, { breaks: true, async: false }) as string;
  return DOMPurify.sanitize(html, { ADD_TAGS: ["figure"], ADD_ATTR: ["class"] });
}

export function ResultPanel({ doc, results, activePage, error, onSelectPage }: Props) {
  const [view, setView] = useState<"rendered" | "raw">("rendered");
  const [copied, setCopied] = useState(false);

  const pages = useMemo(
    () => Object.keys(results).map(Number).sort((a, b) => a - b),
    [results],
  );
  const result = activePage !== null ? results[activePage] : undefined;

  const renderedHtml = useMemo(
    () => (result && view === "rendered" ? renderMarkdown(result.markdown) : ""),
    [result, view],
  );

  async function copy() {
    if (!result) return;
    await navigator.clipboard.writeText(result.markdown);
    setCopied(true);
    setTimeout(() => setCopied(false), 1400);
  }

  function download() {
    if (!pages.length) return;
    const joined = pages.map((p) => results[p].markdown).join("\n\n---\n\n");
    const blob = new Blob([joined], { type: "text/markdown" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    const base = (doc?.filename ?? "document").replace(/\.[^.]+$/, "");
    a.download = `${base}.md`;
    a.click();
    URL.revokeObjectURL(a.href);
  }

  return (
    <section className="panel">
      <div className="result-head">
        <div className="result-head-left">
          <div className="panel-label no-margin">Result</div>
          <div className="page-tabs">
            {pages.map((p) => (
              <button
                key={p}
                className={`page-tab${p === activePage ? " active" : ""}`}
                onClick={() => onSelectPage(p)}
              >
                p.{p}
              </button>
            ))}
          </div>
        </div>
        <div className="result-tools">
          <div className="view-toggle">
            <button
              className={view === "rendered" ? "active" : ""}
              onClick={() => setView("rendered")}
            >
              rendered
            </button>
            <button
              className={view === "raw" ? "active" : ""}
              onClick={() => setView("raw")}
            >
              raw
            </button>
          </div>
          <button className="ghost" disabled={!result} onClick={copy}>
            {copied ? "Copied" : "Copy"}
          </button>
          <button className="ghost" disabled={!pages.length} onClick={download}>
            Download .md
          </button>
        </div>
      </div>

      {error && <div className="error-banner">{error}</div>}

      {!result ? (
        <div className="output empty">
          {doc
            ? "Pick a mode and press “Read this page”."
            : "Markdown extracted from the page appears here."}
        </div>
      ) : view === "raw" ? (
        <div className="output raw">{result.markdown}</div>
      ) : (
        <div className="output" dangerouslySetInnerHTML={{ __html: renderedHtml }} />
      )}

      {result && (
        <div className="elapsed">
          page {result.page} · {result.elapsed_seconds}s
        </div>
      )}
    </section>
  );
}
