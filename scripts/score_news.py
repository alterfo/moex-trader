#!/usr/bin/env python3
"""Score sentiment with cometrain/moexT5 (T5 text2text) over a raw news archive.

Raw input format (cmd/newsfetch, cmd/newsimport output):
    {"ticker": "...", "article_id": "...", "title": "...", "source": "...",
     "trust_weight": 1.0, "published_ts": 1234567890}

Output format (finanalys news_history.jsonl, compatible with
model.LoadFinanalysNewsHistory):
    {"ticker": "...", "article_id": "...", "title": "...", "source": "...",
     "trust_weight": 1.0, "published_ts": 1234567890, "sentiment": 0.75,
     "confidence": 0.92, "reason": "moexT5"}

Usage:
    python3 scripts/score_news.py <raw.jsonl> <news_history.jsonl> [--batch 64]

The strongly adverse/positive tail is what the live veto-gate consumes; the
full numeric range feeds news_sentiment/news_count features. Neutral and
low-confidence rows are kept as-is (sentiment ~0) so they do not bias the
daily weighted aggregate.
"""
import argparse
import json
import sys
import numpy as np
import torch
from transformers import AutoModelForSeq2SeqLM, AutoTokenizer

LABEL_TO_SENTIMENT = {
    "positive": 1.0,
    "negative": -1.0,
    "neutral": 0.0,
}
LABELS = ["positive", "neutral", "negative"]


def build_inputs(tokenizer, titles, prompt):
    texts = [f"{prompt}{t}" for t in titles]
    return tokenizer(texts, return_tensors="pt", padding=True, truncation=True, max_length=256)


def decode_scores(scores, tokenizer, label_ids, label_sentiments):
    # scores: 1D tensor over vocabulary for the first generated token.
    probs = torch.softmax(scores, dim=-1)
    out_labels = []
    out_conf = []
    for idx in LABELS:
        out_labels.append((idx, None))
    label_probs = torch.zeros(len(LABELS))
    for i, lab in enumerate(LABELS):
        if lab in label_ids:
            label_probs[i] = probs[label_ids[lab]].item()
        else:
            label_probs[i] = 0.0
    total = label_probs.sum().item()
    if total <= 0:
        # No in-vocab label predicted; emit neutral with zero confidence.
        return "neutral", 0.0
    best = int(label_probs.argmax().item())
    return LABELS[best], label_probs[best].item() / total


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("input", help="raw JSONL archive")
    ap.add_argument("output", help="output finanalys-format news_history.jsonl")
    ap.add_argument("--batch", type=int, default=64)
    ap.add_argument("--prompt", default="")
    ap.add_argument("--device", default="auto")
    args = ap.parse_args()

    device = args.device
    if device == "auto":
        device = "mps" if torch.backends.mps.is_available() else "cuda" if torch.cuda.is_available() else "cpu"

    model = AutoModelForSeq2SeqLM.from_pretrained("cometrain/moexT5")
    model.to(device)
    model.eval()
    tokenizer = AutoTokenizer.from_pretrained("cometrain/moexT5")

    label_ids = {}
    for lab in LABELS:
        tok = tokenizer.convert_tokens_to_ids([lab])
        if len(tok) == 1 and tok[0] is not None and tok[0] != tokenizer.unk_token_id:
            label_ids[lab] = tok[0]

    records = []
    with open(args.input, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            records.append(json.loads(line))

    out = open(args.output, "w", encoding="utf-8")
    n = 0
    for i in range(0, len(records), args.batch):
        batch = records[i : i + args.batch]
        titles = [r.get("title") or "" for r in batch]
        inputs = build_inputs(tokenizer, titles, args.prompt).to(device)
        with torch.no_grad():
            outputs = model.generate(
                **inputs,
                max_new_tokens=6,
                num_beams=1,
                do_sample=False,
                output_scores=True,
                return_dict_in_generate=True,
            )
        for j, r in enumerate(batch):
            scores = outputs.scores[0][j]
            label, conf = decode_scores(scores, tokenizer, label_ids, LABEL_TO_SENTIMENT)
            rec = dict(r)
            rec["sentiment"] = LABEL_TO_SENTIMENT[label]
            rec["confidence"] = round(conf, 4)
            rec["reason"] = "moexT5"
            out.write(json.dumps(rec, ensure_ascii=False) + "\n")
            n += 1
        if (i // args.batch) % 50 == 0:
            print(f"scored {min(n, len(records))}/{len(records)}", file=sys.stderr, flush=True)
    out.close()
    print(f"done: {n} records -> {args.output}", file=sys.stderr)


if __name__ == "__main__":
    main()