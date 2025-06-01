package service

import (
	"fmt"
	"math" // For rounding
)

// MaxRawScorePossibleForTOEICWriting là tổng điểm thô tối đa có thể đạt được.
// Q1-5: 5 * 3 = 15
// Q6-7: 2 * 4 = 8
// Q8:   1 * 5 = 5
// Total = 28
const MaxRawScorePossibleForTOEICWriting float64 = 28.0
const MaxScaledScoreForTOEICWriting float64 = 200.0

type ScoreConverterService interface {
	ConvertToScaledScore(rawScore float64) (float64, error)
}

type scoreConverterServiceImpl struct {
	// Có thể có các cấu hình cho bảng quy đổi ở đây nếu phức tạp
}

func NewScoreConverterService() ScoreConverterService {
	return &scoreConverterServiceImpl{}
}

// ConvertToScaledScore quy đổi điểm thô sang thang điểm 200.
// Đây là một ví dụ quy đổi TUYẾN TÍNH ĐƠN GIẢN.
// BẠN CẦN THAY THẾ BẰNG BẢNG QUY ĐỔI CHÍNH THỨC HOẶC GẦN ĐÚNG CỦA TOEIC.
func (s *scoreConverterServiceImpl) ConvertToScaledScore(rawScore float64) (float64, error) {
	if rawScore < 0 || rawScore > MaxRawScorePossibleForTOEICWriting {
		return 0, fmt.Errorf("raw score %.2f is out of valid range (0-%.2f)", rawScore, MaxRawScorePossibleForTOEICWriting)
	}

	// Logic quy đổi tuyến tính đơn giản (CẦN THAY THẾ BẰNG BẢNG CHUẨN)
	// Ví dụ: scaledScore = (rawScore / maxRaw) * 200
	//scaledScore := (rawScore / MaxRawScorePossibleForTOEICWriting) * MaxScaledScoreForTOEICWriting

	// Ví dụ một bảng quy đổi dạng bậc thang (lookup table) - CẦN ĐIỀN BẢNG CHÍNH XÁC
	// Đây chỉ là cấu trúc ví dụ, các khoảng điểm và giá trị cần được nghiên cứu kỹ.
	var scaledScore float64
	switch {
	case rawScore <= 0:
		scaledScore = 0
	case rawScore <= 3:
		scaledScore = rawScore * 10 // Ví dụ: 0-30
	case rawScore <= 5:
		scaledScore = 30 + (rawScore-3)*8 // Ví dụ: 30-46
	case rawScore <= 8:
		scaledScore = 46 + (rawScore-5)*7 // Ví dụ: 46-67
	case rawScore <= 11:
		scaledScore = 67 + (rawScore-8)*6 // Ví dụ: 67-85
	case rawScore <= 14:
		scaledScore = 85 + (rawScore-11)*5 // Ví dụ: 85-100
	case rawScore <= 17:
		scaledScore = 100 + (rawScore-14)*6 // Ví dụ: 100-118
	case rawScore <= 20:
		scaledScore = 118 + (rawScore-17)*7 // Ví dụ: 118-139
	case rawScore <= 23:
		scaledScore = 139 + (rawScore-20)*8 // Ví dụ: 139-163
	case rawScore <= 25:
		scaledScore = 163 + (rawScore-23)*9 // Ví dụ: 163-181
	case rawScore <= 27:
		scaledScore = 181 + (rawScore-25)*8 // Ví dụ: 181-197 (cần điều chỉnh để không vượt 200)
	case rawScore >= MaxRawScorePossibleForTOEICWriting:
		scaledScore = MaxScaledScoreForTOEICWriting // Max score
	default: // Should cover all cases up to MaxRawScorePossible
		// Apply a linear scaling for the last segment or adjust ranges above
		// For scores very close to max, ensure it maps correctly
		if rawScore > 27 && rawScore < MaxRawScorePossibleForTOEICWriting {
			scaledScore = 197 + (rawScore-27)*((MaxScaledScoreForTOEICWriting-197)/(MaxRawScorePossibleForTOEICWriting-27))
		} else {
			// Default fallback or error if a range is missed
			scaledScore = MaxScaledScoreForTOEICWriting * (rawScore / MaxRawScorePossibleForTOEICWriting)
		}
	}

	// Đảm bảo điểm không vượt quá 200 và không nhỏ hơn 0
	if scaledScore > MaxScaledScoreForTOEICWriting {
		scaledScore = MaxScaledScoreForTOEICWriting
	}
	if scaledScore < 0 {
		scaledScore = 0
	}

	// Làm tròn đến 0 hoặc 5 (thường điểm TOEIC làm tròn đến 5, ví dụ 150, 155, 160)
	// Hoặc chỉ làm tròn đến số nguyên gần nhất
	// return math.Round(scaledScore/5.0) * 5.0 // Làm tròn đến bội số của 5
	return math.Round(scaledScore), nil // Làm tròn đến số nguyên gần nhất
}
