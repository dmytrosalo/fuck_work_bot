# Go Rewrite — Work Classifier Bot

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the Telegram work classifier bot in Go with ONNX-based sentence embeddings (paraphrase-multilingual-MiniLM-L12-v2) and SQLite persistence, deployed on Fly.io.

**Architecture:** Go binary using telebot for Telegram, Hugot for ONNX transformer inference, and modernc.org/sqlite for CGO-free SQLite. The classifier tokenizes messages, generates embeddings via ONNX, and feeds them to a logistic regression head to predict work vs personal.

**Tech Stack:** Go 1.23, github.com/knights-analytics/hugot (ONNX transformer pipelines), modernc.org/sqlite (pure Go SQLite), gopkg.in/telebot.v3 (Telegram bot), Fly.io for deployment.

---

## File Structure

```
go-bot/
├── cmd/bot/main.go              — entry point, bot setup, scheduled jobs
├── internal/
│   ├── classifier/
│   │   ├── classifier.go        — ONNX embedding + logistic regression classifier
│   │   └── classifier_test.go   — classifier tests
│   ├── storage/
│   │   ├── sqlite.go            — SQLite DB: stats, muted, chats
│   │   └── sqlite_test.go       — storage tests
│   └── handlers/
│       ├── handlers.go          — telegram message & command handlers
│       └── handlers_test.go     — handler tests
├── model/                       — ONNX model files (downloaded at build)
│   ├── model.onnx
│   ├── tokenizer.json
│   └── weights.json             — logistic regression weights
├── scripts/
│   ├── export_model.py          — export HuggingFace model to ONNX
│   └── train_head.py            — train logistic regression on embeddings, export weights
├── Dockerfile
├── go.mod
├── go.sum
└── .gitignore (updated)
```

---

### Task 0: Prepare ONNX model and classifier weights (Python, local only)

This task runs locally in Python to prepare the model artifacts that Go will consume.

**Files:**
- Create: `go-bot/scripts/export_model.py`
- Create: `go-bot/scripts/train_head.py`
- Create: `go-bot/model/` directory with artifacts

- [ ] **Step 0.1: Install Python dependencies**

Run:
```bash
pip install sentence-transformers onnx onnxruntime optimum
```

- [ ] **Step 0.2: Create export script**

Create `go-bot/scripts/export_model.py`:

```python
"""Export paraphrase-multilingual-MiniLM-L12-v2 to ONNX"""
from optimum.onnxruntime import ORTModelForFeatureExtraction
from transformers import AutoTokenizer
import shutil
import os

MODEL_NAME = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
OUTPUT_DIR = os.path.join(os.path.dirname(__file__), "..", "model")

print(f"Exporting {MODEL_NAME} to ONNX...")
model = ORTModelForFeatureExtraction.from_pretrained(MODEL_NAME, export=True)
tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)

os.makedirs(OUTPUT_DIR, exist_ok=True)
model.save_pretrained(OUTPUT_DIR)
tokenizer.save_pretrained(OUTPUT_DIR)

# Keep only essential files
keep = {"model.onnx", "tokenizer.json", "config.json", "special_tokens_map.json", "tokenizer_config.json"}
for f in os.listdir(OUTPUT_DIR):
    if f not in keep:
        path = os.path.join(OUTPUT_DIR, f)
        if os.path.isfile(path):
            os.remove(path)

print(f"Model exported to {OUTPUT_DIR}")
print(f"Files: {os.listdir(OUTPUT_DIR)}")
```

- [ ] **Step 0.3: Run the export**

Run:
```bash
cd go-bot && python scripts/export_model.py
```

Expected: `model/` directory with `model.onnx` (~134MB), `tokenizer.json`, `config.json`

- [ ] **Step 0.4: Create training script for classifier head**

Create `go-bot/scripts/train_head.py`:

```python
"""Train logistic regression on sentence embeddings, export weights as JSON"""
import json
import csv
import numpy as np
from sentence_transformers import SentenceTransformer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report
import os

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.join(SCRIPT_DIR, "..", "..")

# 1. Collect all manual labels
texts = []
labels = []

# From CSV
csv_path = os.path.join(PROJECT_ROOT, "labeling_batch.csv")
with open(csv_path, "r") as f:
    for r in csv.DictReader(f, delimiter=";"):
        label = r["your_label"].strip().lower()
        text = r["text"].strip()
        if label in ("w", "p") and text:
            texts.append(text)
            labels.append(1 if label == "w" else 0)

print(f"CSV labels: {len(texts)}")

# From work review (100 messages — IDs that are NOT work)
not_work_ids = {3,4,5,6,7,8,18,19,33,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60,61,69,70,73,81,82,83,84,86,96,98}

# Regenerate work review list from chat data
import sys
sys.path.insert(0, PROJECT_ROOT)
from work_classifier import WorkClassifier
clf_old = WorkClassifier()

all_work_msgs = []
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
        if text and len(text) >= 5:
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

for i, text in enumerate(work_review, 1):
    texts.append(text)
    labels.append(0 if i in not_work_ids else 1)

print(f"After work review: {len(texts)}")

# From personal review (100 messages — IDs that ARE work)
import random
random.seed(123)

all_pers_msgs = []
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
        if text and len(text) >= 8:
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

print(f"Total labels: {len(texts)} (work={sum(labels)}, personal={len(labels)-sum(labels)})")

# 2. Generate embeddings
print("Generating embeddings...")
model = SentenceTransformer("sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2")
embeddings = model.encode(texts, show_progress_bar=True, normalize_embeddings=True)

print(f"Embedding shape: {embeddings.shape}")

# 3. Train logistic regression
clf = LogisticRegression(C=1.0, max_iter=1000, class_weight="balanced", random_state=42)
clf.fit(embeddings, labels)

preds = clf.predict(embeddings)
print("\nTraining set performance:")
print(classification_report(labels, preds, target_names=["personal", "work"]))

# 4. Export weights as JSON
weights = {
    "coef": clf.coef_[0].tolist(),
    "intercept": clf.intercept_[0].item(),
    "classes": ["personal", "work"],
}

output_path = os.path.join(SCRIPT_DIR, "..", "model", "weights.json")
with open(output_path, "w") as f:
    json.dump(weights, f)

print(f"Weights saved to {output_path}")
print(f"Coefficient dimensions: {len(weights['coef'])}")
```

- [ ] **Step 0.5: Run training**

Run:
```bash
cd go-bot && python scripts/train_head.py
```

Expected: `model/weights.json` with 384-dimensional weight vector + intercept.

- [ ] **Step 0.6: Verify model directory**

Run:
```bash
ls -lh go-bot/model/
```

Expected files:
- `model.onnx` (~134MB)
- `tokenizer.json` (~700KB)
- `config.json` (<1KB)
- `weights.json` (<10KB)
- `special_tokens_map.json` (<1KB)
- `tokenizer_config.json` (<1KB)

---

### Task 1: Initialize Go module and dependencies

**Files:**
- Create: `go-bot/go.mod`
- Create: `go-bot/cmd/bot/main.go` (stub)

- [ ] **Step 1.1: Initialize Go module**

Run:
```bash
cd go-bot && go mod init github.com/dmytrosalo/fuck-work-bot
```

- [ ] **Step 1.2: Add dependencies**

Run:
```bash
cd go-bot
go get gopkg.in/telebot.v3
go get modernc.org/sqlite
go get github.com/knights-analytics/hugot
go get github.com/knights-analytics/hugot/pipelines
```

- [ ] **Step 1.3: Create stub main.go**

Create `go-bot/cmd/bot/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("fuck-work-bot starting...")
}
```

- [ ] **Step 1.4: Verify it compiles**

Run:
```bash
cd go-bot && go build ./cmd/bot/
```

Expected: clean build, no errors.

- [ ] **Step 1.5: Commit**

```bash
git add go-bot/
git commit -m "feat: initialize Go module with dependencies"
```

---

### Task 2: SQLite storage layer

**Files:**
- Create: `go-bot/internal/storage/sqlite.go`
- Create: `go-bot/internal/storage/sqlite_test.go`

- [ ] **Step 2.1: Write storage tests**

Create `go-bot/internal/storage/sqlite_test.go`:

