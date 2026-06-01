"""
Reproducible TF-IDF + LogisticRegression trainer for the work/not-work classifier.

Rebuilds go-bot/model/tfidf_model.json in the exact schema the Go classifier
(internal/classifier/classifier.go) reads. Folds production feedback labels into
the training set so detection improves on the patterns people corrected.

Data sources (all local / gitignored):
  - labeling_batch.csv            ~250 gold labels (your_label = w/p)
  - go-bot/scripts/data/feedback.csv  165 gold labels from production (/work, /notwork)
  - chat/result.json, result_2.json   chat corpus, pseudo-labeled by the CURRENT model

Gold labels get higher sample weight than pseudo-labels so the human corrections
dominate while the pseudo-labels supply vocabulary breadth.

Usage:
    go-bot/venv/bin/python go-bot/scripts/train_tfidf.py [out_path]

Default out_path is go-bot/model/tfidf_model.candidate.json (does NOT overwrite
the live model — promote it only after reviewing the eval numbers).
"""

import csv
import json
import math
import os
import sys
import unicodedata
from collections import Counter

import numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report, confusion_matrix
from sklearn.model_selection import train_test_split

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
GO_BOT_DIR = os.path.dirname(SCRIPT_DIR)
PROJECT_ROOT = os.path.dirname(GO_BOT_DIR)
MODEL_DIR = os.path.join(GO_BOT_DIR, "model")
CURRENT_MODEL_PATH = os.path.join(MODEL_DIR, "tfidf_model.json")

# Must match internal/classifier/classifier.go exactly.
NGRAM_MIN, NGRAM_MAX = 1, 3
MAX_FEATURES = 15000
SUBLINEAR_TF = True
NORM = "l2"

GOLD_WEIGHT = 3.0
GOLD_WORK_MULT = 3.0   # extra weight on gold-work samples (chosen by 5-fold CV)
WEAK_WEIGHT = 1.0
PSEUDO_CONF_THRESHOLD = 0.95

# Mirror of workKeywords in classifier.go (used only by keywordBoost).
WORK_KEYWORDS = [
    "маріт", "marit", "ілір", "ilir", "делна", "delna", "насір", "nassir",
    "руді", "rudi", "аршан", "даглас", "сільвейн", "silvain", "тамара", "конг",
    "валер", "алек", "нуно", "азам",
    "keyo", "кейо", "nrf", "нрф", "biopay", "біопей", "tenderize", "тендерайз",
    "hexaon", "масарі", "masari",
    "деплой", "deploy", "мердж", "merge", "тікет", "ticket", "джира", "jira",
    "лінеар", "linear", "спринт", "sprint", "реліз", "release", "стендап", "standup",
    "дейлі", "daily", "рев'ю", "review", "пайплайн", "ci/cd", "sdk", "api",
    "ендпоінт", "endpoint", "біометр",
    "мітинг", "meeting", "созвон", "дедлайн", "deadline", "естімейт",
    "зарплат", "salary", "відпустк", "контракт",
]


# ── Tokenizer: exact parity with classifier.go tokenize() ────────────────────
# Go: lowercase, then split on any rune that is not unicode.IsLetter or
# unicode.IsDigit; keep runs of length >= 1.
def tokenize(text):
    lower = text.lower()
    tokens = []
    cur = []
    for ch in lower:
        cat = unicodedata.category(ch)
        is_letter = cat[0] == "L"          # unicode.IsLetter -> category L*
        is_digit = cat == "Nd"             # unicode.IsDigit  -> category Nd
        if is_letter or is_digit:
            cur.append(ch)
        elif cur:
            tokens.append("".join(cur))
            cur = []
    if cur:
        tokens.append("".join(cur))
    return tokens


def keyword_boost(text):
    lower = text.lower()
    hits = sum(1 for kw in WORK_KEYWORDS if kw in lower)
    boost = hits * 0.3
    return min(boost, 1.5)


# ── Current model: Python reimplementation of classifier.go Classify() ───────
class CurrentModel:
    def __init__(self, path):
        with open(path, encoding="utf-8") as f:
            m = json.load(f)
        self.vocab = m["vocabulary"]
        self.idf = m["idf"]
        self.coef = m["classifier"]["coef"]
        self.intercept = m["classifier"]["intercept"]
        cfg = m["config"]
        self.nmin, self.nmax = cfg["ngram_range"]
        self.sublinear = cfg["sublinear_tf"]
        self.norm = cfg["norm"]

    def _ngrams(self, tokens):
        out = []
        for n in range(self.nmin, self.nmax + 1):
            for i in range(len(tokens) - n + 1):
                out.append(" ".join(tokens[i:i + n]))
        return out

    def work_prob(self, text):
        if not text:
            return 0.0
        ngrams = self._ngrams(tokenize(text))
        tf = {}
        for ng in ngrams:
            idx = self.vocab.get(ng)
            if idx is not None:
                tf[idx] = tf.get(idx, 0.0) + 1.0
        if self.sublinear:
            for idx in tf:
                tf[idx] = 1.0 + math.log(tf[idx])
        tfidf = {idx: v * self.idf[idx] for idx, v in tf.items()}
        if self.norm == "l2":
            nrm = math.sqrt(sum(v * v for v in tfidf.values()))
            if nrm > 0:
                for idx in tfidf:
                    tfidf[idx] /= nrm
        logit = self.intercept + sum(self.coef[idx] * v for idx, v in tfidf.items())
        logit += keyword_boost(text)
        return 1.0 / (1.0 + math.exp(-logit))

    def predict(self, text):
        return 1 if self.work_prob(text) >= 0.5 else 0


