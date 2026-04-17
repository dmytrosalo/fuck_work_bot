"""Fine-tune paraphrase-multilingual-MiniLM-L12-v2 on chat data, then re-export to ONNX."""
import json
import csv
import os
import sys
import random
import numpy as np

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.join(SCRIPT_DIR, "..", "..")
sys.path.insert(0, PROJECT_ROOT)

random.seed(42)
np.random.seed(42)

# ── 1. Collect all labeled data ──────────────────────────────────────────────

texts, labels = [], []

# CSV batch (250)
csv_path = os.path.join(PROJECT_ROOT, "labeling_batch.csv")
with open(csv_path, "r") as f:
    for r in csv.DictReader(f, delimiter=";"):
        label = r["your_label"].strip().lower()
        text = r["text"].strip()
        if label in ("w", "p") and text:
            texts.append(text)
            labels.append(1 if label == "w" else 0)

print(f"CSV labels: {len(texts)}")

# Work review (100) — regenerate same list
from work_classifier import WorkClassifier
clf_old = WorkClassifier()

def load_chat_texts():
    msgs = []
    for fname in ["chat/result.json", "chat/result_2.json"]:
        fpath = os.path.join(PROJECT_ROOT, fname)
        with open(fpath, "r") as fh:
            data = json.load(fh)
        for m in data["messages"]:
            if m.get("type") != "message": continue
            if m.get("from") == "FuckingWorkTracking": continue
            text = m.get("text", "")
            if isinstance(text, list):
                parts = []
                for part in text:
                    if isinstance(part, str): parts.append(part)
                    elif isinstance(part, dict): parts.append(part.get("text", ""))
                text = "".join(parts)
            text = text.strip()
            if text:
                msgs.append(text)
    return msgs

all_chat = load_chat_texts()

# Work review
all_work_msgs = []
for text in all_chat:
    if len(text) < 5: continue
    r = clf_old.predict(text)
    if r["label"] == "work" and r["confidence"] >= 0.90:
        all_work_msgs.append((text, r["confidence"]))

all_work_msgs.sort(key=lambda x: -x[1])
seen = set()
work_review = []
for text, conf in all_work_msgs:
    short = text[:50].lower()
    if short not in seen:
        seen.add(short)
        work_review.append(text)
    if len(work_review) == 100:
        break

not_work_ids = {3,4,5,6,7,8,18,19,33,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60,61,69,70,73,81,82,83,84,86,96,98}
for i, text in enumerate(work_review, 1):
    texts.append(text)
    labels.append(0 if i in not_work_ids else 1)

# Personal review
all_pers_msgs = []
for text in all_chat:
    if len(text) < 8: continue
    r = clf_old.predict(text)
    if r["label"] == "personal" and r["confidence"] >= 0.90:
        all_pers_msgs.append(text)

seen2 = set()
unique_pers = []
for text in all_pers_msgs:
    short = text[:50].lower()
    if short not in seen2:
        seen2.add(short)
        unique_pers.append(text)
sampled_pers = random.sample(unique_pers, min(100, len(unique_pers)))

is_work_pers = {9, 44}
for i, text in enumerate(sampled_pers, 1):
    texts.append(text)
    labels.append(1 if i in is_work_pers else 0)

print(f"Manual labels: {len(texts)} (work={sum(labels)}, personal={len(labels)-sum(labels)})")

# Auto-label high-confidence from old model for more training data
manual_set = set(texts)
false_pos_words = {"потім", "багато", "постійно"}

auto_texts, auto_labels = [], []
for text in all_chat:
    if text in manual_set or len(text) < 5: continue
    r = clf_old.predict(text)
    if r["confidence"] >= 0.98:
        if r["label"] == "work":
            words = set(text.lower().split())
            if len(words) <= 3 and words.intersection(false_pos_words):
                continue
        auto_texts.append(text)
        auto_labels.append(1 if r["label"] == "work" else 0)

print(f"Auto-labeled (98%+): {len(auto_texts)} (work={sum(auto_labels)}, personal={len(auto_labels)-sum(auto_labels)})")

all_texts = texts + auto_texts
all_labels = labels + auto_labels
print(f"Total training data: {len(all_texts)} (work={sum(all_labels)}, personal={len(all_labels)-sum(all_labels)})")

# ── 2. Fine-tune the embedding model ────────────────────────────────────────

from sentence_transformers import SentenceTransformer, InputExample, losses
from torch.utils.data import DataLoader

model = SentenceTransformer("sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2")

# Create training pairs using contrastive learning
# Group texts by label
work_texts = [t for t, l in zip(all_texts, all_labels) if l == 1]
pers_texts = [t for t, l in zip(all_texts, all_labels) if l == 0]

print(f"\nFine-tuning with {len(work_texts)} work + {len(pers_texts)} personal texts")

# Create pairs: same-class pairs (label=1.0) and diff-class pairs (label=0.0)
train_examples = []

# Same-class pairs (positive)
for _ in range(2000):
    if random.random() < 0.5 and len(work_texts) >= 2:
        a, b = random.sample(work_texts, 2)
        train_examples.append(InputExample(texts=[a, b], label=1.0))
    elif len(pers_texts) >= 2:
        a, b = random.sample(pers_texts, 2)
        train_examples.append(InputExample(texts=[a, b], label=1.0))

