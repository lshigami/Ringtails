package service

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath" // Để lấy MIME type từ phần mở rộng file nếu cần

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
	// Sử dụng model có khả năng vision, ví dụ gemini-1.5-flash hoặc gemini-pro-vision
	// gemini-1.5-flash là một lựa chọn tốt vì nó nhanh và hỗ trợ multimodal.
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.GeminiApiKey))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Gemini client")
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	model := client.GenerativeModel("gemini-1.5-flash") // Đảm bảo model này hỗ trợ vision
	return &geminiService{client: model, cfg: cfg}, nil
}

// fetchImageData tải dữ liệu hình ảnh từ URL và xác định MIME type.
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
		return nil, "", fmt.Errorf("failed to fetch image: status code %d from URL %s", resp.StatusCode, imageURL)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read image data from URL %s: %w", imageURL, err)
	}

	// Cố gắng lấy MIME type từ header Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil && (mediaType == "image/jpeg" || mediaType == "image/png" || mediaType == "image/webp" || mediaType == "image/gif" || mediaType == "image/heic" || mediaType == "image/heif") {
			return imageData, mediaType, nil
		}
	}

	// Nếu không có Content-Type hoặc không hợp lệ, thử đoán từ phần mở rộng file của URL
	ext := filepath.Ext(imageURL)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		// Mặc định hoặc dựa vào phân tích byte đầu tiên (phức tạp hơn, bỏ qua ở đây)
		// Gemini hỗ trợ các định dạng phổ biến, nếu không chắc chắn, PNG hoặc JPEG thường an toàn.
		// Tuy nhiên, cung cấp MIME type chính xác là tốt nhất.
		log.Warn().Str("url", imageURL).Msg("Could not determine MIME type from URL extension or Content-Type header. Attempting to send without specific MIME or assuming common type if API allows.")
		// Gemini có thể tự phát hiện một số loại, nhưng tốt hơn là cung cấp.
		// Nếu API yêu cầu MIME, đây có thể là lỗi.
		// Chúng ta có thể thử một MIME type phổ biến nếu không chắc chắn, ví dụ "image/jpeg"
		// return imageData, "image/jpeg", nil
		return imageData, "", fmt.Errorf("could not determine a supported MIME type for image %s", imageURL)
	}

	return imageData, mimeType, nil
}

func (s *geminiService) GetFeedbackForAttempt(question *model.Question, userAnswer string) (string, error) {
	if s.client == nil {
		log.Warn().Msg("Gemini client not initialized (API key likely missing). Returning placeholder feedback.")
		return "AI Feedback is currently unavailable due to configuration issue.", nil
	}

	ctx := context.Background()
	var parts []genai.Part

	switch question.Type {
	case "sentence_picture":
		if question.ImageURL == nil || *question.ImageURL == "" {
			log.Warn().Uint("questionID", question.ID).Msg("ImageURL is missing for sentence_picture question type.")
			return "Cannot provide feedback: Image is missing for this question.", nil
		}

		imageData, mimeType, err := fetchImageData(*question.ImageURL)
		if err != nil {
			log.Error().Err(err).Str("imageURL", *question.ImageURL).Msg("Failed to fetch or prepare image data for Gemini.")
			return fmt.Sprintf("Error processing image for feedback: %s. Please check if the image URL is valid and accessible.", err.Error()), nil
		}
		if mimeType != "image/png" && mimeType != "image/jpeg" && mimeType != "image/webp" && mimeType != "image/gif" && mimeType != "image/heic" && mimeType != "image/heif" {
			log.Warn().Str("mimeType", mimeType).Str("url", *question.ImageURL).Msg("Unsupported MIME type for Gemini Vision. Common types are image/png, image/jpeg.")
			// Gemini có thể vẫn xử lý được một số, nhưng đây là cảnh báo.
			// Nếu API trả lỗi, đây là lý do.
			// return fmt.Sprintf("Unsupported image type: %s. Please use PNG, JPEG, WEBP, GIF, HEIC or HEIF.", mimeType), nil
		}

		word1 := "N/A"
		if question.GivenWord1 != nil {
			word1 = *question.GivenWord1
		}
		word2 := "N/A"
		if question.GivenWord2 != nil {
			word2 = *question.GivenWord2
		}

		// Thêm dữ liệu hình ảnh vào prompt
		parts = append(parts, genai.ImageData(mimeType, imageData)) // Quan trọng: mimeType phải đúng!

		// Thêm phần văn bản của prompt
		textPrompt := fmt.Sprintf(`You are a TOEIC Writing examiner.
The user was shown the image provided above and given two words/phrases: "%s" and "%s".
They were asked to write ONE grammatically correct sentence that describes the picture using both given words/phrases.

Evaluate the user's sentence based on:
1. Relevance to the Picture: Does the sentence accurately describe an aspect of the provided image?
2. Grammar: Is the sentence grammatically correct?
3. Vocabulary: Are the words used appropriately? Are there any spelling errors?
4. Use of Given Words: Did the user correctly and naturally incorporate both "%s" and "%s" into the sentence in a way that relates to the picture?

Provide constructive feedback. Point out specific errors with corrections. Be concise.

Given words/phrases:
Word 1: %s
Word 2: %s

User's Sentence:
---
%s
---

Feedback:
`, word1, word2, word1, word2, word1, word2, userAnswer)
		parts = append(parts, genai.Text(textPrompt))

	case "email_response", "opinion_essay":
		textPrompt := fmt.Sprintf(`You are a TOEIC Writing examiner.
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
		parts = append(parts, genai.Text(textPrompt))
	default:
		return "", fmt.Errorf("unsupported question type for AI feedback: %s", question.Type)
	}

	// Generate content using parts (which can be mixed image and text)
	resp, err := s.client.GenerateContent(ctx, parts...)
	if err != nil {
		log.Error().Err(err).Str("question_type", question.Type).Msg("Error generating content from Gemini")
		// Check for specific Gemini errors related to image processing if possible
		return fmt.Sprintf("Gemini API error: %s. (If this is an image question, ensure the image URL is public and the format is supported like PNG or JPEG).", err.Error()), nil
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		log.Warn().Interface("geminiResponse", resp).Msg("Gemini response was empty or malformed")
		return "", fmt.Errorf("gemini returned no content or malformed response")
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