# ── Data loading ─────────────────────────────────────────────────────────────
def _clean(text):
    return (text or "").strip()


def _is_noise(text):
    # Drop coin-farm noise: too short or no letters/digits at all.
    if len(text) < 3:
        return True
    return not any(c[0] == "L" or c == "Nd" for c in map(unicodedata.category, text))


def load_labeling_batch():
    path = os.path.join(PROJECT_ROOT, "labeling_batch.csv")
    pairs = []
    with open(path, encoding="utf-8") as f:
        for row in csv.DictReader(f, delimiter=";"):
            text = _clean(row.get("text", ""))
            label = (row.get("your_label", "") or "").strip().lower()
            if text and label in ("w", "p"):
                pairs.append((text, 1 if label == "w" else 0))
    return pairs


def load_feedback():
    path = os.path.join(SCRIPT_DIR, "data", "feedback.csv")
    pairs = []
    with open(path, encoding="utf-8") as f:
        for row in csv.DictReader(f):
            text = _clean(row.get("text", ""))
            label = (row.get("label", "") or "").strip().lower()
            if text and label in ("work", "personal"):
                pairs.append((text, 1 if label == "work" else 0))
    return pairs


def dedup_majority(pairs):
    """Collapse identical text (case-insensitive) to one majority label; drop ties and noise."""
    by_key = {}
    for text, label in pairs:
        if _is_noise(text):
            continue
        key = text.lower()
        by_key.setdefault(key, {"text": text, "votes": Counter()})
        by_key[key]["votes"][label] += 1
    out = {}
    for key, entry in by_key.items():
        votes = entry["votes"]
        top = votes.most_common()
        if len(top) > 1 and top[0][1] == top[1][1]:
            continue  # tie -> drop
        out[key] = (entry["text"], top[0][0])
    return out  # key -> (text, label)


def extract_text(msg):
    text = msg.get("text", "")
    if isinstance(text, str):
        return text
    if isinstance(text, list):
        parts = []
        for part in text:
            if isinstance(part, str):
                parts.append(part)
            elif isinstance(part, dict):
                parts.append(part.get("text", ""))
        return "".join(parts)
    return ""


def load_chat_messages():
    seen = set()
    msgs = []
    for fname in ("chat/result.json", "chat/result_2.json"):
        path = os.path.join(PROJECT_ROOT, fname)
        if not os.path.exists(path):
            continue
        with open(path, encoding="utf-8") as f:
            data = json.load(f)
        for m in data.get("messages", []):
            if m.get("type") != "message":
                continue
            if m.get("from") == "FuckingWorkTracking":
                continue
            text = _clean(extract_text(m))
            if len(text) < 5:
                continue
            key = text.lower()
            if key in seen:
                continue
            seen.add(key)
            msgs.append(text)
    return msgs


# ── Training ─────────────────────────────────────────────────────────────────
def make_vectorizer():
    return TfidfVectorizer(
        tokenizer=tokenize,
        token_pattern=None,
        lowercase=False,            # tokenizer lowercases (matches Go order)
        preprocessor=None,
        ngram_range=(NGRAM_MIN, NGRAM_MAX),
        sublinear_tf=SUBLINEAR_TF,
        norm=NORM,
        max_features=MAX_FEATURES,
    )


def fit_model(texts, labels, weights):
    vec = make_vectorizer()
    X = vec.fit_transform(texts)
    lr = LogisticRegression(C=1.0, max_iter=1000, class_weight="balanced", random_state=42)
    lr.fit(X, labels, sample_weight=weights)
    return vec, lr


def predict_labels(vec, lr, texts):
    """Match Go inference: TF-IDF logit + keyword_boost, then sigmoid threshold."""
    X = vec.transform(texts)
    logits = X.dot(lr.coef_[0]) + lr.intercept_[0]
    logits = np.asarray(logits).ravel()
    logits = logits + np.array([keyword_boost(t) for t in texts])
    return (logits >= 0.0).astype(int)