# Different-class pairs (negative)
for _ in range(2000):
    a = random.choice(work_texts)
    b = random.choice(pers_texts)
    train_examples.append(InputExample(texts=[a, b], label=0.0))

random.shuffle(train_examples)
print(f"Training pairs: {len(train_examples)}")

train_dataloader = DataLoader(train_examples, shuffle=True, batch_size=32)
train_loss = losses.CosineSimilarityLoss(model)

# Fine-tune
print("Fine-tuning...")
model.fit(
    train_objectives=[(train_dataloader, train_loss)],
    epochs=5,
    warmup_steps=100,
    show_progress_bar=True,
)

# ── 3. Evaluate ─────────────────────────────────────────────────────────────

from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report

# Generate new embeddings with fine-tuned model
print("\nGenerating embeddings with fine-tuned model...")
manual_embeddings = model.encode(texts, normalize_embeddings=True, show_progress_bar=True)

# Train new logistic regression head on manual labels only
clf = LogisticRegression(C=1.0, max_iter=1000, class_weight="balanced", random_state=42)
clf.fit(manual_embeddings, labels)

preds = clf.predict(manual_embeddings)
print("\nTraining set performance (manual labels):")
print(classification_report(labels, preds, target_names=["personal", "work"]))

# Test on tricky examples
print("--- Tricky examples ---")
tricky = [
    ("руді в ахуі", "work"),
    ("делна підараска?", "work"),
    ("я тут просив Делну по роботі", "work"),
    ("а чого руді не каже", "work"),
    ("ну і як тобі аршан ілір", "work"),
    ("маріт звільняється?", "work"),
    ("нрф планінг щотижневий", "work"),
    ("потім душ", "personal"),
    ("смачного!", "personal"),
    ("Завтра останній день відпустки", "personal"),
    ("багато вибачень)", "personal"),
    ("А багато по іпотеці ще?", "personal"),
]

tricky_texts = [t for t, _ in tricky]
tricky_emb = model.encode(tricky_texts, normalize_embeddings=True)
tricky_preds = clf.predict(tricky_emb)
tricky_proba = clf.predict_proba(tricky_emb)

for (text, expected), pred, proba in zip(tricky, tricky_preds, tricky_proba):
    label = "work" if pred == 1 else "personal"
    conf = max(proba)
    correct = "✅" if label == expected else "❌"
    print(f"  {correct} {label:8s} ({conf:.0%}) | expected={expected:8s} | {text}")

# ── 4. Export fine-tuned model to ONNX ───────────────────────────────────────

from optimum.onnxruntime import ORTModelForFeatureExtraction
from transformers import AutoTokenizer

output_dir = os.path.join(SCRIPT_DIR, "..", "model")

print(f"\nExporting fine-tuned model to ONNX at {output_dir}...")

# Save the fine-tuned model to a temp directory first
tmp_dir = os.path.join(SCRIPT_DIR, "..", "model_finetuned_tmp")
model.save(tmp_dir)

# Export to ONNX
ort_model = ORTModelForFeatureExtraction.from_pretrained(tmp_dir, export=True)
tokenizer = AutoTokenizer.from_pretrained(tmp_dir)

ort_model.save_pretrained(output_dir)
tokenizer.save_pretrained(output_dir)

# Clean up temp
import shutil
shutil.rmtree(tmp_dir, ignore_errors=True)

# Keep only essential files
keep = {"model.onnx", "tokenizer.json", "config.json", "special_tokens_map.json", "tokenizer_config.json", "weights.json"}
for f in os.listdir(output_dir):
    if f not in keep:
        path = os.path.join(output_dir, f)
        if os.path.isfile(path):
            os.remove(path)

print(f"Model exported. Files: {os.listdir(output_dir)}")

# ── 5. Export new weights ────────────────────────────────────────────────────

# Retrain on ALL data (manual + auto) with fine-tuned embeddings
all_embeddings = model.encode(all_texts, normalize_embeddings=True, show_progress_bar=True)
clf_final = LogisticRegression(C=1.0, max_iter=1000, class_weight="balanced", random_state=42)
clf_final.fit(all_embeddings, all_labels)

weights = {
    "coef": clf_final.coef_[0].tolist(),
    "intercept": clf_final.intercept_[0].item(),
    "classes": ["personal", "work"],
}

weights_path = os.path.join(output_dir, "weights.json")
with open(weights_path, "w") as f:
    json.dump(weights, f)

print(f"Weights saved. Coef dims: {len(weights['coef'])}, Intercept: {weights['intercept']:.4f}")

# Final eval on tricky examples with new weights
print("\n--- Final tricky examples (fine-tuned model + new weights) ---")
tricky_emb2 = model.encode(tricky_texts, normalize_embeddings=True)
for (text, expected), emb in zip(tricky, tricky_emb2):
    logit = weights["intercept"]
    for j, c in enumerate(weights["coef"]):
        logit += c * float(emb[j])
    prob = 1.0 / (1.0 + np.exp(-logit))
    label = "work" if prob >= 0.5 else "personal"
    conf = prob if prob >= 0.5 else 1.0 - prob
    correct = "✅" if label == expected else "❌"
    print(f"  {correct} {label:8s} ({conf:.0%}) | expected={expected:8s} | {text}")

print("\nDone! Fine-tuned model ready at go-bot/model/")
