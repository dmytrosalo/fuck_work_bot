package classifier

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode"
)

// Result holds the classification output for a single message.
type Result struct {
	Label      string  // "work" or "personal"
	Confidence float64 // 0.0 to 1.0
	IsWork     bool
}

// tfidfModel holds the exported scikit-learn TF-IDF + LogReg model.
type tfidfModel struct {
	Vocabulary map[string]int `json:"vocabulary"`
	IDF        []float64      `json:"idf"`
	Config     struct {
		MaxFeatures int    `json:"max_features"`
		NgramRange  []int  `json:"ngram_range"`
		SublinearTF bool   `json:"sublinear_tf"`
		Norm        string `json:"norm"`
	} `json:"config"`
	Classifier struct {
		Coef      []float64 `json:"coef"`
		Intercept float64   `json:"intercept"`
	} `json:"classifier"`
}

// Classifier wraps the TF-IDF + LogReg model.
type Classifier struct {
	model tfidfModel
}

// New loads the TF-IDF model from a JSON file.
func New(modelPath string) (*Classifier, error) {
	data, err := os.ReadFile(modelPath)
	if err != nil {
		return nil, fmt.Errorf("read model: %w", err)
	}

	var model tfidfModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, fmt.Errorf("parse model: %w", err)
	}

	if len(model.Vocabulary) == 0 {
		return nil, fmt.Errorf("empty vocabulary")
	}

	return &Classifier{model: model}, nil
}

// Close is a no-op for TF-IDF (no resources to release).
func (c *Classifier) Close() error {
	return nil
}

// Classify predicts whether a message is work-related or personal.
func (c *Classifier) Classify(text string) (Result, error) {
	if text == "" {
		return Result{Label: "personal", Confidence: 1.0, IsWork: false}, nil
	}

	// Tokenize into n-grams
	tokens := tokenize(text)
	ngrams := generateNgrams(tokens, c.model.Config.NgramRange[0], c.model.Config.NgramRange[1])

	// Build TF-IDF vector (sparse — only compute for terms in vocabulary)
	tf := make(map[int]float64)
	for _, ng := range ngrams {
		if idx, ok := c.model.Vocabulary[ng]; ok {
			tf[idx]++
		}
	}

	// Apply sublinear TF: tf = 1 + log(tf)
	if c.model.Config.SublinearTF {
		for idx, count := range tf {
			if count > 0 {
				tf[idx] = 1.0 + math.Log(count)
			}
		}
	}

	// Multiply by IDF
	tfidf := make(map[int]float64)
	for idx, tfVal := range tf {
		tfidf[idx] = tfVal * c.model.IDF[idx]
	}

	// L2 normalize
	if c.model.Config.Norm == "l2" {
		var norm float64
		for _, v := range tfidf {
			norm += v * v
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			for idx := range tfidf {
				tfidf[idx] /= norm
			}
		}
	}

	// Logistic regression: dot(coef, tfidf) + intercept
	logit := c.model.Classifier.Intercept
	for idx, val := range tfidf {
		logit += c.model.Classifier.Coef[idx] * val
	}

	// Add keyword boost
	logit += keywordBoost(text)

	// Sigmoid
	prob := 1.0 / (1.0 + math.Exp(-logit))

	isWork := prob >= 0.5
	confidence := prob
	if !isWork {
		confidence = 1.0 - prob
	}

	label := "personal"
	if isWork {
		label = "work"
	}

	return Result{Label: label, Confidence: confidence, IsWork: isWork}, nil
}

// tokenize splits text into lowercase words.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// generateNgrams creates n-grams from tokens.
func generateNgrams(tokens []string, minN, maxN int) []string {
	var ngrams []string
	for n := minN; n <= maxN; n++ {
		for i := 0; i <= len(tokens)-n; i++ {
			ngram := strings.Join(tokens[i:i+n], " ")
			ngrams = append(ngrams, ngram)
		}
	}
	return ngrams
}

// Work keywords for boosting.
var workKeywords = []string{
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
}

func keywordBoost(text string) float64 {
	lower := strings.ToLower(text)
	hits := 0
	for _, kw := range workKeywords {
		if strings.Contains(lower, kw) {
			hits++
		}
	}
	boost := float64(hits) * 0.3
	if boost > 1.5 {
		boost = 1.5
	}
	return boost
}
