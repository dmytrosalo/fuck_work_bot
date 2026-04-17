package classifier

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/options"
	"github.com/knights-analytics/hugot/pipelines"
)

// Result holds the classification output for a single message.
type Result struct {
	Label      string  // "work" or "personal"
	Confidence float64 // 0.0 to 1.0
	IsWork     bool
}

// logisticWeights holds the pre-trained logistic regression head.
type logisticWeights struct {
	Coef      []float64 `json:"coef"`
	Intercept float64   `json:"intercept"`
	Classes   []string  `json:"classes"`
}

// Classifier wraps the ONNX sentence embedding model and logistic regression head.
type Classifier struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
	weights  logisticWeights
}

// New creates a new Classifier from the model directory and weights file.
// modelDir should contain model.onnx and tokenizer.json.
// weightsPath points to the JSON logistic regression weights file.
// onnxLibPath is the directory containing the ONNX Runtime shared library (e.g. /opt/homebrew/lib).
// If empty, the default system path is used.
func New(modelDir, weightsPath, onnxLibPath string) (*Classifier, error) {
	// Load logistic regression weights.
	weightsData, err := os.ReadFile(weightsPath)
	if err != nil {
		return nil, fmt.Errorf("read weights: %w", err)
	}
	var weights logisticWeights
	if err := json.Unmarshal(weightsData, &weights); err != nil {
		return nil, fmt.Errorf("parse weights: %w", err)
	}
	if len(weights.Coef) == 0 {
		return nil, fmt.Errorf("weights file has empty coef array")
	}
	if len(weights.Classes) != 2 {
		return nil, fmt.Errorf("expected 2 classes, got %d", len(weights.Classes))
	}

	// Create ONNX Runtime session.
	var sessionOpts []options.WithOption
	if onnxLibPath != "" {
		sessionOpts = append(sessionOpts, options.WithOnnxLibraryPath(onnxLibPath))
	}
	session, err := hugot.NewORTSession(sessionOpts...)
	if err != nil {
		return nil, fmt.Errorf("create ORT session: %w", err)
	}

	// Create feature extraction pipeline with normalization (L2).
	config := hugot.FeatureExtractionConfig{
		ModelPath: modelDir,
		Name:      "sentence-embeddings",
		Options: []hugot.FeatureExtractionOption{
			pipelines.WithNormalization(),
		},
	}
	pipeline, err := hugot.NewPipeline(session, config)
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("create pipeline: %w", err)
	}

	return &Classifier{
		session:  session,
		pipeline: pipeline,
		weights:  weights,
	}, nil
}

// Classify predicts whether a message is work-related or personal.
func (c *Classifier) Classify(text string) (Result, error) {
	results, err := c.ClassifyBatch([]string{text})
	if err != nil {
		return Result{}, err
	}
	return results[0], nil
}

// ClassifyBatch predicts labels for multiple messages at once.
func (c *Classifier) ClassifyBatch(texts []string) ([]Result, error) {
	output, err := c.pipeline.RunPipeline(texts)
	if err != nil {
		return nil, fmt.Errorf("run pipeline: %w", err)
	}

	results := make([]Result, len(texts))
	for i, embedding := range output.Embeddings {
		results[i] = c.predict(embedding)
	}
	return results, nil
}

// predict applies the logistic regression head to a single embedding vector.
func (c *Classifier) predict(embedding []float32) Result {
	// dot(coef, embedding) + intercept
	logit := c.weights.Intercept
	for j := 0; j < len(c.weights.Coef) && j < len(embedding); j++ {
		logit += c.weights.Coef[j] * float64(embedding[j])
	}

	// sigmoid
	prob := 1.0 / (1.0 + math.Exp(-logit))

	// classes[0] = "personal", classes[1] = "work"
	// prob is the probability of class[1] ("work") in standard sklearn binary LR.
	// sklearn's logistic regression: coef and intercept correspond to the positive class (classes[1]).
	isWork := prob >= 0.5
	confidence := prob
	if !isWork {
		confidence = 1.0 - prob
	}

	label := c.weights.Classes[0] // "personal"
	if isWork {
		label = c.weights.Classes[1] // "work"
	}

	return Result{
		Label:      label,
		Confidence: confidence,
		IsWork:     isWork,
	}
}

// Close releases all resources held by the classifier.
func (c *Classifier) Close() error {
	if c.session != nil {
		return c.session.Destroy()
	}
	return nil
}
