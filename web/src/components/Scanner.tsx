import { DragEvent, KeyboardEvent, useRef, useState } from "react";
import { api, DocInfo } from "../api";

interface Props {
  doc: DocInfo | null;
  page: number;
  busy: boolean;
  onUpload: (file: File) => void;
  onClear: () => void;
  onPageChange: (page: number) => void;
}

export function Scanner({ doc, page, busy, onUpload, onClear, onPageChange }: Props) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [dragover, setDragover] = useState(false);
  const [uploading, setUploading] = useState(false);

  async function pick(file: File | undefined) {
    if (!file) return;
    setUploading(true);
    try {
      await onUpload(file);
    } finally {
      setUploading(false);
    }
  }

  function handleDrop(e: DragEvent) {
    e.preventDefault();
    setDragover(false);
    pick(e.dataTransfer.files?.[0]);
  }

  function handleKey(e: KeyboardEvent) {
    if ((e.key === "Enter" || e.key === " ") && !doc) {
      e.preventDefault();
      fileInput.current?.click();
    }
  }

  const cls = [
    "scanner",
    dragover ? "dragover" : "",
    doc ? "has-doc" : "",
    busy ? "scanning" : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <>
      <div
        className={cls}
        role="button"
        tabIndex={0}
        aria-label="Upload a PDF or image"
        onClick={() => !doc && fileInput.current?.click()}
        onKeyDown={handleKey}
        onDragOver={(e) => {
          e.preventDefault();
          setDragover(true);
        }}
        onDragLeave={() => setDragover(false)}
        onDrop={handleDrop}
      >
        {!doc && (
          <div className="drop-hint">
            <strong>{uploading ? "Uploading…" : "Drop a document here"}</strong>
            or click to choose a file
            <div className="types">PDF · PNG · JPG</div>
          </div>
        )}
        {doc && (
          <div className="sheet">
            <img src={api.previewUrl(doc.doc_id, page)} alt={`Page ${page} preview`} />
            <div className="beam" />
          </div>
        )}
      </div>
      <input
        ref={fileInput}
        type="file"
        accept=".pdf,.png,.jpg,.jpeg"
        hidden
        onChange={(e) => {
          pick(e.target.files?.[0]);
          e.target.value = "";
        }}
      />

      {doc && (
        <div className="doc-meta">
          <span className="name">
            {doc.filename} · {doc.pages} page{doc.pages > 1 ? "s" : ""}
          </span>
          <button className="ghost" onClick={onClear} disabled={busy}>
            Remove
          </button>
        </div>
      )}

      {doc && doc.pages > 1 && (
        <div className="stepper">
          <button
            aria-label="Previous page"
            disabled={page <= 1 || busy}
            onClick={() => onPageChange(page - 1)}
          >
            ‹
          </button>
          <span className="pos">
            page <b>{page}</b> / {doc.pages}
          </span>
          <button
            aria-label="Next page"
            disabled={page >= doc.pages || busy}
            onClick={() => onPageChange(page + 1)}
          >
            ›
          </button>
        </div>
      )}
    </>
  );
}
