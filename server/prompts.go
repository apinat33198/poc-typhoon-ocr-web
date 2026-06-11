package main

import "fmt"

// Prompt templates ported from scb-10x/typhoon-ocr (packages/typhoon_ocr/ocr_utils.py,
// PROMPTS_SYS), Apache 2.0. Kept byte-for-byte so model behavior matches upstream.

func promptDefault(baseText string) string {
	return "Below is an image of a document page along with its dimensions. " +
		"Simply return the markdown representation of this document, presenting tables in markdown format as they naturally appear.\n" +
		"If the document contains images, use a placeholder like dummy.png for each image.\n" +
		"Your final output must be in JSON format with a single key `natural_text` containing the response.\n" +
		"RAW_TEXT_START\n" + baseText + "\nRAW_TEXT_END"
}

func promptStructure(baseText string) string {
	return "Below is an image of a document page, along with its dimensions and possibly some raw textual content previously extracted from it. " +
		"Note that the text extraction may be incomplete or partially missing. Carefully consider both the layout and any available text to reconstruct the document accurately.\n" +
		"Your task is to return the markdown representation of this document, presenting tables in HTML format as they naturally appear.\n" +
		"If the document contains images or figures, analyze them and include the tag <figure>IMAGE_ANALYSIS</figure> in the appropriate location.\n" +
		"Your final output must be in JSON format with a single key `natural_text` containing the response.\n" +
		"RAW_TEXT_START\n" + baseText + "\nRAW_TEXT_END"
}

func promptV15(figureLanguage string) string {
	return fmt.Sprintf(`Extract all text from the image.


Instructions:
- Only return the clean Markdown.
- Do not include any explanation or extra text.
- You must include all information on the page.


Formatting Rules:
- Tables: Render tables using <table>...</table> in clean HTML format.
- Equations: Render equations using LaTeX syntax with inline ($...$) and block ($$...$$).
- Images/Charts/Diagrams: Wrap any clearly defined visual areas (e.g. charts, diagrams, pictures) in:


<figure>
Describe the image's main elements (people, objects, text), note any contextual clues (place, event, culture), mention visible text and its meaning, provide deeper analysis when relevant (especially for financial charts, graphs, or documents), comment on style or architecture if relevant, then give a concise overall summary. Describe in %s.
</figure>


- Page Numbers: Wrap page numbers in <page_number>...</page_number> (e.g., <page_number>14</page_number>).
- Checkboxes: Use ☐ for unchecked and ☑ for checked boxes.
    `, figureLanguage)
}
