import { FigureLanguage, TaskType } from "../api";

const MODES: { value: TaskType; name: string; desc: string }[] = [
  {
    value: "v1.5",
    name: "v1.5",
    desc: "Clean markdown with HTML tables, LaTeX equations, and figure descriptions. Needs the typhoon-ocr 1.5 weights.",
  },
  {
    value: "default",
    name: "default",
    desc: "Markdown tables. Good for free-form pages like infographics. Works with the hosted preview API.",
  },
  {
    value: "structure",
    name: "structure",
    desc: "HTML tables and figure tags. Better for complex layouts, forms, and dense tables.",
  },
];

interface Props {
  mode: TaskType;
  figLang: FigureLanguage;
  onMode: (m: TaskType) => void;
  onFigLang: (l: FigureLanguage) => void;
}

export function ModePicker({ mode, figLang, onMode, onFigLang }: Props) {
  return (
    <>
      <div className="modes">
        {MODES.map((m) => (
          <label key={m.value} className={`mode${mode === m.value ? " selected" : ""}`}>
            <input
              type="radio"
              name="mode"
              value={m.value}
              checked={mode === m.value}
              onChange={() => onMode(m.value)}
            />
            <span className="name">{m.name}</span>
            <span className="desc">{m.desc}</span>
          </label>
        ))}
      </div>
      {mode === "v1.5" && (
        <div className="fig-lang">
          <label htmlFor="figLangSel">Describe figures in</label>
          <select
            id="figLangSel"
            value={figLang}
            onChange={(e) => onFigLang(e.target.value as FigureLanguage)}
          >
            <option>Thai</option>
            <option>English</option>
          </select>
        </div>
      )}
    </>
  );
}
