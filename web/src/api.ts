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
  base_url: string;
}

export type TaskType = "v1.5" | "default" | "structure";
export type FigureLanguage = "Thai" | "English";

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

  previewUrl: (docId: string, page: number): string =>
    `/api/documents/${docId}/pages/${page}`,

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
};