TRICKY = [
    ("руді в ахуі", "work"),
    ("делна підараска?", "work"),
    ("я тут просив Делну по роботі", "work"),
    ("ну і як тобі аршан ілір", "work"),
    ("маріт звільняється?", "work"),
    ("нрф планінг щотижневий", "work"),
    ("деплой впав, треба фіксити", "work"),
    ("потім душ", "personal"),
    ("смачного!", "personal"),
    ("Завтра останній день відпустки", "personal"),
    ("багато вибачень)", "personal"),
    ("А багато по іпотеці ще?", "personal"),
]


def report(name, y_true, y_pred):
    print(f"\n=== {name} ===")
    print(classification_report(y_true, y_pred, target_names=["personal", "work"], digits=3, zero_division=0))
    cm = confusion_matrix(y_true, y_pred, labels=[0, 1])
    print("confusion [rows=true personal/work, cols=pred personal/work]:")
    print(cm)


def main():
    out_path = sys.argv[1] if len(sys.argv) > 1 else os.path.join(MODEL_DIR, "tfidf_model.candidate.json")

    print("Loading gold labels...")
    batch = load_labeling_batch()
    feedback = load_feedback()
    print(f"  labeling_batch.csv: {len(batch)}  |  feedback.csv: {len(feedback)}")

    gold = dedup_majority(batch + feedback)
    gold_texts = [t for t, _ in gold.values()]
    gold_labels = [l for _, l in gold.values()]
    print(f"  gold after clean/dedup: {len(gold_texts)} "
          f"(work={sum(gold_labels)}, personal={len(gold_labels) - sum(gold_labels)})")
    gold_keys = set(gold.keys())

    print("\nPseudo-labeling chat corpus with current model...")
    current = CurrentModel(CURRENT_MODEL_PATH)
    chat = [t for t in load_chat_messages() if t.lower() not in gold_keys]
    print(f"  chat candidates: {len(chat)}")
    weak_texts, weak_labels = [], []
    for t in chat:
        p = current.work_prob(t)
        conf = p if p >= 0.5 else 1.0 - p
        if conf >= PSEUDO_CONF_THRESHOLD:
            weak_texts.append(t)
            weak_labels.append(1 if p >= 0.5 else 0)
    print(f"  pseudo-labels @>= {PSEUDO_CONF_THRESHOLD}: {len(weak_texts)} "
          f"(work={sum(weak_labels)}, personal={len(weak_labels) - sum(weak_labels)})")

    # ── Held-out evaluation: train on gold_train + weak, eval on gold_test ──
    print("\n--- Held-out evaluation (eval on gold only) ---")
    g_tr_t, g_te_t, g_tr_l, g_te_l = train_test_split(
        gold_texts, gold_labels, test_size=0.2, random_state=42, stratify=gold_labels
    )
    eval_texts = g_tr_t + weak_texts
    eval_labels = g_tr_l + weak_labels
    eval_weights = [GOLD_WEIGHT * (GOLD_WORK_MULT if l == 1 else 1.0) for l in g_tr_l] \
                   + [WEAK_WEIGHT] * len(weak_texts)
    vec_e, lr_e = fit_model(eval_texts, eval_labels, eval_weights)

    new_pred = predict_labels(vec_e, lr_e, g_te_t)
    cur_pred = [current.predict(t) for t in g_te_t]
    report("CURRENT model on held-out gold", g_te_l, cur_pred)
    report("NEW model on held-out gold", g_te_l, new_pred)

    print("\n--- Tricky examples (NEW model) ---")
    tr_pred = predict_labels(vec_e, lr_e, [t for t, _ in TRICKY])
    for (text, expected), pred in zip(TRICKY, tr_pred):
        label = "work" if pred == 1 else "personal"
        mark = "OK " if label == expected else "XX "
        print(f"  {mark} {label:8s} | expected {expected:8s} | {text}")

    # ── Final model: refit on ALL gold + weak, export ──
    print("\nFitting final model on all gold + pseudo-labels...")
    all_texts = gold_texts + weak_texts
    all_labels = gold_labels + weak_labels
    all_weights = [GOLD_WEIGHT * (GOLD_WORK_MULT if l == 1 else 1.0) for l in gold_labels] \
                  + [WEAK_WEIGHT] * len(weak_texts)
    vec, lr = fit_model(all_texts, all_labels, all_weights)

    vocab = {term: int(idx) for term, idx in vec.vocabulary_.items()}
    model = {
        "vocabulary": vocab,
        "idf": vec.idf_.tolist(),
        "config": {
            "max_features": MAX_FEATURES,
            "ngram_range": [NGRAM_MIN, NGRAM_MAX],
            "sublinear_tf": SUBLINEAR_TF,
            "norm": NORM,
        },
        "classifier": {
            "coef": lr.coef_[0].tolist(),
            "intercept": float(lr.intercept_[0]),
        },
    }
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(model, f, ensure_ascii=False)
    print(f"\nWrote candidate model: {out_path}")
    print(f"  vocab size: {len(vocab)}  |  intercept: {model['classifier']['intercept']:.4f}")


if __name__ == "__main__":
    main()