```go
package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpdateAndGetStats(t *testing.T) {
	db := tempDB(t)

	db.UpdateStats("123", "Alice", true)
	db.UpdateStats("123", "Alice", true)
	db.UpdateStats("123", "Alice", false)

	stats, err := db.GetAllStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 user, got %d", len(stats))
	}
	if stats[0].Work != 2 || stats[0].Personal != 1 {
		t.Fatalf("expected work=2, personal=1, got work=%d, personal=%d", stats[0].Work, stats[0].Personal)
	}
}

func TestMuteUnmute(t *testing.T) {
	db := tempDB(t)

	db.Mute("123")
	if !db.IsMuted("123") {
		t.Fatal("expected muted")
	}

	db.Unmute("123")
	if db.IsMuted("123") {
		t.Fatal("expected not muted")
	}
}

func TestTrackChat(t *testing.T) {
	db := tempDB(t)

	db.TrackChat("chat1")
	db.TrackChat("chat2")
	db.TrackChat("chat1") // duplicate

	chats, err := db.GetActiveChats()
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 2 {
		t.Fatalf("expected 2 chats, got %d", len(chats))
	}
}

func TestDailyStats(t *testing.T) {
	db := tempDB(t)

	db.UpdateDailyStats("123", "Alice", true)
	db.UpdateDailyStats("123", "Alice", false)

	stats, err := db.GetDailyStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 user, got %d", len(stats))
	}
	if stats[0].Work != 1 || stats[0].Personal != 1 {
		t.Fatalf("expected work=1, personal=1, got work=%d, personal=%d", stats[0].Work, stats[0].Personal)
	}

	db.ResetDailyStats()
	stats, err = db.GetDailyStats()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Fatalf("expected 0 after reset, got %d", len(stats))
	}
}
```

- [ ] **Step 2.2: Run tests to verify they fail**

Run:
```bash
cd go-bot && go test ./internal/storage/ -v
```

Expected: compilation errors (types not defined yet).

- [ ] **Step 2.3: Implement storage**

Create `go-bot/internal/storage/sqlite.go`:

```go
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type UserStats struct {
	UserID   string
	Name     string
	Work     int
	Personal int
}

type DB struct {
	db *sql.DB
}

func New(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &DB{db: db}, nil
}

func migrate(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS stats (
			user_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			work INTEGER NOT NULL DEFAULT 0,
			personal INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS daily_stats (
			user_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			work INTEGER NOT NULL DEFAULT 0,
			personal INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS muted (
			user_id TEXT PRIMARY KEY
		);
		CREATE TABLE IF NOT EXISTS chats (
			chat_id TEXT PRIMARY KEY
		);
	`
	_, err := db.Exec(schema)
	return err
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) UpdateStats(userID, name string, isWork bool) {
	col := "personal"
	if isWork {
		col = "work"
	}
	query := fmt.Sprintf(`
		INSERT INTO stats (user_id, name, %s) VALUES (?, ?, 1)
		ON CONFLICT(user_id) DO UPDATE SET name=?, %s=%s+1
	`, col, col, col)
	d.db.Exec(query, userID, name, name)
}

