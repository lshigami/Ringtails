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

// GeminiLLMService interface (giữ nguyên)
type GeminiLLMService interface {
	ScoreAndFeedbackAnswer(question *model.Question, userAnswer string) (feedback string, score float64, err error)
}

type geminiLLMService struct {
	client *genai.GenerativeModel
	cfg    *config.Config
}

// NewGeminiLLMService (giữ nguyên)
func NewGeminiLLMService(cfg *config.Config) (GeminiLLMService, error) {
	if cfg.GeminiApiKey == "" {
		log.Warn().Msg("GEMINI_API_KEY is not set. GeminiLLMService will be non-functional.")
		return &geminiLLMService{cfg: cfg, client: nil}, nil
	}
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiApiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Gemini client: %w", err)
	}
	model := client.GenerativeModel("gemini-1.5-flash") // Or your preferred vision model
	return &geminiLLMService{client: model, cfg: cfg}, nil
}

// fetchImageData (giữ nguyên từ phiên bản trước)
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
	if mimeType == "" {
		ext := filepath.Ext(imageURL)
		mimeType = mime.TypeByExtension(ext)
		if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
			log.Warn().Str("url", imageURL).Str("ext", ext).Msg("Could not determine valid MIME type from extension or Content-Type.")
			return imageData, "", fmt.Errorf("unsupported or undeterminable image MIME type for %s", imageURL)
		}
	}
	supportedMIMETypes := map[string]bool{
		"image/png": true, "image/jpeg": true, "image/webp": true,
		"image/gif": true, "image/heic": true, "image/heif": true,
	}
	if !supportedMIMETypes[mimeType] {
		log.Warn().Str("mimeType", mimeType).Msg("MIME type determined but may not be supported by Gemini.")
	}
	return imageData, mimeType, nil
}

// parseScoreAndFeedback (giữ nguyên từ phiên bản trước, nhưng có thể cần điều chỉnh nếu format output của LLM thay đổi)
func parseScoreAndFeedback(rawResponse string) (scoreStr string, feedbackStr string, err error) {
	scorePrefix := "Score:"
	feedbackPrefix := "Feedback:"

	scoreIndex := strings.Index(rawResponse, scorePrefix)
	feedbackIndex := strings.Index(rawResponse, feedbackPrefix)

	if scoreIndex == -1 {
		return "", rawResponse, fmt.Errorf("response does not contain 'Score:' prefix. Raw: %s", rawResponse)
	}

	endOfScoreLine := strings.Index(rawResponse[scoreIndex:], "\n")
	if endOfScoreLine == -1 {
		scoreStr = strings.TrimSpace(rawResponse[scoreIndex+len(scorePrefix):])
	} else {
		scoreStr = strings.TrimSpace(rawResponse[scoreIndex+len(scorePrefix) : scoreIndex+endOfScoreLine])
	}

	if feedbackIndex != -1 && feedbackIndex > scoreIndex {
		feedbackStr = strings.TrimSpace(rawResponse[feedbackIndex+len(feedbackPrefix):])
	} else {
		if endOfScoreLine != -1 && len(rawResponse) > scoreIndex+endOfScoreLine+1 {
			feedbackStr = strings.TrimSpace(rawResponse[scoreIndex+endOfScoreLine+1:])
			if strings.HasPrefix(strings.ToLower(feedbackStr), "feedback:") { // Handle case "Feedback: Feedback: ..."
				feedbackStr = strings.TrimSpace(feedbackStr[len(feedbackPrefix):])
			}
		} else {
			feedbackStr = "Feedback not found in the expected format after the score."
		}
	}
	// Ensure scoreStr only contains the number
	parts := strings.Fields(scoreStr)
	if len(parts) > 0 {
		scoreStr = parts[0] // Take the first part assuming it's the number
	}

	return scoreStr, feedbackStr, nil
}

