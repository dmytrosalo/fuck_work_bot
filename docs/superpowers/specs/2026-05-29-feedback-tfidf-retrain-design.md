# Feedback-driven TF-IDF retrain

**Date:** 2026-05-29
**Status:** Approved (design)

## Goal

Improve the bot's work/not-work classification using the 165 manual feedback
labels collected in production (`/work` and `/notwork` replies), by retraining
the TF-IDF + LogReg model that runs in Go. "Improve detection" means better
accuracy on **new, unseen** messages — not just memorizing past corrections.

## Context

- **Production classifier** (`go-bot/internal/classifier/classifier.go`): TF-IDF +
  Logistic Regression, loaded from `go-bot/model/tfidf_model.json`. Config:
  `ngram_range=[1,3]`, `sublinear_tf=true`, `norm="l2"`, `max_features=15000`,
  vocab size 15000. Plus an additive Go-side `keywordBoost` heuristic.
- The original script that built `tfidf_model.json` is **missing** from the repo
  (so is `work_classifier.py`, the "old" model the existing scripts bootstrap
  from). The embedding scripts (`train_head.py`, `finetune_model.py`,
  `export_model.py`) produce a *different* artifact (`weights.json` + 118 MB
  ONNX) that does not run in production — the 256 MB Fly box can't load it.
- **Feedback data** lives in the production SQLite `feedback(text, label)` table:
  165 rows = 17 `work` + 148 `personal`. Collected via `/work` / `/notwork`
  replies that pay 10 🪙, so labels are gamified and may contain noise. The table
  does **not** record the model's prediction, so "is this a correction?" must be
  computed by running the classifier.
- Available training inputs (all gitignored, local only): `labeling_batch.csv`
  (~250 gold labels), `chat/result.json` + `chat/result_2.json` (~24 MB chat
  export), and the exported feedback rows. sklearn is installed in `go-bot/venv`.

## Approach

Reconstruct a self-contained, reproducible TF-IDF trainer in Python that folds
the feedback into the training set and re-emits `tfidf_model.json` in the **exact
schema the Go code already reads**. No Go changes.

### Data assembly

- **Gold (sample_weight ≈ 3.0):**
  - `labeling_batch.csv` — rows where `your_label ∈ {w, p}` → 1=work / 0=personal.
  - `feedback.csv` — exported from the live `bot.db` (`text, label`). Cleaning:
    trim; drop `len < 3`; drop emoji-/punctuation-only; case-insensitive exact
    dedup; resolve conflicting labels on identical text by majority vote, drop
    ties. Dedup against `labeling_batch.csv` (gold-vs-gold conflict → drop).
- **Weak / pseudo-labels (sample_weight ≈ 1.0):**
  - Extract text messages from `chat/result*.json` (handle string/list/dict text
    forms; skip the bot's own messages `from == "FuckingWorkTracking"`; dedup;
    drop short). Exclude anything already in gold.
  - Label each with the **current** model (Python reimplementation of the Go
    scoring, including `keywordBoost`, loaded from the current
    `tfidf_model.json`). Keep only predictions with confidence ≥ 0.95.
  - Purpose: rebuild the rich 15k vocabulary. High gold weight keeps the human
    corrections dominant; the high confidence threshold limits echo of the old
    model's mistakes.

### Model — match existing config exactly

- `TfidfVectorizer(ngram_range=(1,3), sublinear_tf=True, norm="l2",
  max_features=15000)` with a **custom tokenizer that mirrors `classifier.go`
  `tokenize()` exactly**: lowercase, split on any non-letter/non-digit (unicode
  aware), keep tokens of length ≥ 1. Tokenizer parity is critical — sklearn's
  default `token_pattern` drops 1-char tokens and would desync the trained vocab
  from Go inference, silently degrading accuracy.
- `LogisticRegression(C=1.0, class_weight="balanced", max_iter=1000,
  random_state=42)`, labels 1=work / 0=personal so a positive logit = work,
  matching the Go decision rule. Fit with per-sample weights (gold > weak).

### Export — `tfidf_model.json`

Emit the same keys the Go struct reads:
- `vocabulary`: `vectorizer.vocabulary_` (term → int index)
- `idf`: array aligned by feature index (`vectorizer.idf_`)
- `config`: `{max_features, ngram_range:[1,3], sublinear_tf:true, norm:"l2"}`
- `classifier`: `{coef: lr.coef_[0], intercept: lr.intercept_[0]}`

### Evaluation gate (decide ship / no-ship from real numbers)

- Stratified 80/20 split on **gold only** + k-fold CV. Training set includes
  pseudo-labels; evaluation is on held-out **gold** only (pseudo-labels are not
  trustworthy ground truth).
- Print per-class precision / recall / F1 for **new vs current** model on the
  same held-out gold, plus the `tricky` sanity examples (reuse the list from
  `finetune_model.py`).
- **Ship rule:** new model must beat current overall **without dropping work
  recall** (the minority class the bot actually reacts to). Numbers are presented
  to the user; the user approves the deploy. No auto-ship.

### Deploy

- Regenerate `go-bot/model/tfidf_model.json`; commit it together with the new
  `train_tfidf.py`. Push to `main` → Fly auto-deploys via GitHub Actions.
- `keywordBoost` in Go is left untouched (out of scope).

## Privacy / gitignore

`labeling_batch.csv` and `chat/` are **already gitignored** — training text is
intentionally kept out of the repo. Therefore:
- `feedback.csv` (exported chat text) is **gitignored**, not committed.
- Only the trained artifact (`tfidf_model.json`) and `train_tfidf.py` are
  committed. The script reads local-only data files so retraining is
  reproducible by anyone who has the data, without leaking it into git.

## Deliverables

- `go-bot/scripts/train_tfidf.py` — reproducible trainer + exporter (new).
- A feedback-export step (script or documented `flyctl` command) producing
  `feedback.csv` from the live `bot.db`.
- `.gitignore` entry for `feedback.csv`.
- Regenerated `go-bot/model/tfidf_model.json` (the shipped artifact).

## Out of scope (YAGNI)

- Live override / exact-match lookup layer.
- Automated self-improving retrain loop.
- Changes to `keywordBoost`, the decision threshold, or the Go classifier code.
- The embedding/ONNX pipeline.

## Risks

- **Label noise** from gamified feedback → mitigated by cleaning + majority vote.
- **Class imbalance** (148 personal vs 17 work) → `class_weight="balanced"` +
  evaluation gated on work recall.
- **Pseudo-label echo** of the current model → high gold weight + ≥0.95 threshold.
- **Tokenizer drift** between Python training and Go inference → custom tokenizer
  with exact parity (explicitly tested against sample strings).
- **Tiny gold set** (~415) → pseudo-labels provide vocabulary breadth.
