package service

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/lshigami/Ringtails/config"
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

type GeminiLLMService interface {
	ScoreAndFeedbackAnswer(question *model.Question, userAnswer string) (feedback string, score float64, err error)
}

type geminiLLMService struct {
	client *genai.GenerativeModel
	cfg    *config.Config
}

func NewGeminiLLMService(cfg *config.Config) (GeminiLLMService, error) {
	if cfg.GeminiApiKey == "" {
		log.Warn().Msg("GEMINI_API_KEY is not set. GeminiLLMService will be non-functional.")
		// Return a service that indicates unavailability but doesn't crash
		return &geminiLLMService{cfg: cfg, client: nil}, nil
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiApiKey))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Gemini client")
		return nil, fmt.Errorf("failed to initialize Gemini client: %w", err)
	}
	// Ensure the model supports vision for sentence_picture type
	model := client.GenerativeModel("gemini-1.5-flash") // or "gemini-pro-vision" if preferred
	return &geminiLLMService{client: model, cfg: cfg}, nil
}

// fetchImageData downloads image data from a URL.
func fetchImageData(imageURL string) ([]byte, string, error) {
	if imageURL == "" {
		return nil, "", fmt.Errorf("image URL is empty")
	}
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch image from URL %s: %w", imageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to fetch image (status %d) from URL %s", resp.StatusCode, imageURL)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image data from URL %s: %w", imageURL, err)
	}

	contentType := resp.Header.Get("Content-Type")
	var mimeType string
	if contentType != "" {
		parsedMime, _, parseErr := mime.ParseMediaType(contentType)
		if parseErr == nil && strings.HasPrefix(parsedMime, "image/") {
			mimeType = parsedMime
		}
	}
	if mimeType == "" { // Fallback to extension
		ext := filepath.Ext(imageURL)
		mimeType = mime.TypeByExtension(ext)
		if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
			log.Warn().Str("url", imageURL).Str("ext", ext).Msg("Could not determine valid MIME type from extension or Content-Type.")
			// Gemini supports specific image types. If not determined, it might fail.
			// It's better to fail here if MIME type is crucial and unknown.
			return imageData, "", fmt.Errorf("unsupported or undeterminable image MIME type for %s", imageURL)
		}
	}
	// Validate against Gemini supported types
	supportedMIMETypes := map[string]bool{
		"image/png": true, "image/jpeg": true, "image/webp": true,
		"image/gif": true, "image/heic": true, "image/heif": true,
	}
	if !supportedMIMETypes[mimeType] {
		log.Warn().Str("mimeType", mimeType).Msg("MIME type determined but may not be supported by Gemini.")
		// We can still try, Gemini might handle it or return an error.
	}

	return imageData, mimeType, nil
}

func parseScoreAndFeedback(rawResponse string) (scoreStr string, feedbackStr string, err error) {
	scorePrefix := "Score:"
	feedbackPrefix := "Feedback:"

	scoreIndex := strings.Index(rawResponse, scorePrefix)
	feedbackIndex := strings.Index(rawResponse, feedbackPrefix)

	if scoreIndex == -1 { // Score prefix is mandatory
		return "", rawResponse, fmt.Errorf("response does not contain 'Score:' prefix. Raw: %s", rawResponse)
	}

	// Extract score value
	endOfScoreLine := strings.Index(rawResponse[scoreIndex:], "\n")
	if endOfScoreLine == -1 { // Score might be the last thing or on a single line
		scoreStr = strings.TrimSpace(rawResponse[scoreIndex+len(scorePrefix):])
	} else {
		scoreStr = strings.TrimSpace(rawResponse[scoreIndex+len(scorePrefix) : scoreIndex+endOfScoreLine])
	}

	// Extract feedback
	if feedbackIndex != -1 && feedbackIndex > scoreIndex {
		feedbackStr = strings.TrimSpace(rawResponse[feedbackIndex+len(feedbackPrefix):])
	} else {
		// If Feedback prefix is missing, or before Score, assume rest of the string after score line is feedback
		if endOfScoreLine != -1 && len(rawResponse) > scoreIndex+endOfScoreLine+1 {
			feedbackStr = strings.TrimSpace(rawResponse[scoreIndex+endOfScoreLine+1:])
		} else if endOfScoreLine == -1 && scoreStr != rawResponse[scoreIndex+len(scorePrefix):] {
			// This case is unlikely if score is well-formed and not the only thing.
			feedbackStr = "No specific feedback found after score."
		} else {
			feedbackStr = "Feedback not found in the expected format."
		}
	}
	return scoreStr, feedbackStr, nil
}