// ScoreAndFeedbackAnswer cập nhật với prompt chi tiết hơn
func (s *geminiLLMService) ScoreAndFeedbackAnswer(question *model.Question, userAnswer string) (string, float64, error) {
	if s.client == nil {
		return "AI Service is unavailable (client not initialized).", 0.0, fmt.Errorf("gemini client not initialized")
	}

	ctx := context.Background()
	var parts []genai.Part
	maxScore := question.MaxScore
	if maxScore <= 0 {
		switch question.Type {
		case "sentence_picture":
			maxScore = 3.0
		case "email_response":
			maxScore = 4.0
		case "opinion_essay":
			maxScore = 5.0
		default:
			maxScore = 3.0
		}
		log.Warn().Uint("questionID", question.ID).Float64("fallbackMaxScore", maxScore).Msg("Question MaxScore is invalid or not set, using type-based fallback.")
	}

	// Hướng dẫn chung cho format output của LLM
	outputFormatInstruction := fmt.Sprintf(`
Please provide your evaluation in two distinct parts:
1. Score: A numerical score for the answer, from 0.0 to %.1f (e.g., 2.5, 3.0). The score should reflect the overall quality based on all criteria.
2. Feedback: Detailed, constructive feedback. Specifically:
    - Identify strong points of the response.
    - Point out specific errors in grammar, vocabulary, coherence, or task achievement.
    - For each error, explain briefly why it's an error.
    - Provide a concrete example of how to correct the error or improve the sentence/section.
    - If appropriate, suggest alternative phrasing or vocabulary.
    - Offer general advice for improvement related to the identified weaknesses.

Format your response strictly as:
Score: [Your Numerical Score Here]
Feedback:
[Your Detailed Feedback Here, using bullet points or clear paragraphs for different aspects]
---
`, maxScore)

	var textPromptBuilder strings.Builder
	textPromptBuilder.WriteString("You are an expert TOEIC Writing Test instructor with deep knowledge of the TOEIC Writing Test format and scoring criteria.\n")
	textPromptBuilder.WriteString("Please evaluate the following user's TOEIC writing response.\n\n")

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
		textPromptBuilder.WriteString("Task Prompt (for context):\n")
		textPromptBuilder.WriteString(question.Prompt) // Thêm prompt của câu hỏi (thường là "Write ONE sentence...")
		textPromptBuilder.WriteString("\n\nEvaluate the user's sentence based on the following TOEIC scoring criteria:\n")
		textPromptBuilder.WriteString("- Grammar: Accuracy of the grammatical structure used to form a single, complete sentence.\n")
		textPromptBuilder.WriteString("- Vocabulary: Appropriate and accurate use of the given words/phrases and any other vocabulary within the sentence.\n")
		textPromptBuilder.WriteString("- Relevance to Picture: The sentence must describe the provided picture and incorporate both given words/phrases meaningfully in relation to the picture.\n")
		textPromptBuilder.WriteString("- Task Achievement: Successfully writes ONE sentence that uses both given words/phrases.\n\n")

	case "email_response":
		textPromptBuilder.WriteString("The user was asked to respond to the following email prompt.\n")
		textPromptBuilder.WriteString("Email Prompt (Task):\n---\n")
		textPromptBuilder.WriteString(question.Prompt)
		textPromptBuilder.WriteString("\n---\n\n")
		textPromptBuilder.WriteString("Evaluate the user's email response based on the following TOEIC scoring criteria:\n")
		textPromptBuilder.WriteString("- Grammar: Accuracy and variety of grammatical structures.\n")
		textPromptBuilder.WriteString("- Vocabulary: Appropriateness, variety, and accuracy of word choice for an email.\n")
		textPromptBuilder.WriteString("- Coherence and Cohesion: Logical organization of ideas, clear flow, and effective use of linking words and phrases suitable for an email.\n")
		textPromptBuilder.WriteString("- Task Achievement: How well the response addresses all parts of the email prompt (e.g., answering questions, making requests as instructed), maintains appropriate tone, and follows email conventions.\n")
		textPromptBuilder.WriteString("- Relevance and Appropriateness: Suitability of tone (e.g., formal, semi-formal) and content for the email context.\n\n")

	case "opinion_essay":
		textPromptBuilder.WriteString("The user was asked to write an opinion essay based on the following prompt.\n")
		textPromptBuilder.WriteString("Essay Prompt (Task):\n---\n")
		textPromptBuilder.WriteString(question.Prompt)
		textPromptBuilder.WriteString("\n---\n\n")
		textPromptBuilder.WriteString("Evaluate the user's opinion essay based on the following TOEIC scoring criteria:\n")
		textPromptBuilder.WriteString("- Grammar: Accuracy and variety of grammatical structures.\n")
		textPromptBuilder.WriteString("- Vocabulary: Appropriateness, range, and accuracy of academic/formal word choice.\n")
		textPromptBuilder.WriteString("- Coherence and Cohesion: Clear thesis statement, logical organization of supporting paragraphs, smooth transitions, and effective use of cohesive devices.\n")
		textPromptBuilder.WriteString("- Task Achievement: How well the essay develops and supports an opinion in response to the prompt, provides relevant reasons and examples, and meets typical essay structure (introduction, body paragraphs, conclusion).\n")
		textPromptBuilder.WriteString("- Relevance and Appropriateness: The arguments are relevant to the prompt and the language is appropriate for an opinion essay.\n\n")
	default:
		return "", 0.0, fmt.Errorf("unsupported question type for scoring: %s", question.Type)
	}

	textPromptBuilder.WriteString("User's Answer:\n---\n")
	textPromptBuilder.WriteString(userAnswer)
	textPromptBuilder.WriteString("\n---\n\n")
	textPromptBuilder.WriteString(outputFormatInstruction) // Hướng dẫn format output

	parts = append(parts, genai.Text(textPromptBuilder.String()))

	// Call Gemini API
	resp, err := s.client.GenerateContent(ctx, parts...)
	if err != nil {
		log.Error().Err(err).Str("questionType", question.Type).Msg("Gemini API error during scoring")
		return fmt.Sprintf("Gemini API error: %s. Please try again.", err.Error()), 0.0, err
	}

	// Parse response
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
		return fmt.Sprintf("Could not parse AI response. Raw: %s", fullResponseText), 0.0, parseErr
	}

	parsedScore, scoreErr := strconv.ParseFloat(strings.TrimSpace(scoreStr), 64)
	if scoreErr != nil {
		log.Warn().Err(scoreErr).Str("scoreStr", scoreStr).Str("feedback", feedbackStr).Msg("Failed to parse score string to float")
		return feedbackStr, 0.0, fmt.Errorf("could not parse score value ('%s') from AI response. Feedback provided: %s", scoreStr, feedbackStr)
	}

	// Clamp score
	if parsedScore > maxScore {
		parsedScore = maxScore
	}
	if parsedScore < 0 {
		parsedScore = 0
	}

	return strings.TrimSpace(feedbackStr), parsedScore, nil
}