func (d *DB) GetAllStats() ([]UserStats, error) {
	rows, err := d.db.Query("SELECT user_id, name, work, personal FROM stats ORDER BY (work+personal) DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []UserStats
	for rows.Next() {
		var s UserStats
		if err := rows.Scan(&s.UserID, &s.Name, &s.Work, &s.Personal); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

func (d *DB) UpdateDailyStats(userID, name string, isWork bool) {
	col := "personal"
	if isWork {
		col = "work"
	}
	query := fmt.Sprintf(`
		INSERT INTO daily_stats (user_id, name, %s) VALUES (?, ?, 1)
		ON CONFLICT(user_id) DO UPDATE SET name=?, %s=%s+1
	`, col, col, col)
	d.db.Exec(query, userID, name, name)
}

func (d *DB) GetDailyStats() ([]UserStats, error) {
	rows, err := d.db.Query("SELECT user_id, name, work, personal FROM daily_stats ORDER BY (work+personal) DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []UserStats
	for rows.Next() {
		var s UserStats
		if err := rows.Scan(&s.UserID, &s.Name, &s.Work, &s.Personal); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

func (d *DB) ResetDailyStats() {
	d.db.Exec("DELETE FROM daily_stats")
}

func (d *DB) Mute(userID string) {
	d.db.Exec("INSERT OR IGNORE INTO muted (user_id) VALUES (?)", userID)
}

func (d *DB) Unmute(userID string) {
	d.db.Exec("DELETE FROM muted WHERE user_id = ?", userID)
}

func (d *DB) IsMuted(userID string) bool {
	var count int
	d.db.QueryRow("SELECT COUNT(*) FROM muted WHERE user_id = ?", userID).Scan(&count)
	return count > 0
}

func (d *DB) TrackChat(chatID string) {
	d.db.Exec("INSERT OR IGNORE INTO chats (chat_id) VALUES (?)", chatID)
}

func (d *DB) GetActiveChats() ([]string, error) {
	rows, err := d.db.Query("SELECT chat_id FROM chats")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		chats = append(chats, id)
	}
	return chats, nil
}
```

- [ ] **Step 2.4: Run tests**

Run:
```bash
cd go-bot && go test ./internal/storage/ -v
```

Expected: all 4 tests PASS.

- [ ] **Step 2.5: Commit**

```bash
git add go-bot/internal/storage/
git commit -m "feat: add SQLite storage layer with stats, mute, chat tracking"
```

---

### Task 3: ONNX classifier

**Files:**
- Create: `go-bot/internal/classifier/classifier.go`
- Create: `go-bot/internal/classifier/classifier_test.go`

- [ ] **Step 3.1: Write classifier tests**

Create `go-bot/internal/classifier/classifier_test.go`:

```go
package classifier

import (
	"os"
	"testing"
)

func getModelDir() string {
	// Default to model/ in project root
	dir := os.Getenv("MODEL_DIR")
	if dir == "" {
		dir = "../../model"
	}
	return dir
}

func TestClassifierLoads(t *testing.T) {
	modelDir := getModelDir()
	if _, err := os.Stat(modelDir + "/model.onnx"); os.IsNotExist(err) {
		t.Skip("model files not found, skipping integration test")
	}

	c, err := New(modelDir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
}

func TestClassifyWork(t *testing.T) {
	modelDir := getModelDir()
	if _, err := os.Stat(modelDir + "/model.onnx"); os.IsNotExist(err) {
		t.Skip("model files not found, skipping integration test")
	}

	c, err := New(modelDir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	workMessages := []string{
		"нрф планінг щотижневий",
		"деплой на прод зробили",
		"маріт звільняється?",
		"зараз ще один созвон буде",
		"делна підараска?",
	}

	for _, msg := range workMessages {
		result := c.Predict(msg)
		if !result.IsWork {
			t.Errorf("expected work for %q, got personal (conf=%.2f)", msg, result.Confidence)
		}
	}
}

func TestClassifyPersonal(t *testing.T) {
	modelDir := getModelDir()
	if _, err := os.Stat(modelDir + "/model.onnx"); os.IsNotExist(err) {
		t.Skip("model files not found, skipping integration test")
	}

	c, err := New(modelDir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	personalMessages := []string{
		"потім душ",
		"смачного!",
		"Завтра останній день відпустки",
		"я їду до мами сьогодні",
		"п'ятниця нарешті",
	}

	for _, msg := range personalMessages {
		result := c.Predict(msg)
		if result.IsWork {
			t.Errorf("expected personal for %q, got work (conf=%.2f)", msg, result.Confidence)
		}
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail**

Run:
```bash
cd go-bot && go test ./internal/classifier/ -v
```

Expected: compilation errors (types not defined).

- [ ] **Step 3.3: Implement classifier**

Create `go-bot/internal/classifier/classifier.go`:

```go
package classifier

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

type Result struct {
	Label      string  // "work" or "personal"
	Confidence float64 // 0.0 to 1.0
	IsWork     bool
}

type weights struct {
	Coef      []float64 `json:"coef"`
	Intercept float64   `json:"intercept"`
}

type Classifier struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
	weights  weights
}

func New(modelDir string) (*Classifier, error) {
	// Load logistic regression weights
	wData, err := os.ReadFile(modelDir + "/weights.json")
	if err != nil {
		return nil, fmt.Errorf("read weights: %w", err)
	}

	var w weights
	if err := json.Unmarshal(wData, &w); err != nil {
		return nil, fmt.Errorf("parse weights: %w", err)
	}

	// Create Hugot session with ONNX runtime
	session, err := hugot.NewSession(
		hugot.WithOnnxLibraryPath(""), // uses default system path
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// Create feature extraction pipeline
	pipeline, err := hugot.NewPipeline(session, pipelines.NewFeatureExtractionPipeline, modelDir)
	if err != nil {
		session.Destroy()
		return nil, fmt.Errorf("create pipeline: %w", err)
	}

	return &Classifier{
		session:  session,
		pipeline: pipeline,
		weights:  w,
	}, nil
}

func (c *Classifier) Close() {
	if c.session != nil {
		c.session.Destroy()
	}
}

func (c *Classifier) Predict(text string) Result {
	if text == "" {
		return Result{Label: "personal", Confidence: 1.0, IsWork: false}
	}

	// Get embeddings from transformer model
	batchResult, err := c.pipeline.RunPipeline([]string{text})
	if err != nil {
		return Result{Label: "personal", Confidence: 1.0, IsWork: false}
	}

	embeddings := batchResult.GetOutput()
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return Result{Label: "personal", Confidence: 1.0, IsWork: false}
	}

	// Mean pooling: average over token dimension to get sentence embedding
	embedding := meanPool(embeddings[0])

	// Normalize embedding
	embedding = normalize(embedding)

	// Logistic regression: sigmoid(dot(coef, embedding) + intercept)
	score := c.weights.Intercept
	for i, v := range embedding {
		if i < len(c.weights.Coef) {
			score += c.weights.Coef[i] * float64(v)
		}
	}

	prob := sigmoid(score)
	isWork := prob >= 0.5
	confidence := prob
	if !isWork {
		confidence = 1.0 - prob
	}

	label := "personal"
	if isWork {
		label = "work"
	}

	return Result{
		Label:      label,
		Confidence: confidence,
		IsWork:     isWork,
	}
}

func meanPool(tokenEmbeddings [][]float32) []float32 {
	if len(tokenEmbeddings) == 0 {
		return nil
	}
	dim := len(tokenEmbeddings[0])
	result := make([]float32, dim)
	for _, token := range tokenEmbeddings {
		for i, v := range token {
			result[i] += v
		}
	}
	n := float32(len(tokenEmbeddings))
	for i := range result {
		result[i] /= n
	}
	return result
}

func normalize(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return v
	}
	result := make([]float32, len(v))
	for i, x := range v {
		result[i] = float32(float64(x) / norm)
	}
	return result
}

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}
```

Note: The Hugot API may differ slightly depending on the version. The implementer should check the Hugot documentation and adapt the pipeline creation and output extraction accordingly. The key pattern is: create session → create feature extraction pipeline from model dir → run pipeline on text → get embeddings → apply logistic regression.

- [ ] **Step 3.4: Run tests**

Run:
```bash
cd go-bot && go test ./internal/classifier/ -v -timeout 60s
```

Expected: all 3 tests PASS (TestClassifyWork and TestClassifyPersonal may need tuning based on actual model output — adjust confidence or expected labels if the embedding model produces different results).

- [ ] **Step 3.5: Commit**

```bash
git add go-bot/internal/classifier/
git commit -m "feat: add ONNX-based work classifier with Hugot"
```

---

### Task 4: Telegram handlers

**Files:**
- Create: `go-bot/internal/handlers/handlers.go`
- Create: `go-bot/internal/handlers/handlers_test.go`

- [ ] **Step 4.1: Write handler tests**

Create `go-bot/internal/handlers/handlers_test.go`:

```go
package handlers

import (
	"testing"
)

func TestRandomRoast(t *testing.T) {
	r1 := randomRoast()
	if r1 == "" {
		t.Fatal("expected non-empty roast")
	}

	// Should return different values over many calls (probabilistic)
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		seen[randomRoast()] = true
	}
	if len(seen) < 3 {
		t.Fatalf("expected variety in roasts, got %d unique", len(seen))
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail**

Run:
```bash
cd go-bot && go test ./internal/handlers/ -v
```

Expected: compilation error.

- [ ] **Step 4.3: Implement handlers**

Create `go-bot/internal/handlers/handlers.go`:

```go
package handlers

import (
	"fmt"
	"math/rand"
	"strconv"

	"github.com/dmytrosalo/fuck-work-bot/internal/classifier"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

var workRoasts = []string{
	"О, хтось знову не може відпустити роботу навіть у чаті",
	"Так, ми всі вражені твоєю зайнятістю. Ні, насправді ні.",
	"Чат для відпочинку, а не для твоїх робочих драм",
	"Ти взагалі вмієш говорити про щось крім роботи?",
	"Вау, робота. Як оригінально. Всім дуже цікаво.",
	"Хтось явно не вміє відділяти роботу від життя",
	"Знову ця корпоративна нудьга в чаті...",
	"Ми зрозуміли, ти працюєш. Можна далі жити?",
	"Робота-робота... А особистість у тебе є?",
	"Чергова робоча тема? Як несподівано від тебе.",
	"Ти на годиннику чи просто не можеш зупинитись?",
	"Слухай, є інші теми для розмов. Google допоможе.",
	"О ні, знову хтось важливий зі своєю важливою роботою",
	"Так, так, дедлайни, мітинги, ми в захваті. Далі що?",
	"Може краще в робочий чат? Або в щоденник?",
	"Друже, це чат, а не твій LinkedIn",
	"Знову робочі проблеми? Психотерапевт дешевший",
	"Цікаво, ти й уві сні про роботу говориш?",
	"Нагадую: тут люди відпочивають від роботи. Ну, крім тебе.",
	"Ого, ще одне повідомлення про роботу! Який сюрприз!",
	"Може хоч раз поговоримо про щось людське?",
	"Твій роботодавець не платить за рекламу в цьому чаті",
	"Роботоголізм — це діагноз, до речі",
	"Дивно, що ти ще не створив окремий чат для своїх тікетів",
	"О, знову ти зі своїми важливими справами. Фанфари!",
	"Тут є правило: хто пише про роботу — той лох",
	"Знаєш що крутіше за роботу? Буквально все.",
	"А ти точно не бот? Бо тільки боти так багато про роботу",
	"Ми не твої колеги, можеш розслабитись",
	"Хтось забув вимкнути робочий режим",
}

func randomRoast() string {
	return workRoasts[rand.Intn(len(workRoasts))]
}

type Bot struct {
	clf *classifier.Classifier
	db  *storage.DB
}

func New(clf *classifier.Classifier, db *storage.DB) *Bot {
	return &Bot{clf: clf, db: db}
}

func (b *Bot) Register(bot *tele.Bot) {
	bot.Handle("/start", b.handleStart)
	bot.Handle("/check", b.handleCheck)
	bot.Handle("/stats", b.handleStats)
	bot.Handle("/mute", b.handleMute)
	bot.Handle("/unmute", b.handleUnmute)
	bot.Handle(tele.OnText, b.handleMessage)
}

func (b *Bot) handleStart(c tele.Context) error {
	return c.Send(
		"*Привіт!* Я слідкую за робочими повідомленнями\n\n"+
			"Команди:\n"+
			"/check <text> - перевірити текст\n"+
			"/stats - статистика\n"+
			"/mute - вимкнути трекінг\n"+
			"/unmute - увімкнути трекінг",
		&tele.SendOptions{ParseMode: tele.ModeMarkdown},
	)
}

func (b *Bot) handleCheck(c tele.Context) error {
	text := c.Message().Payload
	if text == "" {
		return c.Send("Usage: /check <text>")
	}

	result := b.clf.Predict(text)
	emoji := "😎"
	if result.IsWork {
		emoji = "💼"
	}

	return c.Send(fmt.Sprintf("%s %s\nConfidence: %.0f%%", emoji, result.Label, result.Confidence*100))
}

func (b *Bot) handleStats(c tele.Context) error {
	stats, err := b.db.GetAllStats()
	if err != nil || len(stats) == 0 {
		return c.Send("📊 Немає статистики")
	}

	msg := "📊 Статистика:\n\n"
	totalWork, totalPersonal := 0, 0

	for _, s := range stats {
		total := s.Work + s.Personal
		totalWork += s.Work
		totalPersonal += s.Personal
		if total > 0 {
			pct := float64(s.Work) / float64(total) * 100
			msg += fmt.Sprintf("👤 %s: %d msgs (💼 %.0f%%)\n", s.Name, total, pct)
		}
	}

	grand := totalWork + totalPersonal
	if grand > 0 {
		msg += fmt.Sprintf("\n📈 Загалом: %d\n", grand)
		msg += fmt.Sprintf("💼 Робота: %d (%.0f%%)\n", totalWork, float64(totalWork)/float64(grand)*100)
		msg += fmt.Sprintf("😎 Персональне: %d (%.0f%%)", totalPersonal, float64(totalPersonal)/float64(grand)*100)
	}

	return c.Send(msg)
}

func (b *Bot) handleMute(c tele.Context) error {
	b.db.Mute(strconv.FormatInt(c.Sender().ID, 10))
	return c.Send("🔇 Трекінг вимкнено.\n/unmute щоб увімкнути назад")
}

func (b *Bot) handleUnmute(c tele.Context) error {
	b.db.Unmute(strconv.FormatInt(c.Sender().ID, 10))
	return c.Send("🔊 Трекінг увімкнено! Тепер я знову слідкую за тобою 👀")
}

func (b *Bot) handleMessage(c tele.Context) error {
	text := c.Text()
	if text == "" {
		return nil
	}

	userID := strconv.FormatInt(c.Sender().ID, 10)
	chatID := strconv.FormatInt(c.Chat().ID, 10)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	if userName == "" {
		userName = userID
	}

	// Track chat
	b.db.TrackChat(chatID)

	// Skip muted users
	if b.db.IsMuted(userID) {
		return nil
	}

	// Classify
	result := b.clf.Predict(text)

	// Update stats
	b.db.UpdateStats(userID, userName, result.IsWork)
	b.db.UpdateDailyStats(userID, userName, result.IsWork)

	// Reply if work with high confidence
	if result.IsWork && result.Confidence >= 0.95 {
		// Try to react with clown emoji
		_ = c.Bot().React(c.Chat(), c.Message(), tele.ReactionOptions{
			Reactions: []tele.Reaction{{Type: "emoji", Emoji: "🤡"}},
		})

		roast := randomRoast()
		return c.Reply(fmt.Sprintf("%s (%.0f%%)", roast, result.Confidence*100))
	}

	return nil
}

// DailyReport sends daily stats to all active chats and resets
func (b *Bot) DailyReport(bot *tele.Bot) {
	stats, err := b.db.GetDailyStats()
	if err != nil || len(stats) == 0 {
		return
	}

	msg := "📊 *Щоденний звіт*\n\n"

	for _, s := range stats {
		total := s.Work + s.Personal
		if total > 0 {
			pct := float64(s.Work) / float64(total) * 100
			msg += fmt.Sprintf("👤 *%s*: %d повідомлень (💼 %.0f%% робочих)\n", s.Name, total, pct)
		}
	}

	chats, _ := b.db.GetActiveChats()
	for _, chatID := range chats {
		id, err := strconv.ParseInt(chatID, 10, 64)
		if err != nil {
			continue
		}
		bot.Send(&tele.Chat{ID: id}, msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	b.db.ResetDailyStats()
}
```

- [ ] **Step 4.4: Run tests**

Run:
```bash
cd go-bot && go test ./internal/handlers/ -v
```

Expected: PASS.

- [ ] **Step 4.5: Commit**

```bash
git add go-bot/internal/handlers/
git commit -m "feat: add telegram command and message handlers"
```

---

### Task 5: Main entry point with scheduled jobs

**Files:**
- Modify: `go-bot/cmd/bot/main.go`

- [ ] **Step 5.1: Implement main.go**

Replace `go-bot/cmd/bot/main.go`:

```go
package main

import (
	"log"
	"os"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/classifier"
	"github.com/dmytrosalo/fuck-work-bot/internal/handlers"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		modelDir = "./model"
	}

	// Init storage
	db, err := storage.New(dataDir + "/bot.db")
	if err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}
	defer db.Close()
	log.Println("Storage initialized")

	// Init classifier
	clf, err := classifier.New(modelDir)
	if err != nil {
		log.Fatalf("Failed to init classifier: %v", err)
	}
	defer clf.Close()
	log.Println("Classifier loaded")

	// Init bot
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := tele.NewBot(pref)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Register handlers
	h := handlers.New(clf, db)
	h.Register(bot)

	// Schedule daily report at 23:00 Kyiv time
	go scheduleDailyReport(bot, h)

	log.Println("Bot starting...")
	bot.Start()
}

func scheduleDailyReport(bot *tele.Bot, h *handlers.Bot) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		log.Printf("Failed to load Kyiv timezone: %v, using UTC+2", err)
		kyiv = time.FixedZone("Kyiv", 2*60*60)
	}

	for {
		now := time.Now().In(kyiv)
		// Next 23:00 Kyiv time
		next := time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, kyiv)
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		sleepDuration := time.Until(next)
		log.Printf("Next daily report in %v (at %s)", sleepDuration.Round(time.Minute), next.Format("15:04 MST"))

		time.Sleep(sleepDuration)

		log.Println("Sending daily report...")
		h.DailyReport(bot)
	}
}
```

- [ ] **Step 5.2: Verify it compiles**

Run:
```bash
cd go-bot && go build ./cmd/bot/
```

Expected: clean build.

- [ ] **Step 5.3: Commit**

```bash
git add go-bot/cmd/bot/main.go
git commit -m "feat: add main entry point with daily report scheduler"
```

---

### Task 6: Dockerfile and deployment config

**Files:**
- Create: `go-bot/Dockerfile`
- Update: `go-bot/.gitignore`

- [ ] **Step 6.1: Create Dockerfile**

Create `go-bot/Dockerfile`:

```dockerfile
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# Install ONNX Runtime
RUN apt-get update && apt-get install -y wget && \
    wget -q https://github.com/microsoft/onnxruntime/releases/download/v1.20.0/onnxruntime-linux-x64-1.20.0.tgz && \
    tar -xzf onnxruntime-linux-x64-1.20.0.tgz && \
    cp onnxruntime-linux-x64-1.20.0/lib/* /usr/local/lib/ && \
    cp -r onnxruntime-linux-x64-1.20.0/include/* /usr/local/include/ && \
    ldconfig && \
    rm -rf onnxruntime-linux-x64-1.20.0*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o bot ./cmd/bot/

# Runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy ONNX Runtime
COPY --from=builder /usr/local/lib/libonnxruntime* /usr/local/lib/
RUN ldconfig

# Copy binary and model
COPY --from=builder /app/bot /bot
COPY --from=builder /app/model/ /model/

ENV MODEL_DIR=/model
ENV DATA_DIR=/data

CMD ["/bot"]
```

- [ ] **Step 6.2: Update .gitignore**

Create/update `go-bot/.gitignore`:

```
# Binary
bot

# Model files (large, downloaded separately)
model/model.onnx

# IDE
.idea/
.vscode/
```

- [ ] **Step 6.3: Commit**

```bash
git add go-bot/Dockerfile go-bot/.gitignore
git commit -m "feat: add Dockerfile for Fly.io deployment with ONNX Runtime"
```

---

### Task 7: Integration test — full local run

- [ ] **Step 7.1: Build locally**

Run:
```bash
cd go-bot && go build ./cmd/bot/
```

Expected: clean build.

- [ ] **Step 7.2: Test with bot token locally**

Run:
```bash
cd go-bot && TELEGRAM_BOT_TOKEN="$TELEGRAM_BOT_TOKEN" DATA_DIR="./testdata" MODEL_DIR="./model" ./bot
```

Test in Telegram:
1. Send `/start` — should get welcome message
2. Send `/check деплой на прод` — should show work
3. Send `/check смачного!` — should show personal
4. Send a work message — should get roasted (if 95%+ confidence)
5. Send `/stats` — should show your stats
6. Send `/mute` then work message — should be silent
7. Send `/unmute` — tracking resumes

- [ ] **Step 7.3: Clean up test data**

Run:
```bash
rm -rf go-bot/testdata
```

- [ ] **Step 7.4: Commit any fixes**

```bash
git add -A go-bot/
git commit -m "fix: integration test fixes"
```

---

### Task 8: Deploy to Fly.io

- [ ] **Step 8.1: Update fly.toml to point to go-bot**

The existing `fly.toml` in the project root should work. Ensure `[build]` section doesn't specify a Dockerfile path, or update it:

```toml
[build]
  dockerfile = "go-bot/Dockerfile"
```

Or move/copy `fly.toml` into `go-bot/` and deploy from there.

- [ ] **Step 8.2: Deploy**

Run:
```bash
fly deploy --dockerfile go-bot/Dockerfile
```

Or from `go-bot/`:
```bash
cd go-bot && fly deploy -a fuck-work-bot
```

- [ ] **Step 8.3: Verify deployment**

Run:
```bash
fly status -a fuck-work-bot
fly logs -a fuck-work-bot
```

Expected: app running, logs show "Bot starting...", "Classifier loaded", "Storage initialized".

- [ ] **Step 8.4: Test live bot**

Same tests as Task 7 Step 7.2 but against the live bot.

- [ ] **Step 8.5: Commit final state**

```bash
git add -A
git commit -m "feat: deploy Go rewrite to Fly.io"
```
