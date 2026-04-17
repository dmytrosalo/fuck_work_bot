package classifier

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	modelDirRel   = "../../model"
	weightsRel    = "../../model/weights.json"
	onnxLibMacOS  = "/opt/homebrew/lib"
)

func onnxLibPath() string {
	if p := os.Getenv("ONNX_RUNTIME_LIB"); p != "" {
		return p
	}
	if _, err := os.Stat(filepath.Join(onnxLibMacOS, "libonnxruntime.dylib")); err == nil {
		return onnxLibMacOS
	}
	return "" // let hugot find it
}

func modelDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(modelDirRel)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "model.onnx")); os.IsNotExist(err) {
		t.Skip("model files not present, skipping integration test")
	}
	return dir
}

func weightsPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(weightsRel)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Skip("weights.json not present, skipping integration test")
	}
	return p
}

func newTestClassifier(t *testing.T) *Classifier {
	t.Helper()
	c, err := New(modelDir(t), weightsPath(t), onnxLibPath())
	if err != nil {
		t.Fatalf("failed to create classifier: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("close classifier: %v", err)
		}
	})
	return c
}

func TestClassifierLoads(t *testing.T) {
	_ = newTestClassifier(t)
}

func TestClassifyWork(t *testing.T) {
	c := newTestClassifier(t)

	workMessages := []string{
		"нрф планінг щотижневий",
		"деплой на прод зробили",
		"маріт звільняється?",
		"зараз ще один созвон буде",
		"делна підараска?",
	}

	for _, msg := range workMessages {
		t.Run(msg, func(t *testing.T) {
			result, err := c.Classify(msg)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			t.Logf("msg=%q label=%s confidence=%.4f", msg, result.Label, result.Confidence)
			if !result.IsWork {
				t.Errorf("expected work, got %s (confidence=%.4f)", result.Label, result.Confidence)
			}
		})
	}
}

func TestClassifyPersonal(t *testing.T) {
	c := newTestClassifier(t)

	personalMessages := []string{
		"потім душ",
		"смачного!",
		"Завтра останній день відпустки",
		"я їду до мами сьогодні",
		"п'ятниця нарешті",
	}

	for _, msg := range personalMessages {
		t.Run(msg, func(t *testing.T) {
			result, err := c.Classify(msg)
			if err != nil {
				t.Fatalf("classify error: %v", err)
			}
			t.Logf("msg=%q label=%s confidence=%.4f", msg, result.Label, result.Confidence)
			if result.IsWork {
				t.Errorf("expected personal, got %s (confidence=%.4f)", result.Label, result.Confidence)
			}
		})
	}
}
