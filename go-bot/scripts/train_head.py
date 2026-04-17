"""
Train a logistic regression classifier head on top of sentence embeddings.

Collects labeled data from three sources:
1. labeling_batch.csv (250 manual labels)
2. Work review: 100 messages classified as work by old model (with corrections)
3. Personal review: 100 messages classified as personal by old model (with corrections)

Exports weights to go-bot/model/weights.json for Go inference.
"""

import csv
import json
import os
import random
import sys

import numpy as np
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report
from sentence_transformers import SentenceTransformer

# Paths
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
MODEL_DIR = os.path.join(PROJECT_ROOT, "go-bot", "model")

# Add project root to path so we can import the old classifier
sys.path.insert(0, PROJECT_ROOT)
from work_classifier import WorkClassifier


def extract_text(msg):
    """Extract text from a Telegram message (handles string and list formats)."""
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
    """Load all text messages from chat history."""
    messages = []
    for fname in ["chat/result.json", "chat/result_2.json"]:
        path = os.path.join(PROJECT_ROOT, fname)
        if os.path.exists(path):
            with open(path, "r", encoding="utf-8") as f:
                data = json.load(f)
            for msg in data.get("messages", []):
                if msg.get("type") == "message":
                    text = extract_text(msg)
                    if text.strip():
                        messages.append(text)
    return messages


def load_labeling_batch():
    """Load labels from labeling_batch.csv (semicolon-delimited)."""
    texts, labels = [], []
    path = os.path.join(PROJECT_ROOT, "labeling_batch.csv")
    with open(path, "r", encoding="utf-8") as f:
        reader = csv.DictReader(f, delimiter=";")
        for row in reader:
            text = row.get("text", "").strip()
            label = row.get("your_label", "").strip().lower()
            if text and label in ("w", "p"):
                texts.append(text)
                labels.append(1 if label == "w" else 0)
    return texts, labels


def get_work_review(classifier, chat_messages):
    """
    Get 100 messages classified as work by old model.
    Filter >= 0.90 work confidence, sort desc, deduplicate by first 50 chars, take first 100.
    Corrections: certain 1-indexed IDs are NOT work.
    """
    NOT_WORK_IDS = {3, 4, 5, 6, 7, 8, 18, 19, 33, 46, 47, 48, 49, 50, 51, 52, 53, 54,
                    55, 56, 57, 58, 59, 60, 61, 69, 70, 73, 81, 82, 83, 84, 86, 96, 98}

    scored = []
    for text in chat_messages:
        result = classifier.predict(text)
        if result["label"] == "work":
            work_conf = result["confidence"]
        else:
            work_conf = 1.0 - result["confidence"]
        if work_conf >= 0.90:
            scored.append((text, work_conf))

    # Sort by confidence descending
    scored.sort(key=lambda x: x[1], reverse=True)

    # Deduplicate by first 50 chars
    seen = set()
    unique = []
    for text, conf in scored:
        key = text[:50]
        if key not in seen:
            seen.add(key)
            unique.append(text)

    # Take first 100
    review_list = unique[:100]

    # Apply corrections: 1-indexed
    texts, labels = [], []
    for i, text in enumerate(review_list):
        idx = i + 1  # 1-indexed
        if idx in NOT_WORK_IDS:
            labels.append(0)  # Actually personal
        else:
            labels.append(1)  # Confirmed work
        texts.append(text)

    return texts, labels


def get_personal_review(classifier, chat_messages):
    """
    Get 100 messages classified as personal by old model.
    Filter >= 0.90 personal confidence and len >= 8, deduplicate, random sample 100.
    Corrections: IDs 9 and 44 ARE work.
    """
    WORK_IDS = {9, 44}

    scored = []
    for text in chat_messages:
        result = classifier.predict(text)
        if result["label"] == "personal":
            personal_conf = result["confidence"]
        else:
            personal_conf = 1.0 - result["confidence"]
        if personal_conf >= 0.90 and len(text) >= 8:
            scored.append(text)

    # Deduplicate by first 50 chars
    seen = set()
    unique = []
    for text in scored:
        key = text[:50]
        if key not in seen:
            seen.add(key)
            unique.append(text)

    # Random sample
    random.seed(123)
    review_list = random.sample(unique, 100)

    # Apply corrections: 1-indexed
    texts, labels = [], []
    for i, text in enumerate(review_list):
        idx = i + 1  # 1-indexed
        if idx in WORK_IDS:
            labels.append(1)  # Actually work
        else:
            labels.append(0)  # Confirmed personal
        texts.append(text)

    return texts, labels


def main():
    print("Loading old classifier...")
    classifier = WorkClassifier()

    print("Loading chat messages...")
    chat_messages = load_chat_messages()
    print(f"  Loaded {len(chat_messages)} messages")

    # Source 1: labeling batch
    print("\nSource 1: labeling_batch.csv")
    batch_texts, batch_labels = load_labeling_batch()
    print(f"  {len(batch_texts)} samples (work={sum(batch_labels)}, personal={len(batch_labels)-sum(batch_labels)})")

    # Source 2: work review
    print("\nSource 2: Work review")
    work_texts, work_labels = get_work_review(classifier, chat_messages)
    print(f"  {len(work_texts)} samples (work={sum(work_labels)}, personal={len(work_labels)-sum(work_labels)})")

    # Source 3: personal review
    print("\nSource 3: Personal review")
    personal_texts, personal_labels = get_personal_review(classifier, chat_messages)
    print(f"  {len(personal_texts)} samples (work={sum(personal_labels)}, personal={len(personal_labels)-sum(personal_labels)})")

    # Combine all
    all_texts = batch_texts + work_texts + personal_texts
    all_labels = batch_labels + work_labels + personal_labels
    print(f"\nTotal: {len(all_texts)} samples (work={sum(all_labels)}, personal={len(all_labels)-sum(all_labels)})")

    # Generate embeddings
    print("\nGenerating embeddings...")
    model = SentenceTransformer("sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2")
    embeddings = model.encode(all_texts, normalize_embeddings=True, show_progress_bar=True)
    print(f"  Embedding shape: {embeddings.shape}")

    # Train classifier
    print("\nTraining LogisticRegression...")
    lr = LogisticRegression(C=1.0, max_iter=1000, class_weight="balanced", random_state=42)
    lr.fit(embeddings, all_labels)

    # Classification report
    preds = lr.predict(embeddings)
    print("\nClassification Report (on training data):")
    print(classification_report(all_labels, preds, target_names=["personal", "work"]))

    # Export weights
    weights = {
        "coef": lr.coef_[0].tolist(),
        "intercept": float(lr.intercept_[0]),
        "classes": ["personal", "work"],
    }

    weights_path = os.path.join(MODEL_DIR, "weights.json")
    with open(weights_path, "w") as f:
        json.dump(weights, f)

    print(f"Weights saved to {weights_path}")
    print(f"  Coefficients: {len(weights['coef'])} values")
    print(f"  Intercept: {weights['intercept']:.6f}")
    print("\nDone!")


if __name__ == "__main__":
    main()
