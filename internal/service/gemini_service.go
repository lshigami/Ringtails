package service

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"github.com/lshigami/Ringtails/config"
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

type GeminiService interface {
	GetFeedbackForAttempt(question *model.Question, userAnswer string) (string, error)
}

type geminiService struct {
	client *genai.GenerativeModel
	cfg    *config.Config
}

func NewGeminiService(cfg *config.Config) (GeminiService, error) {
	if cfg.GeminiApiKey == "" {
		log.Warn().Msg("GEMINI_API_KEY is not set. GeminiService will not function.")
		return &geminiService{cfg: cfg, client: nil}, nil
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiApiKey))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Gemini client")
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	model := client.GenerativeModel("gemini-2.5-pro-preview-05-06")
	return &geminiService{client: model, cfg: cfg}, nil
}

func (s *geminiService) GetFeedbackForAttempt(question *model.Question, userAnswer string) (string, error) {
	if s.client == nil {
		log.Warn().Msg("Gemini client not initialized (API key likely missing). Returning placeholder feedback.")
		return "AI Feedback is currently unavailable.", nil
	}

	var prompt string
	switch question.Type {
	case "sentence_picture":
		word1 := "N/A"
		if question.GivenWord1 != nil {
			word1 = *question.GivenWord1
		}
		word2 := "N/A"
		if question.GivenWord2 != nil {
			word2 = *question.GivenWord2
		}
		prompt = fmt.Sprintf(`You are a TOEIC Writing examiner.
The user was shown a picture (which you cannot see, but its URL might be %s if relevant for context, though you should not attempt to access it) and given two words/phrases: "%s" and "%s".
They were asked to write ONE grammatically correct sentence that describes the picture using both given words/phrases.

Evaluate the user's sentence based on:
1. Grammar: Is the sentence grammatically correct?
2. Vocabulary: Are the words used appropriately? Are there any spelling errors?
3. Use of Given Words: Did the user correctly and naturally incorporate both "%s" and "%s"?
4. Coherence (assumed): Assume the sentence makes sense in the context of some picture if it uses the words correctly.

Provide constructive feedback. Point out specific errors with corrections. Be concise.

Given words/phrases:
Word 1: %s
Word 2: %s

User's Sentence:
---
%s
---

Feedback:
`, formatOptionalString(question.ImageURL), word1, word2, word1, word2, word1, word2, userAnswer)

	case "email_response", "opinion_essay":
		prompt = fmt.Sprintf(`You are a TOEIC Writing examiner.
Please evaluate the following user's answer based on the provided question/prompt.
Focus on:
1. Task Completion: Did the user address all parts of the question/prompt effectively?
2. Grammar and Vocabulary: Accuracy, range, and appropriateness. Check for errors.
3. Organization and Cohesion: Clarity, logical flow, use of paragraphs (if applicable), and cohesive devices.

Provide constructive feedback, point out specific errors with corrections if possible, and suggest areas for improvement.
Keep the feedback concise and helpful for a TOEIC learner.

Question/Prompt:
---
%s
---

User's Answer:
---
%s
---

Feedback:
`, question.Prompt, userAnswer)
	default:
		return "", fmt.Errorf("unsupported question type for AI feedback: %s", question.Type)
	}

	ctx := context.Background()
	resp, err := s.client.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		log.Error().Err(err).Str("question_type", question.Type).Msg("Error generating content from Gemini")
		return "", fmt.Errorf("gemini API error: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		log.Warn().Msg("Gemini response was empty or malformed")
		return "", fmt.Errorf("gemini returned no content")
	}

	feedback := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			feedback += string(txt)
		}
	}
	if feedback == "" {
		return "Received an empty feedback from AI.", nil
	}
	return feedback, nil
}

// Helper to safely dereference string pointers for formatting
func formatOptionalString(s *string) string {
	if s == nil {
		return "N/A"
	}
	return *s
}
