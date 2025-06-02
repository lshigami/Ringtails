package service

import (
	"fmt"
	"math"

	"github.com/rs/zerolog/log" // Thêm log để debug
)

const MaxRawScorePossibleForTOEICWriting float64 = 28.0
const MaxScaledScoreForTOEICWriting float64 = 200.0

type ScoreConverterService interface {
	ConvertToScaledScore(rawScore float64) (float64, error)
}

type scoreConverterServiceImpl struct{}

func NewScoreConverterService() ScoreConverterService {
	return &scoreConverterServiceImpl{}
}

func (s *scoreConverterServiceImpl) ConvertToScaledScore(rawScore float64) (float64, error) {
	log.Debug().Float64("inputRawScore", rawScore).Msg("ScoreConverterService: Converting raw score") // Log đầu vào

	if rawScore < 0 || rawScore > MaxRawScorePossibleForTOEICWriting {
		log.Error().Float64("rawScore", rawScore).Msg("ScoreConverterService: Raw score out of valid range")
		return 0, fmt.Errorf("raw score %.2f is out of valid range (0-%.2f)", rawScore, MaxRawScorePossibleForTOEICWriting)
	}

	// ---- BẮT ĐẦU LOGIC QUY ĐỔI - ĐÂY LÀ NƠI CẦN XEM XÉT KỸ ----
	// Logic tuyến tính đơn giản để làm ví dụ debug (NÊN THAY BẰNG BẢNG CHUẨN)
	if MaxRawScorePossibleForTOEICWriting == 0 { // Tránh chia cho 0
		log.Error().Msg("ScoreConverterService: MaxRawScorePossibleForTOEICWriting is zero, cannot perform linear scaling.")
		return 0, fmt.Errorf("max raw score configuration error")
	}
	scaledScoreLinear := (rawScore / MaxRawScorePossibleForTOEICWriting) * MaxScaledScoreForTOEICWriting
	log.Debug().Float64("rawScore", rawScore).Float64("linearlyScaledScore", scaledScoreLinear).Msg("ScoreConverterService: Linearly scaled score (for reference)")

	// Bảng quy đổi ví dụ (CẦN THAY THẾ BẰNG BẢNG QUY ĐỔI CHÍNH XÁC CỦA TOEIC WRITING)
	// Đây là một ví dụ được cải thiện hơn một chút so với trước, nhưng vẫn cần bảng chuẩn.
	var scaledScore float64
	switch {
	case rawScore == 0:
		scaledScore = 0
	case rawScore <= 2:
		scaledScore = 10 // Mốc điểm tùy ý
	case rawScore <= 4:
		scaledScore = 20
	case rawScore <= 6:
		scaledScore = 40
	case rawScore <= 8:
		scaledScore = 60
	case rawScore <= 10:
		scaledScore = 80
	case rawScore <= 12:
		scaledScore = 100
	case rawScore <= 14:
		scaledScore = 110
	case rawScore <= 16:
		scaledScore = 120
	case rawScore <= 18:
		scaledScore = 130
	case rawScore == 18.8:
		scaledScore = 140 // THÊM TEST CASE CỤ THỂ CHO 18.8 ĐỂ DEBUG
	case rawScore <= 19:
		scaledScore = 140
	case rawScore <= 21:
		scaledScore = 150
	case rawScore <= 23:
		scaledScore = 170
	case rawScore <= 25:
		scaledScore = 180
	case rawScore <= 27:
		scaledScore = 190
	case rawScore >= MaxRawScorePossibleForTOEICWriting:
		scaledScore = MaxScaledScoreForTOEICWriting // Max score 28 -> 200
	default:
		// Nếu không có trong các case trên, sử dụng phép nội suy tuyến tính đơn giản như một fallback
		// Hoặc bạn có thể định nghĩa các khoảng điểm chặt chẽ hơn.
		log.Warn().Float64("rawScore", rawScore).Msg("ScoreConverterService: Raw score did not match specific case, using linear fallback for this segment.")
		// Ví dụ: nếu rawScore là 18.5, nó sẽ không khớp case nào ở trên.
		// Cần một logic nội suy tốt hơn hoặc một bảng map chi tiết.
		// Tạm thời, nếu không khớp, dùng lại linear scaling cho debug
		scaledScore = (rawScore / MaxRawScorePossibleForTOEICWriting) * MaxScaledScoreForTOEICWriting
	}
	// ---- KẾT THÚC LOGIC QUY ĐỔI ----

	// Đảm bảo điểm không vượt quá 200 và không nhỏ hơn 0
	if scaledScore > MaxScaledScoreForTOEICWriting {
		scaledScore = MaxScaledScoreForTOEICWriting
	}
	if scaledScore < 0 {
		scaledScore = 0
	}

	// Làm tròn điểm theo chuẩn TOEIC (thường là bội số của 5 hoặc 10)
	// Ví dụ: làm tròn đến bội số của 5 gần nhất
	finalScaledScore := math.Round(scaledScore/5.0) * 5.0
	log.Debug().Float64("rawScore", rawScore).Float64("calculatedScaledScore", scaledScore).Float64("finalRoundedScaledScore", finalScaledScore).Msg("ScoreConverterService: Score conversion details")

	return finalScaledScore, nil
}
