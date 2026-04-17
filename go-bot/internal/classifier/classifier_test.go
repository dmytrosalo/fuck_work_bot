package classifier

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestClassifier(t *testing.T) *Classifier {
	t.Helper()
	modelPath, err := filepath.Abs("../../model/tfidf_model.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skip("tfidf_model.json not present")
	}
	c, err := New(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestClassifyWork(t *testing.T) {
	c := newTestClassifier(t)
	for _, msg := range []string{"нрф планінг щотижневий", "деплой на прод", "делна підараска?"} {
		result, err := c.Classify(msg)
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsWork {
			t.Errorf("expected work for %q, got %s (%.0f%%)", msg, result.Label, result.Confidence*100)
		}
	}
}

func TestClassifyPersonal(t *testing.T) {
	c := newTestClassifier(t)
	for _, msg := range []string{"смачного!", "п'ятниця нарешті", "я їду до мами"} {
		result, err := c.Classify(msg)
		if err != nil {
			t.Fatal(err)
		}
		if result.IsWork {
			t.Errorf("expected personal for %q, got %s (%.0f%%)", msg, result.Label, result.Confidence*100)
		}
	}
}
