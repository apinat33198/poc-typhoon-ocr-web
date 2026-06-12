export interface DocInfo {
  doc_id: string;
  filename: string;
  pages: number;
  is_pdf: boolean;
}

export interface OcrResult {
  page: number;
  task_type: TaskType;
  markdown: string;
  elapsed_seconds: number;
}

export interface ServerConfig {
  model: string;
}

export type TaskType = "v1.5" | "default" | "structure";
export type FigureLanguage = "Thai" | "English";

export interface BatchHandlers {
  onResult: (r: OcrResult) => void;
  onPageError: (page: number, detail: string) => void;
}

async function unwrap<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { detail?: string }).detail ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

export const api = {
  config: (): Promise<ServerConfig> =>
    fetch("/api/config").then((r) => unwrap<ServerConfig>(r)),

  upload: (file: File): Promise<DocInfo> => {
    const form = new FormData();
    form.append("file", file);
    return fetch("/api/documents", { method: "POST", body: form }).then((r) =>
      unwrap<DocInfo>(r),
    );
  },

  previewUrl: (docId: string, page: number, width?: number): string =>
    `/api/documents/${docId}/pages/${page}${width ? `?w=${width}` : ""}`,

  ocr: (
    docId: string,
    page: number,
    taskType: TaskType,
    figureLanguage: FigureLanguage,
    signal?: AbortSignal,
  ): Promise<OcrResult> =>
    fetch(`/api/documents/${docId}/ocr`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        page,
        task_type: taskType,
        figure_language: figureLanguage,
      }),
      signal,
    }).then((r) => unwrap<OcrResult>(r)),

  // Batch OCR over SSE: the server runs pages through a worker pool and
  // streams each result as it completes. Resolves when the stream ends.
  ocrAll: async (
    docId: string,
    pages: number[],
    taskType: TaskType,
    figureLanguage: FigureLanguage,
    signal: AbortSignal,
    handlers: BatchHandlers,
  ): Promise<void> => {
    const res = await fetch(`/api/documents/${docId}/ocr-all`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        pages,
        task_type: taskType,
        figure_language: figureLanguage,
      }),
      signal,
    });
    if (!res.ok || !res.body) {
      const body = await res.json().catch(() => ({}));
      throw new Error((body as { detail?: string }).detail ?? res.statusText);
    }

    const dispatch = (chunk: string) => {
      let event = "message";
      let data = "";
      for (const line of chunk.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (!data) return;
      if (event === "result") {
        handlers.onResult(JSON.parse(data) as OcrResult);
      } else if (event === "page_error") {
        const e = JSON.parse(data) as { page: number; detail: string };
        handlers.onPageError(e.page, e.detail);
      }
    };

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      let sep;
      while ((sep = buf.indexOf("\n\n")) >= 0) {
        dispatch(buf.slice(0, sep));
        buf = buf.slice(sep + 2);
      }
    }
  },
};