func (s *geminiLLMService) ScoreAndFeedbackAnswer(question *model.Question, userAnswer string) (string, float64, error) {
	if s.client == nil {
		return "AI Service is unavailable (client not initialized).", 0.0, fmt.Errorf("gemini client not initialized")
	}

	ctx := context.Background()
	var parts []genai.Part
	maxScore := question.MaxScore
	if maxScore <= 0 { // Fallback max score based on type if not set
		switch question.Type {
		case "sentence_picture":
			maxScore = 3.0
		case "email_response":
			maxScore = 4.0
		case "opinion_essay":
			maxScore = 5.0
		default:
			maxScore = 3.0 // Default for unknown types
		}
		log.Warn().Uint("questionID", question.ID).Float64("fallbackMaxScore", maxScore).Msg("Question MaxScore is invalid or not set, using type-based fallback.")
	}

	scoringInstruction := fmt.Sprintf(`
Provide your evaluation in two parts:
1. Score: A numerical score for the answer, from 0.0 to %.1f (e.g., 2.5, 3.0).
2. Feedback: Constructive feedback for the learner.

Format your response strictly as:
Score: [Your Numerical Score Here]
Feedback: [Your Detailed Feedback Here]
---
`, maxScore)

	var textPromptBuilder strings.Builder
	textPromptBuilder.WriteString("You are a TOEIC Writing examiner.\n")

	switch question.Type {
	case "sentence_picture":
		if question.ImageURL != nil && *question.ImageURL != "" {
			imageData, mimeType, err := fetchImageData(*question.ImageURL)
			if err != nil {
				log.Error().Err(err).Str("imageURL", *question.ImageURL).Msg("Failed to fetch image for scoring")
				return fmt.Sprintf("Error processing image: %s. Cannot score.", err.Error()), 0.0, err
			}
			parts = append(parts, genai.ImageData(mimeType, imageData))
			textPromptBuilder.WriteString("The user was shown the image provided above and ")
		} else {
			textPromptBuilder.WriteString("The user was supposed to be shown an image (but it was not provided to you) and ")
		}
		word1, word2 := "N/A", "N/A"
		if question.GivenWord1 != nil {
			word1 = *question.GivenWord1
		}
		if question.GivenWord2 != nil {
			word2 = *question.GivenWord2
		}
		textPromptBuilder.WriteString(fmt.Sprintf("given two words/phrases: \"%s\" and \"%s\".\n", word1, word2))
		textPromptBuilder.WriteString("They were asked to write ONE grammatically correct sentence that describes the picture using both given words/phrases.\n\n")
		textPromptBuilder.WriteString("Evaluation Criteria:\n")
		textPromptBuilder.WriteString("1. Relevance to the Picture: Does the sentence accurately describe an aspect of the provided image? (If no image, assume relevance if words are used contextually).\n")
		textPromptBuilder.WriteString("2. Grammar: Is the sentence grammatically correct?\n")
		textPromptBuilder.WriteString("3. Vocabulary: Are words used appropriately? Spelling errors?\n")
		textPromptBuilder.WriteString(fmt.Sprintf("4. Use of Given Words: Correct and natural incorporation of both \"%s\" and \"%s\".\n\n", word1, word2))

	case "email_response", "opinion_essay":
		textPromptBuilder.WriteString("Evaluate the user's answer based on the provided question/prompt.\n")
		textPromptBuilder.WriteString("Question/Prompt:\n---\n")
		textPromptBuilder.WriteString(question.Prompt)
		textPromptBuilder.WriteString("\n---\n")
		textPromptBuilder.WriteString("Evaluation Criteria:\n")
		textPromptBuilder.WriteString("1. Task Completion: Addresses all parts of the prompt effectively?\n")
		textPromptBuilder.WriteString("2. Grammar and Vocabulary: Accuracy, range, appropriateness.\n")
		textPromptBuilder.WriteString("3. Organization and Cohesion: Clarity, logical flow, paragraphing (if applicable), cohesive devices.\n\n")
	default:
		return "", 0.0, fmt.Errorf("unsupported question type for scoring: %s", question.Type)
	}

	textPromptBuilder.WriteString("User's Answer:\n---\n")
	textPromptBuilder.WriteString(userAnswer)
	textPromptBuilder.WriteString("\n---\n")
	textPromptBuilder.WriteString(scoringInstruction)

	parts = append(parts, genai.Text(textPromptBuilder.String()))

	resp, err := s.client.GenerateContent(ctx, parts...)
	if err != nil {
		log.Error().Err(err).Str("questionType", question.Type).Msg("Gemini API error during scoring")
		return fmt.Sprintf("Gemini API error: %s. Please try again.", err.Error()), 0.0, err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		log.Warn().Msg("Gemini returned no candidates or parts in response.")
		return "Gemini returned an empty or malformed response.", 0.0, fmt.Errorf("gemini returned no content")
	}

	fullResponseText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			fullResponseText += string(txt)
		}
	}

	if fullResponseText == "" {
		return "Gemini returned no text content.", 0.0, fmt.Errorf("gemini returned no text content")
	}

	scoreStr, feedbackStr, parseErr := parseScoreAndFeedback(fullResponseText)
	if parseErr != nil {
		log.Warn().Err(parseErr).Str("rawResponse", fullResponseText).Msg("Failed to parse score and feedback from Gemini response")
		// Return the full response as feedback if parsing fails, score 0
		return fmt.Sprintf("Could not parse AI response. Raw: %s", fullResponseText), 0.0, parseErr
	}

	parsedScore, scoreErr := strconv.ParseFloat(strings.TrimSpace(scoreStr), 64)
	if scoreErr != nil {
		log.Warn().Err(scoreErr).Str("scoreStr", scoreStr).Msg("Failed to parse score string to float")
		// Return the feedback part, score 0
		return feedbackStr, 0.0, fmt.Errorf("could not parse score value ('%s') from AI response. Feedback: %s", scoreStr, feedbackStr)
	}

	// Clamp score to the 0-maxScore range
	if parsedScore > maxScore {
		parsedScore = maxScore
	}
	if parsedScore < 0 {
		parsedScore = 0
	}

	return strings.TrimSpace(feedbackStr), parsedScore, nil
}
