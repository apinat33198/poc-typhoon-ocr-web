# Typhoon OCR Web

A self-hostable web UI for [scb-10x/typhoon-ocr](https://github.com/scb-10x/typhoon-ocr) — drop in a Thai (or English) PDF or image and get clean markdown back.

**Go backend** (stdlib only, single binary) + **TypeScript frontend** (Vite + React). Replaces the upstream Gradio demo and adds multi-page OCR.

## Features

- Drag-and-drop PDF / PNG / JPG upload with per-page preview
- All three prompt modes: `v1.5` (clean markdown, LaTeX, figure descriptions in Thai or English), `default` (markdown tables), `structure` (HTML tables for complex layouts)
- OCR a single page or the whole document — pages run in parallel through a server-side worker pool (`OCR_CONCURRENCY`, default 4) with results streamed in as they complete; cancel mid-run, resume from the first unread page, and failed pages are retried on resume instead of aborting the run
- Rendered / raw / compare views — compare shows the original page side by side with the OCR result (click the image for full size) — plus per-page copy and download all pages as one `.md`
- Works against any OpenAI-compatible endpoint: local vLLM or api.opentyphoon.ai
- Same prompts and sampling parameters as upstream (temp 0.1, top_p 0.6, repetition_penalty 1.1–1.2)

## Requirements

- Go 1.22+, Node 18+
- poppler: `apt-get install poppler-utils` (Linux) / `brew install poppler` (macOS)

## Build & run

```bash
# frontend
cd web
npm install
npm run build

# backend
cd ../server
go build -o typhoon-ocr-web .
cd ..
cp .env.example .env         # then edit
./server/typhoon-ocr-web     # → http://localhost:7870
```

The server finds the frontend build automatically (checked relative to the working directory and the binary), so it can be launched from the repo root or anywhere else; set `WEB_DIR` to override. If you see a `frontend build not found` warning at startup, run the `npm run build` step above.

For frontend development with hot reload, run the Go server and `npm run dev` in `web/` — Vite proxies `/api` to `:7870`.

## Pointing at a model

**Local vLLM** (e.g. on a DGX Spark) — the upstream serve command:

```bash
vllm serve scb10x/typhoon-ocr-7b --served-model-name typhoon-ocr --dtype bfloat16 --port 8101
```

with `.env`:

```
TYPHOON_BASE_URL=http://localhost:8101/v1
TYPHOON_API_KEY=no-key
TYPHOON_OCR_MODEL=typhoon-ocr
```

For the newer v1.5 weights use `scb10x/typhoon-ocr1.5-*` from Hugging Face and keep the served model name. If vLLM runs on another machine, change the host in `TYPHOON_BASE_URL`.

**Hosted API** — set `TYPHOON_BASE_URL=https://api.opentyphoon.ai/v1`, your API key, and `TYPHOON_OCR_MODEL=typhoon-ocr-preview`. The preview model only supports `default` and `structure`; the server enforces this.

## Architecture

```
web/      Vite + React + TS  ──build──▶  web/dist (served by the Go binary)
server/   Go stdlib HTTP server
            ├─ pdfinfo / pdftoppm / pdftotext (poppler) for page count,
            │  rendering, and anchor-text extraction
            └─ POST {base_url}/chat/completions with the typhoon prompts
```

Notes:

- The `default`/`structure` prompts include "anchor text" (positioned text from the PDF layer). Upstream extracts it with pypdf; this port approximates it with `pdftotext -bbox`. The recommended `v1.5` mode doesn't use anchor text at all.
- Uploads live in temp files and are deleted after an hour.
- Prompts are ported byte-for-byte from the upstream package (Apache 2.0).

## License

Apache 2.0, same as upstream.
